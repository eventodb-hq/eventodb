package pebble

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
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
	msg.Time = time.Now().UTC()

	// Generate ID if not provided
	if msg.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate UUID: %w", err)
		}
		msg.ID = id.String()
	}

	// Serialize message to JSON using jsoniter
	messageJSON, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}

	// Compress JSON using S2
	compressedMessage := compressJSON(messageJSON)

	// Extract category
	category := extractCategory(streamName)

	// Create atomic batch with all 5 keys
	batch := handle.db.NewBatch()
	defer batch.Close()

	// 1. M:{gp} → compressed message JSON
	batch.Set(formatMessageKey(globalPosition), compressedMessage, nil)

	// 2. SI:{stream}:{position} → global position
	batch.Set(formatStreamIndexKey(streamName, newPosition), []byte(encodeInt64(globalPosition)), nil)

	// 3. CI:{category}:{gp} → stream name
	batch.Set(formatCategoryIndexKey(category, globalPosition), []byte(streamName), nil)

	// 4. VI:{stream} → new position
	batch.Set(formatVersionIndexKey(streamName), []byte(encodeInt64(newPosition)), nil)

	// 5. GP → incremented global position
	batch.Set(formatGlobalPositionKey(), []byte(encodeInt64(globalPosition+1)), nil)

	// Commit batch WITHOUT sync for performance (WAL provides durability)
	// The WAL (Write-Ahead Log) ensures durability without forcing fsync on every write
	if err := batch.Commit(pebble.NoSync); err != nil {
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

// ImportBatch writes messages with explicit positions (for import/restore)
// All messages in batch are inserted in a single atomic batch
func (s *PebbleStore) ImportBatch(ctx context.Context, namespace string, messages []*store.Message) error {
	if len(messages) == 0 {
		return nil
	}

	// Get namespace handle (lazy load if needed)
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return err
	}

	// Serialize writes to maintain consistency
	handle.writeMu.Lock()
	defer handle.writeMu.Unlock()

	// First pass: check for duplicate global positions
	for _, msg := range messages {
		key := formatMessageKey(msg.GlobalPosition)
		_, closer, err := handle.db.Get(key)
		if err == nil {
			closer.Close()
			return fmt.Errorf("%w: %d", store.ErrPositionExists, msg.GlobalPosition)
		}
		if err != pebble.ErrNotFound {
			return fmt.Errorf("failed to check position existence: %w", err)
		}
	}

	// Create atomic batch
	batch := handle.db.NewBatch()
	defer batch.Close()

	// Track max global position for updating GP counter
	var maxGlobalPosition int64 = 0

	for _, msg := range messages {
		// Serialize message to JSON
		messageJSON, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to serialize message: %w", err)
		}

		// Compress JSON using S2
		compressedMessage := compressJSON(messageJSON)

		// Extract category
		category := extractCategory(msg.StreamName)

		// 1. M:{gp} → compressed message JSON
		batch.Set(formatMessageKey(msg.GlobalPosition), compressedMessage, nil)

		// 2. SI:{stream}:{position} → global position
		batch.Set(formatStreamIndexKey(msg.StreamName, msg.Position), []byte(encodeInt64(msg.GlobalPosition)), nil)

		// 3. CI:{category}:{gp} → stream name
		batch.Set(formatCategoryIndexKey(category, msg.GlobalPosition), []byte(msg.StreamName), nil)

		// 4. VI:{stream} → update if this position is higher than current
		currentVersion, _ := getStreamVersion(handle.db, msg.StreamName)
		if msg.Position > currentVersion {
			batch.Set(formatVersionIndexKey(msg.StreamName), []byte(encodeInt64(msg.Position)), nil)
		}

		// Track max for GP counter update
		if msg.GlobalPosition > maxGlobalPosition {
			maxGlobalPosition = msg.GlobalPosition
		}
	}

	// 5. Update GP counter if imported positions exceed current
	currentGP, _ := getAndIncrementGlobalPosition(handle.db)
	if maxGlobalPosition >= currentGP {
		batch.Set(formatGlobalPositionKey(), []byte(encodeInt64(maxGlobalPosition+1)), nil)
	}

	// Commit batch
	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("failed to commit import batch: %w", err)
	}

	return nil
}

// ClearNamespaceMessages deletes all messages from a namespace
func (s *PebbleStore) ClearNamespaceMessages(ctx context.Context, namespace string) (int64, error) {
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return 0, err
	}

	handle.writeMu.Lock()
	defer handle.writeMu.Unlock()

	// Count messages first
	var count int64
	iter, err := handle.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("M:"),
		UpperBound: prefixUpperBound([]byte("M:")),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create iterator: %w", err)
	}
	for iter.First(); iter.Valid(); iter.Next() {
		count++
	}
	iter.Close()

	// Delete all data using prefix deletion
	// Delete M: (messages), SI: (stream index), CI: (category index), VI: (version index)
	prefixes := []string{"M:", "SI:", "CI:", "VI:"}

	batch := handle.db.NewBatch()
	defer batch.Close()

	for _, prefix := range prefixes {
		iter, err := handle.db.NewIter(&pebble.IterOptions{
			LowerBound: []byte(prefix),
			UpperBound: prefixUpperBound([]byte(prefix)),
		})
		if err != nil {
			return 0, fmt.Errorf("failed to create iterator for %s: %w", prefix, err)
		}

		for iter.First(); iter.Valid(); iter.Next() {
			batch.Delete(iter.Key(), nil)
		}
		iter.Close()
	}

	// Reset global position counter
	batch.Set(formatGlobalPositionKey(), []byte(encodeInt64(1)), nil)

	if err := batch.Commit(pebble.Sync); err != nil {
		return 0, fmt.Errorf("failed to commit clear batch: %w", err)
	}

	return count, nil
}
