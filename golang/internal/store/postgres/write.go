package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
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
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate UUID: %w", err)
		}
		msg.ID = id.String()
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

// ImportBatch writes messages with explicit positions (for import/restore)
// All messages in batch are inserted in a single transaction
func (s *PostgresStore) ImportBatch(ctx context.Context, namespace string, messages []*store.Message) error {
	if len(messages) == 0 {
		return nil
	}

	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement for batch insert with explicit global_position
	// Use OVERRIDING SYSTEM VALUE to allow explicit global_position on SERIAL column
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		INSERT INTO "%s".messages (id, stream_name, type, position, global_position, data, metadata, time)
		OVERRIDING SYSTEM VALUE
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)
	`, schemaName))
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, msg := range messages {
		var dataParam interface{} = nil
		var metadataParam interface{} = nil

		if msg.Data != nil {
			dataJSON, err := json.Marshal(msg.Data)
			if err != nil {
				return fmt.Errorf("failed to marshal data: %w", err)
			}
			dataParam = string(dataJSON)
		}
		if msg.Metadata != nil {
			metadataJSON, err := json.Marshal(msg.Metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal metadata: %w", err)
			}
			metadataParam = string(metadataJSON)
		}

		_, err = stmt.ExecContext(ctx,
			msg.ID,
			msg.StreamName,
			msg.Type,
			msg.Position,
			msg.GlobalPosition,
			dataParam,
			metadataParam,
			msg.Time,
		)
		if err != nil {
			// Check for unique constraint violation (duplicate global_position)
			if isPostgresUniqueConstraintError(err) {
				return fmt.Errorf("%w: %d", store.ErrPositionExists, msg.GlobalPosition)
			}
			return fmt.Errorf("failed to insert message: %w", err)
		}
	}

	// Update the sequence to ensure new writes get positions after imported ones
	// Find the max global_position we just inserted
	var maxGP int64
	for _, msg := range messages {
		if msg.GlobalPosition > maxGP {
			maxGP = msg.GlobalPosition
		}
	}

	// Set the sequence to max + 1 so next auto-generated value is correct
	_, err = tx.ExecContext(ctx, fmt.Sprintf(
		`SELECT setval('"%s".messages_global_position_seq', $1, true)`,
		schemaName,
	), maxGP)
	if err != nil {
		return fmt.Errorf("failed to update sequence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// isPostgresUniqueConstraintError checks if the error is a unique constraint violation
func isPostgresUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// PostgreSQL unique constraint error patterns (pgx driver)
	return strings.Contains(errStr, "duplicate key value") ||
		strings.Contains(errStr, "unique constraint") ||
		strings.Contains(errStr, "23505") // PostgreSQL error code for unique_violation
}

// ClearNamespaceMessages deletes all messages from a namespace
func (s *PostgresStore) ClearNamespaceMessages(ctx context.Context, namespace string) (int64, error) {
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return 0, err
	}

	// Delete all messages and reset sequence
	result, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM "%s".messages`, schemaName))
	if err != nil {
		return 0, fmt.Errorf("failed to delete messages: %w", err)
	}

	deleted, _ := result.RowsAffected()

	// Reset the sequence to 1
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(
		`ALTER SEQUENCE "%s".messages_global_position_seq RESTART WITH 1`,
		schemaName,
	))
	if err != nil {
		return deleted, fmt.Errorf("failed to reset sequence: %w", err)
	}

	return deleted, nil
}
