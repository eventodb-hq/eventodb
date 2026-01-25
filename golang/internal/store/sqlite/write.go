package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
)

// WriteMessage writes a message with serialized access per namespace
func (s *SQLiteStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *store.Message) (*store.WriteResult, error) {
	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return nil, err
	}

	// Generate UUID if not provided
	if msg.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate UUID: %w", err)
		}
		msg.ID = id.String()
	}

	// Serialize writes to this namespace
	handle.writeMu.Lock()
	defer handle.writeMu.Unlock()

	return executeWriteMessage(ctx, handle.db, streamName, msg)
}

// executeWriteMessage performs the actual write
func executeWriteMessage(ctx context.Context, db *sql.DB, streamName string, msg *store.Message) (*store.WriteResult, error) {
	if _, err := uuid.Parse(msg.ID); err != nil {
		return nil, fmt.Errorf("invalid UUID format: %w", err)
	}

	// Get current stream version
	var currentVersion sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT MAX(position) FROM messages WHERE stream_name = ?`, streamName).Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get stream version: %w", err)
	}

	streamVersion := int64(-1)
	if currentVersion.Valid {
		streamVersion = currentVersion.Int64
	}

	// Check expected version
	if msg.ExpectedVersion != nil {
		if *msg.ExpectedVersion != streamVersion {
			return nil, store.NewVersionConflictError(streamName, *msg.ExpectedVersion, streamVersion)
		}
	}

	nextPosition := streamVersion + 1

	var dataJSON, metadataJSON []byte
	if msg.Data != nil {
		dataJSON, err = json.Marshal(msg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data: %w", err)
		}
	}
	if msg.Metadata != nil {
		metadataJSON, err = json.Marshal(msg.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	result, err := db.ExecContext(ctx,
		`INSERT INTO messages (id, stream_name, type, position, data, metadata, time) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, streamName, msg.Type, nextPosition, dataJSON, metadataJSON, time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to insert message: %w", err)
	}

	globalPosition, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get global position: %w", err)
	}

	return &store.WriteResult{
		Position:       nextPosition,
		GlobalPosition: globalPosition,
	}, nil
}

// ImportBatch writes messages with explicit positions (for import/restore)
// All messages in batch are inserted in a single transaction
func (s *SQLiteStore) ImportBatch(ctx context.Context, namespace string, messages []*store.Message) error {
	if len(messages) == 0 {
		return nil
	}

	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return err
	}

	// Serialize writes to this namespace
	handle.writeMu.Lock()
	defer handle.writeMu.Unlock()

	tx, err := handle.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement for batch insert
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO messages (id, stream_name, type, position, global_position, data, metadata, time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, msg := range messages {
		var dataJSON, metadataJSON []byte
		if msg.Data != nil {
			dataJSON, err = json.Marshal(msg.Data)
			if err != nil {
				return fmt.Errorf("failed to marshal data: %w", err)
			}
		}
		if msg.Metadata != nil {
			metadataJSON, err = json.Marshal(msg.Metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal metadata: %w", err)
			}
		}

		_, err = stmt.ExecContext(ctx,
			msg.ID,
			msg.StreamName,
			msg.Type,
			msg.Position,
			msg.GlobalPosition,
			dataJSON,
			metadataJSON,
			msg.Time.Unix(),
		)
		if err != nil {
			// Check for unique constraint violation (duplicate global_position)
			if isUniqueConstraintError(err) {
				return fmt.Errorf("%w: %d", store.ErrPositionExists, msg.GlobalPosition)
			}
			return fmt.Errorf("failed to insert message: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// isUniqueConstraintError checks if the error is a unique constraint violation
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// SQLite unique constraint error patterns
	return strings.Contains(errStr, "UNIQUE constraint failed") ||
		strings.Contains(errStr, "constraint failed")
}
