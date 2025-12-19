package pebble

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/google/uuid"
	"github.com/message-db/message-db/internal/store"
)

// WriteMessage writes a message to a stream in the specified namespace
func (s *PebbleStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *store.Message) (*store.WriteResult, error) {
	if streamName == "" {
		return nil, fmt.Errorf("stream name cannot be empty")
	}

	// Get namespace handle (lazy load if needed)
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Serialize writes to maintain GP counter consistency
	handle.writeMu.Lock()
	defer handle.writeMu.Unlock()

	// Get current stream version
	currentVersion, err := getStreamVersion(handle.db, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream version: %w", err)
	}

	// Check optimistic locking
	if msg.ExpectedVersion != nil {
		if *msg.ExpectedVersion != currentVersion {
			return nil, store.ErrVersionConflict
		}
	}

	// Calculate new position
	newPosition := currentVersion + 1

	// Get and increment global position
	globalPosition, err := getAndIncrementGlobalPosition(handle.db)
	if err != nil {
		return nil, fmt.Errorf("failed to get global position: %w", err)
	}

	// Prepare message
	msg.Position = newPosition
	msg.GlobalPosition = globalPosition
	msg.StreamName = streamName

	// Generate ID if not provided
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Serialize message to JSON
	messageJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}

	// Extract category
	category := extractCategory(streamName)

	// Create atomic batch with all 5 keys
	batch := handle.db.NewBatch()
	defer batch.Close()

	// 1. M:{gp} → full message JSON
	batch.Set(formatMessageKey(globalPosition), messageJSON, nil)

	// 2. SI:{stream}:{position} → global position
	batch.Set(formatStreamIndexKey(streamName, newPosition), []byte(encodeInt64(globalPosition)), nil)

	// 3. CI:{category}:{gp} → stream name
	batch.Set(formatCategoryIndexKey(category, globalPosition), []byte(streamName), nil)

	// 4. VI:{stream} → new position
	batch.Set(formatVersionIndexKey(streamName), []byte(encodeInt64(newPosition)), nil)

	// 5. GP → incremented global position
	batch.Set(formatGlobalPositionKey(), []byte(encodeInt64(globalPosition+1)), nil)

	// Commit batch with sync
	if err := batch.Commit(pebble.Sync); err != nil {
		return nil, fmt.Errorf("failed to commit write batch: %w", err)
	}

	return &store.WriteResult{
		Position:       newPosition,
		GlobalPosition: globalPosition,
	}, nil
}

// getStreamVersion reads the current version from VI:{stream} or returns -1
func getStreamVersion(db *pebble.DB, stream string) (int64, error) {
	key := formatVersionIndexKey(stream)
	value, closer, err := db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return -1, nil // Stream doesn't exist yet
		}
		return -1, err
	}
	defer closer.Close()

	version, err := decodeInt64(value)
	if err != nil {
		return -1, fmt.Errorf("failed to decode version: %w", err)
	}

	return version, nil
}

// getAndIncrementGlobalPosition reads and increments the GP counter
func getAndIncrementGlobalPosition(db *pebble.DB) (int64, error) {
	key := formatGlobalPositionKey()
	value, closer, err := db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			// First message in namespace, start at 1
			return 1, nil
		}
		return 0, err
	}
	defer closer.Close()

	gp, err := decodeInt64(value)
	if err != nil {
		return 0, fmt.Errorf("failed to decode global position: %w", err)
	}

	return gp, nil
}
