package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/message-db/message-db/internal/store"
)

// WriteMessage writes a message to a stream with optimistic locking support
func (s *PostgresStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *store.Message) (*store.WriteResult, error) {
	// 1. Get schema name for namespace
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Generate UUID if not provided
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// 3. Validate UUID format
	if _, err := uuid.Parse(msg.ID); err != nil {
		return nil, fmt.Errorf("invalid UUID format: %w", err)
	}

	// 4. Serialize data and metadata to JSONB
	// We need to pass as strings or interface{} for JSONB parameters
	var dataParam interface{} = nil
	var metadataParam interface{} = nil

	if msg.Data != nil {
		dataJSON, err := json.Marshal(msg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data: %w", err)
		}
		dataParam = string(dataJSON)
	}

	if msg.Metadata != nil {
		metadataJSON, err := json.Marshal(msg.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataParam = string(metadataJSON)
	}

	// 5. Call write_message stored procedure
	// Note: write_message() internally calls acquire_lock() for category-level locking
	query := fmt.Sprintf(
		`SELECT "%s".write_message($1, $2, $3, $4::jsonb, $5::jsonb, $6)`,
		schemaName,
	)

	var position int64
	var lastErr error

	// Retry logic for handling race conditions during schema creation
	for attempts := 0; attempts < 3; attempts++ {
		err = s.db.QueryRowContext(
			ctx,
			query,
			msg.ID,
			streamName,
			msg.Type,
			dataParam,
			metadataParam,
			msg.ExpectedVersion,
		).Scan(&position)

		if err == nil {
			// Success
			break
		}

		lastErr = err

		// Check for version conflict error (don't retry this)
		// pgx doesn't prefix errors with "pq:", so check for the actual error message
		errMsg := err.Error()
		if strings.Contains(errMsg, "Wrong expected version") {
			return nil, store.ErrVersionConflict
		}

		// Check if error is due to missing relation (schema not ready yet)
		if strings.Contains(err.Error(), "does not exist") && attempts < 2 {
			// Wait a bit and retry
			time.Sleep(20 * time.Millisecond)
			continue
		}

		// Other errors, don't retry
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to write message: %w", lastErr)
	}

	// 6. Query for global_position
	// We need to get the global_position that was just assigned
	globalQuery := fmt.Sprintf(
		`SELECT global_position FROM "%s".messages WHERE stream_name = $1 AND position = $2`,
		schemaName,
	)

	var globalPosition int64

	// Retry logic for the global position query as well
	for attempts := 0; attempts < 3; attempts++ {
		err = s.db.QueryRowContext(ctx, globalQuery, streamName, position).Scan(&globalPosition)
		if err == nil {
			break
		}

		// Check if error is due to missing relation (schema not ready yet)
		if strings.Contains(err.Error(), "does not exist") && attempts < 2 {
			time.Sleep(20 * time.Millisecond)
			continue
		}

		return nil, fmt.Errorf("failed to get global position: %w", err)
	}

	return &store.WriteResult{
		Position:       position,
		GlobalPosition: globalPosition,
	}, nil
}
