package timescale

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/message-db/message-db/internal/store"
)

// GetStreamMessages retrieves messages from a stream
func (s *TimescaleStore) GetStreamMessages(ctx context.Context, namespace, streamName string, opts *store.GetOpts) ([]*store.Message, error) {
	// 1. Get schema name for namespace
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Use defaults if opts is nil
	if opts == nil {
		opts = store.NewGetOpts()
	}

	// 3. Determine starting position
	position := opts.Position
	if opts.GlobalPosition != nil {
		position = opts.Position
	}

	// 4. Call get_stream_messages stored procedure
	query := fmt.Sprintf(
		`SELECT * FROM "%s".get_stream_messages($1, $2, $3, $4)`,
		schemaName,
	)

	rows, err := s.db.QueryContext(
		ctx,
		query,
		streamName,
		position,
		opts.BatchSize,
		nil, // condition is deprecated
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query stream messages: %w", err)
	}
	defer rows.Close()

	// 5. Parse results
	return s.scanMessages(rows)
}

// GetCategoryMessages retrieves messages from a category with consumer group support
func (s *TimescaleStore) GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *store.CategoryOpts) ([]*store.Message, error) {
	// 1. Get schema name for namespace
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Use defaults if opts is nil
	if opts == nil {
		opts = store.NewCategoryOpts()
	}

	// 3. Determine starting position
	position := opts.Position
	if opts.GlobalPosition != nil {
		position = *opts.GlobalPosition
	}

	// 4. Call get_category_messages stored procedure
	query := fmt.Sprintf(
		`SELECT * FROM "%s".get_category_messages($1, $2, $3, $4, $5, $6, $7)`,
		schemaName,
	)

	rows, err := s.db.QueryContext(
		ctx,
		query,
		categoryName,
		position,
		opts.BatchSize,
		opts.Correlation,
		opts.ConsumerMember,
		opts.ConsumerSize,
		nil, // condition is deprecated
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query category messages: %w", err)
	}
	defer rows.Close()

	// 5. Parse results
	return s.scanMessages(rows)
}

// GetLastStreamMessage retrieves the last message from a stream
func (s *TimescaleStore) GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*store.Message, error) {
	// 1. Get schema name for namespace
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Call get_last_stream_message stored procedure
	query := fmt.Sprintf(
		`SELECT * FROM "%s".get_last_stream_message($1, $2)`,
		schemaName,
	)

	rows, err := s.db.QueryContext(
		ctx,
		query,
		streamName,
		msgType,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query last stream message: %w", err)
	}
	defer rows.Close()

	// 3. Parse results
	messages, err := s.scanMessages(rows)
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, store.ErrStreamNotFound
	}

	return messages[0], nil
}

// GetStreamVersion retrieves the current version of a stream
func (s *TimescaleStore) GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error) {
	// 1. Get schema name for namespace
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return 0, err
	}

	// 2. Call stream_version stored procedure
	query := fmt.Sprintf(
		`SELECT "%s".stream_version($1)`,
		schemaName,
	)

	var version int64
	err = s.db.QueryRowContext(ctx, query, streamName).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get stream version: %w", err)
	}

	return version, nil
}

// scanMessages is a helper function to scan rows into Message structs
func (s *TimescaleStore) scanMessages(rows *sql.Rows) ([]*store.Message, error) {
	messages := []*store.Message{}

	for rows.Next() {
		var (
			id             uuid.UUID
			streamName     string
			msgType        string
			position       int64
			globalPosition int64
			dataJSON       []byte
			metadataJSON   []byte
			timestamp      sql.NullTime
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
		if len(dataJSON) > 0 {
			if err := json.Unmarshal(dataJSON, &data); err != nil {
				return nil, fmt.Errorf("failed to unmarshal data: %w", err)
			}
		}

		// Parse JSON metadata
		var metadata map[string]interface{}
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		msg := &store.Message{
			ID:             id.String(),
			StreamName:     streamName,
			Type:           msgType,
			Position:       position,
			GlobalPosition: globalPosition,
			Data:           data,
			Metadata:       metadata,
		}

		if timestamp.Valid {
			msg.Time = timestamp.Time
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}
