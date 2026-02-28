package timescale

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
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

	// 3. If GlobalPosition is specified, use a direct query instead of the stored procedure
	if opts.GlobalPosition != nil {
		query := fmt.Sprintf(
			`SELECT id, stream_name, type, position, global_position, data, metadata, time
			 FROM "%s".messages
			 WHERE stream_name = $1 AND global_position >= $2
			 ORDER BY position ASC`,
			schemaName,
		)

		if opts.BatchSize > 0 {
			query += fmt.Sprintf(` LIMIT %d`, opts.BatchSize)
		}

		rows, err := s.db.QueryContext(ctx, query, streamName, *opts.GlobalPosition)
		if err != nil {
			return nil, fmt.Errorf("failed to query stream messages: %w", err)
		}
		defer rows.Close()

		return s.scanMessages(rows, opts.BatchSize)
	}

	// 4. Call get_stream_messages stored procedure for position-based queries
	query := fmt.Sprintf(
		`SELECT * FROM "%s".get_stream_messages($1, $2, $3, $4)`,
		schemaName,
	)

	rows, err := s.db.QueryContext(
		ctx,
		query,
		streamName,
		opts.Position,
		opts.BatchSize,
		nil, // condition is deprecated
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query stream messages: %w", err)
	}
	defer rows.Close()

	// 5. Parse results with capacity hint
	return s.scanMessages(rows, opts.BatchSize)
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

	// 5. Parse results with capacity hint
	return s.scanMessages(rows, opts.BatchSize)
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

	// 3. Parse results with capacity hint (expect 1 message)
	messages, err := s.scanMessages(rows, 1)
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

// ListStreams returns streams in a namespace with optional prefix filtering and pagination.
func (s *TimescaleStore) ListStreams(ctx context.Context, namespace string, opts *store.ListStreamsOpts) ([]*store.StreamInfo, error) {
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &store.ListStreamsOpts{Limit: 100}
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := fmt.Sprintf(`
		SELECT stream_name, MAX(position) AS version, MAX(time) AS last_activity
		FROM "%s".messages
		WHERE ($1 = '' OR stream_name LIKE $1 || '%%')
		  AND ($2 = '' OR stream_name > $2)
		GROUP BY stream_name
		ORDER BY stream_name ASC
		LIMIT $3`, schemaName)

	rows, err := s.db.QueryContext(ctx, query, opts.Prefix, opts.Cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list streams: %w", err)
	}
	defer rows.Close()

	var results []*store.StreamInfo
	for rows.Next() {
		var si store.StreamInfo
		var lastActivity sql.NullTime
		if err := rows.Scan(&si.StreamName, &si.Version, &lastActivity); err != nil {
			return nil, fmt.Errorf("failed to scan stream info: %w", err)
		}
		if lastActivity.Valid {
			si.LastActivity = lastActivity.Time.UTC()
		}
		results = append(results, &si)
	}
	if results == nil {
		results = []*store.StreamInfo{}
	}
	return results, rows.Err()
}

// ListCategories returns distinct categories in a namespace with stream and message counts.
func (s *TimescaleStore) ListCategories(ctx context.Context, namespace string) ([]*store.CategoryInfo, error) {
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT split_part(stream_name, '-', 1) AS category,
		       COUNT(DISTINCT stream_name) AS stream_count,
		       COUNT(*) AS message_count
		FROM "%s".messages
		GROUP BY category
		ORDER BY category ASC`, schemaName)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	defer rows.Close()

	var results []*store.CategoryInfo
	for rows.Next() {
		var ci store.CategoryInfo
		if err := rows.Scan(&ci.Category, &ci.StreamCount, &ci.MessageCount); err != nil {
			return nil, fmt.Errorf("failed to scan category info: %w", err)
		}
		results = append(results, &ci)
	}
	if results == nil {
		results = []*store.CategoryInfo{}
	}
	return results, rows.Err()
}

// scanMessages is a helper function to scan rows into Message structs
func (s *TimescaleStore) scanMessages(rows *sql.Rows, capacityHint int64) ([]*store.Message, error) {
	// Pre-allocate slice with capacity hint to reduce allocations
	capacity := int(capacityHint)
	if capacity <= 0 || capacity > 10000 {
		capacity = 1000 // reasonable default
	}
	messages := make([]*store.Message, 0, capacity)

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
