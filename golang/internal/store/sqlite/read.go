package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/eventodb/eventodb/internal/store"
)

// GetStreamMessages retrieves messages from a stream
func (s *SQLiteStore) GetStreamMessages(ctx context.Context, namespace, streamName string, opts *store.GetOpts) ([]*store.Message, error) {
	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = store.NewGetOpts()
	}

	var query string
	var args []interface{}

	// Use global_position filter if GlobalPosition is specified, otherwise use position
	if opts.GlobalPosition != nil {
		query = `SELECT id, stream_name, type, position, global_position, data, metadata, time
			FROM messages WHERE stream_name = ? AND global_position >= ? ORDER BY position ASC`
		args = []interface{}{streamName, *opts.GlobalPosition}
	} else {
		query = `SELECT id, stream_name, type, position, global_position, data, metadata, time
			FROM messages WHERE stream_name = ? AND position >= ? ORDER BY position ASC`
		args = []interface{}{streamName, opts.Position}
	}

	if opts.BatchSize > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.BatchSize)
	}

	rows, err := handle.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows, opts.BatchSize)
}

// GetCategoryMessages retrieves messages from a category
func (s *SQLiteStore) GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *store.CategoryOpts) ([]*store.Message, error) {
	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = store.NewCategoryOpts()
	}

	// For category queries, Position represents the global position filter
	// GlobalPosition is an alternative way to specify the same thing
	position := opts.Position
	if opts.GlobalPosition != nil {
		position = *opts.GlobalPosition
	}

	query := `SELECT id, stream_name, type, position, global_position, data, metadata, time
		FROM messages
		WHERE substr(stream_name, 1, instr(stream_name || '-', '-') - 1) = ?
		AND global_position >= ?
		ORDER BY global_position ASC`
	args := []interface{}{categoryName, position}

	if opts.Correlation != nil && *opts.Correlation != "" {
		query = `SELECT id, stream_name, type, position, global_position, data, metadata, time
			FROM messages
			WHERE substr(stream_name, 1, instr(stream_name || '-', '-') - 1) = ?
			AND global_position >= ?
			AND json_extract(metadata, '$.correlationStreamName') LIKE ?
			ORDER BY global_position ASC`
		args = append(args, *opts.Correlation+"-%")
	}

	rows, err := handle.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	allMessages, err := scanMessages(rows, opts.BatchSize)
	if err != nil {
		return nil, err
	}

	// Consumer group filtering
	if opts.ConsumerMember != nil && opts.ConsumerSize != nil {
		// Pre-allocate for consumer group filtering
		capacity := int(opts.BatchSize)
		if capacity <= 0 || capacity > 10000 {
			capacity = 1000
		}
		messages := make([]*store.Message, 0, capacity)
		for _, msg := range allMessages {
			if store.IsAssignedToConsumerMember(msg.StreamName, *opts.ConsumerMember, *opts.ConsumerSize) {
				messages = append(messages, msg)
				if opts.BatchSize > 0 && int64(len(messages)) >= opts.BatchSize {
					break
				}
			}
		}
		return messages, nil
	}

	if opts.BatchSize > 0 && int64(len(allMessages)) > opts.BatchSize {
		return allMessages[:opts.BatchSize], nil
	}

	return allMessages, nil
}

// GetLastStreamMessage retrieves the last message from a stream
func (s *SQLiteStore) GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*store.Message, error) {
	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return nil, err
	}

	query := `SELECT id, stream_name, type, position, global_position, data, metadata, time
		FROM messages WHERE stream_name = ?`
	args := []interface{}{streamName}

	if msgType != nil && *msgType != "" {
		query += ` AND type = ?`
		args = append(args, *msgType)
	}
	query += ` ORDER BY position DESC LIMIT 1`

	rows, err := handle.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	messages, err := scanMessages(rows, 1)
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, store.ErrStreamNotFound
	}
	return messages[0], nil
}

// GetStreamVersion retrieves the current version of a stream
func (s *SQLiteStore) GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error) {
	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return 0, err
	}

	var version int64
	err = handle.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) FROM messages WHERE stream_name = ?`,
		streamName).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get stream version: %w", err)
	}
	return version, nil
}

func scanMessages(rows *sql.Rows, capacityHint int64) ([]*store.Message, error) {
	// Pre-allocate slice with capacity hint to reduce allocations
	capacity := int(capacityHint)
	if capacity <= 0 || capacity > 10000 {
		capacity = 1000 // reasonable default
	}
	messages := make([]*store.Message, 0, capacity)

	for rows.Next() {
		var id, streamName, msgType string
		var position, globalPosition, timestamp int64
		var dataJSON, metadataJSON []byte

		if err := rows.Scan(&id, &streamName, &msgType, &position, &globalPosition, &dataJSON, &metadataJSON, &timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}

		var data, metadata map[string]interface{}
		if len(dataJSON) > 0 && string(dataJSON) != "null" {
			json.Unmarshal(dataJSON, &data)
		}
		if len(metadataJSON) > 0 && string(metadataJSON) != "null" {
			json.Unmarshal(metadataJSON, &metadata)
		}

		messages = append(messages, &store.Message{
			ID:             id,
			StreamName:     streamName,
			Type:           msgType,
			Position:       position,
			GlobalPosition: globalPosition,
			Data:           data,
			Metadata:       metadata,
			Time:           time.Unix(timestamp, 0).UTC(),
		})
	}

	return messages, rows.Err()
}
