package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/message-db/message-db/internal/store"
)

// GetStreamMessages retrieves messages from a stream
func (s *SQLiteStore) GetStreamMessages(ctx context.Context, namespace, streamName string, opts *store.GetOpts) ([]*store.Message, error) {
	// 1. Get or create namespace DB
	nsDB, err := s.getOrCreateNamespaceDB(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Use defaults if opts is nil
	if opts == nil {
		opts = store.NewGetOpts()
	}

	// 3. Build query
	query := `
		SELECT id, stream_name, type, position, global_position, data, metadata, time
		FROM messages
		WHERE stream_name = ?
		  AND position >= ?
		ORDER BY position ASC
	`

	args := []interface{}{streamName, opts.Position}

	// 4. Add batch size limit
	if opts.BatchSize > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.BatchSize)
	}

	// 5. Execute query
	rows, err := nsDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query stream messages: %w", err)
	}
	defer rows.Close()

	// 6. Scan results
	return s.scanMessages(rows)
}

// GetCategoryMessages retrieves messages from a category with consumer group support
func (s *SQLiteStore) GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *store.CategoryOpts) ([]*store.Message, error) {
	// 1. Get or create namespace DB
	nsDB, err := s.getOrCreateNamespaceDB(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Use defaults if opts is nil
	if opts == nil {
		opts = store.NewCategoryOpts()
	}

	// 3. Build base query
	// Extract category from stream_name using SQLite string functions
	// Category is the part before the first '-'
	query := `
		SELECT id, stream_name, type, position, global_position, data, metadata, time
		FROM messages
		WHERE substr(stream_name, 1, instr(stream_name || '-', '-') - 1) = ?
		  AND global_position >= ?
	`

	args := []interface{}{categoryName, opts.Position}

	// 4. Add correlation filter if specified
	if opts.Correlation != nil && *opts.Correlation != "" {
		// Filter by metadata.correlationStreamName category
		// We need to extract the category from the JSON metadata field
		query += ` AND json_extract(metadata, '$.correlationStreamName') LIKE ?`
		correlationPattern := *opts.Correlation + "-%"
		args = append(args, correlationPattern)
	}

	// 5. Order by global_position
	query += ` ORDER BY global_position ASC`

	// 6. Execute query (we'll filter consumer groups in Go)
	rows, err := nsDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query category messages: %w", err)
	}
	defer rows.Close()

	// 7. Scan results
	allMessages, err := s.scanMessages(rows)
	if err != nil {
		return nil, err
	}

	// 8. Apply consumer group filtering if specified
	if opts.ConsumerMember != nil && opts.ConsumerSize != nil {
		messages := make([]*store.Message, 0, len(allMessages))
		collected := int64(0)

		for _, msg := range allMessages {
			// Check if this message is assigned to this consumer member
			if store.IsAssignedToConsumerMember(msg.StreamName, *opts.ConsumerMember, *opts.ConsumerSize) {
				messages = append(messages, msg)
				collected++

				// Check if we've reached batch size
				if opts.BatchSize > 0 && collected >= opts.BatchSize {
					break
				}
			}
		}

		return messages, nil
	}

	// 9. Apply batch size limit (if no consumer group filtering)
	if opts.BatchSize > 0 && int64(len(allMessages)) > opts.BatchSize {
		return allMessages[:opts.BatchSize], nil
	}

	return allMessages, nil
}

// GetLastStreamMessage retrieves the last message from a stream
func (s *SQLiteStore) GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*store.Message, error) {
	// 1. Get or create namespace DB
	nsDB, err := s.getOrCreateNamespaceDB(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Build query
	query := `
		SELECT id, stream_name, type, position, global_position, data, metadata, time
		FROM messages
		WHERE stream_name = ?
	`

	args := []interface{}{streamName}

	// 3. Add type filter if specified
	if msgType != nil && *msgType != "" {
		query += ` AND type = ?`
		args = append(args, *msgType)
	}

	// 4. Order by position descending and limit to 1
	query += ` ORDER BY position DESC LIMIT 1`

	// 5. Execute query
	rows, err := nsDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query last stream message: %w", err)
	}
	defer rows.Close()

	// 6. Scan results
	messages, err := s.scanMessages(rows)
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, nil // No message found
	}

	return messages[0], nil
}

// GetStreamVersion retrieves the current version of a stream
func (s *SQLiteStore) GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error) {
	// 1. Get or create namespace DB
	nsDB, err := s.getOrCreateNamespaceDB(namespace)
	if err != nil {
		return 0, err
	}

	// 2. Query for max position
	query := `SELECT COALESCE(MAX(position), -1) FROM messages WHERE stream_name = ?`

	var version int64
	err = nsDB.QueryRowContext(ctx, query, streamName).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get stream version: %w", err)
	}

	return version, nil
}

// scanMessages is a helper function to scan rows into Message structs
func (s *SQLiteStore) scanMessages(rows *sql.Rows) ([]*store.Message, error) {
	messages := []*store.Message{}

	for rows.Next() {
		var (
			id             string
			streamName     string
			msgType        string
			position       int64
			globalPosition int64
			dataJSON       []byte
			metadataJSON   []byte
			timestamp      int64
		)

		err := rows.Scan(
			&id,
			&streamName,
			&msgType,
			&position,
			&globalPosition,
			&dataJSON,
			&metadataJSON,
			&timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}

		// Parse JSON data
		var data map[string]interface{}
		if len(dataJSON) > 0 && string(dataJSON) != "null" {
			if err := json.Unmarshal(dataJSON, &data); err != nil {
				return nil, fmt.Errorf("failed to unmarshal data: %w", err)
			}
		}

		// Parse JSON metadata
		var metadata map[string]interface{}
		if len(metadataJSON) > 0 && string(metadataJSON) != "null" {
			if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		msg := &store.Message{
			ID:             id,
			StreamName:     streamName,
			Type:           msgType,
			Position:       position,
			GlobalPosition: globalPosition,
			Data:           data,
			Metadata:       metadata,
			Time:           time.Unix(timestamp, 0).UTC(),
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// extractCategoryFromStreamName extracts the category from a stream name
// This is a helper for building SQL queries
func extractCategoryFromStreamName(streamName string) string {
	if idx := strings.IndexByte(streamName, '-'); idx > 0 {
		return streamName[:idx]
	}
	return streamName
}
