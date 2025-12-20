package sqlite

import (
	"context"
	"testing"

	storepkg "github.com/eventodb/eventodb/internal/store"
)

// TestDebug_CategoryQuery - manual debug test
func TestDebug_CategoryQuery(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_debug_ns", "hash_debug", "Debug namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_debug_ns")

	// Write messages to different streams in the same category
	streams := []string{"account-1", "account-2", "account-3"}
	for _, stream := range streams {
		msg := &storepkg.Message{
			StreamName: stream,
			Type:       "TestEvent",
			Data:       map[string]interface{}{"test": "data"},
		}
		result, err := store.WriteMessage(ctx, "test_debug_ns", stream, msg)
		if err != nil {
			t.Fatalf("Failed to write message to %s: %v", stream, err)
		}
		t.Logf("Wrote to %s: position=%d, global_position=%d", stream, result.Position, result.GlobalPosition)
	}

	// Read from category
	opts := storepkg.NewCategoryOpts()
	t.Logf("Category opts: Position=%d, BatchSize=%d", opts.Position, opts.BatchSize)

	messages, err := store.GetCategoryMessages(ctx, "test_debug_ns", "account", opts)
	if err != nil {
		t.Fatalf("Failed to get category messages: %v", err)
	}

	t.Logf("Found %d messages", len(messages))
	for i, msg := range messages {
		t.Logf("  %d: stream=%s, gpos=%d, pos=%d", i+1, msg.StreamName, msg.GlobalPosition, msg.Position)
	}

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}
}

// TestDebug_StreamGlobalPosition - test stream read with GlobalPosition filter
func TestDebug_StreamGlobalPosition(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_gpos_ns", "hash_gpos", "Debug namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_gpos_ns")

	stream := "test-stream"

	// Write 4 messages
	var globalPositions []int64
	for i := 0; i < 4; i++ {
		msg := &storepkg.Message{
			StreamName: stream,
			Type:       "TestEvent",
			Data:       map[string]interface{}{"index": i},
		}
		result, err := store.WriteMessage(ctx, "test_gpos_ns", stream, msg)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
		globalPositions = append(globalPositions, result.GlobalPosition)
		t.Logf("Wrote message %d: position=%d, global_position=%d", i, result.Position, result.GlobalPosition)
	}

	// Get messages starting from the 3rd message's global position (index 2)
	targetGPos := globalPositions[2]
	t.Logf("Target global_position: %d", targetGPos)

	opts := &storepkg.GetOpts{
		GlobalPosition: &targetGPos,
		BatchSize:      1000,
	}

	messages, err := store.GetStreamMessages(ctx, "test_gpos_ns", stream, opts)
	if err != nil {
		t.Fatalf("Failed to get stream messages: %v", err)
	}

	t.Logf("Found %d messages", len(messages))
	for i, msg := range messages {
		t.Logf("  %d: gpos=%d, pos=%d", i+1, msg.GlobalPosition, msg.Position)
	}

	// Should get messages 3 and 4 (indices 2 and 3)
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}

	// Verify all messages have global_position >= targetGPos
	for i, msg := range messages {
		if msg.GlobalPosition < targetGPos {
			t.Errorf("Message %d has global_position %d, expected >= %d", i, msg.GlobalPosition, targetGPos)
		}
	}
}
