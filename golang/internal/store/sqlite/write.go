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

// WriteMessage writes a message with serialized access per namespace
func (s *SQLiteStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *store.Message) (*store.WriteResult, error) {
	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return nil, err
	}

	// Generate UUID if not provided
	if msg.ID == "" {
		msg.ID = uuid.New().String()
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
