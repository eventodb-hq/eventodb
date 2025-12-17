package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/message-db/message-db/internal/store"
)

// WriteMessage writes a message to a stream with optimistic locking support
// Implements the write_message logic in pure Go (no stored procedures)
func (s *SQLiteStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *store.Message) (*store.WriteResult, error) {
	// 1. Get or create namespace DB
	nsDB, err := s.getOrCreateNamespaceDB(namespace)
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

	// 4. Begin transaction (SQLite uses transaction-level locking, simpler than Postgres advisory locks)
	tx, err := nsDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 5. Get current stream version
	var currentVersion sql.NullInt64
	query := `SELECT MAX(position) FROM messages WHERE stream_name = ?`
	err = tx.QueryRowContext(ctx, query, streamName).Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get current stream version: %w", err)
	}

	// Convert NULL to -1 (stream doesn't exist yet)
	streamVersion := int64(-1)
	if currentVersion.Valid {
		streamVersion = currentVersion.Int64
	}

	// 6. Check expected version if provided (optimistic locking)
	if msg.ExpectedVersion != nil {
		if *msg.ExpectedVersion != streamVersion {
			return nil, store.NewVersionConflictError(streamName, *msg.ExpectedVersion, streamVersion)
		}
	}

	// 7. Calculate next position
	nextPosition := streamVersion + 1

	// 8. Serialize JSON data
	var dataJSON []byte
	if msg.Data != nil {
		dataJSON, err = json.Marshal(msg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data: %w", err)
		}
	}

	var metadataJSON []byte
	if msg.Metadata != nil {
		metadataJSON, err = json.Marshal(msg.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// 9. Insert message
	insertQuery := `
		INSERT INTO messages (id, stream_name, type, position, data, metadata, time)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := tx.ExecContext(
		ctx,
		insertQuery,
		msg.ID,
		streamName,
		msg.Type,
		nextPosition,
		dataJSON,
		metadataJSON,
		time.Now().Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert message: %w", err)
	}

	// 10. Get the global_position (auto-incremented by SQLite)
	globalPosition, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get global position: %w", err)
	}

	// 11. Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &store.WriteResult{
		Position:       nextPosition,
		GlobalPosition: globalPosition,
	}, nil
}
