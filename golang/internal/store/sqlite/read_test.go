package sqlite

import (
	"context"
	"testing"

	storepkg "github.com/message-db/message-db/internal/store"
)

// Helper function to write test messages
func writeTestMessages(t *testing.T, store *SQLiteStore, namespace, streamName string, count int) {
	t.Helper()

	ctx := context.Background()
	for i := 0; i < count; i++ {
		msg := &storepkg.Message{
			StreamName: streamName,
			Type:       "TestMessage",
			Data: map[string]interface{}{
				"index": i,
				"value": "Message " + string(rune(i+'A')),
			},
		}

		_, err := store.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("Failed to write test message %d: %v", i, err)
		}
	}
}

// MDB001_5A_T6: Test GetStreamMessages returns correct messages
func TestMDB001_5A_T6_GetStreamMessages_ReturnsCorrectMessages(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r1", "hash_r1", "Test namespace r1")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r1")

	streamName := "account-read-test"

	// Write 5 test messages
	writeTestMessages(t, store, "test_ns_r1", streamName, 5)

	// Read all messages
	messages, err := store.GetStreamMessages(ctx, "test_ns_r1", streamName, nil)
	if err != nil {
		t.Fatalf("Failed to read messages: %v", err)
	}

	if len(messages) != 5 {
		t.Fatalf("Expected 5 messages, got %d", len(messages))
	}

	// Verify messages are in order
	for i, msg := range messages {
		if msg.Position != int64(i) {
			t.Errorf("Expected message %d to have position %d, got %d", i, i, msg.Position)
		}

		if msg.StreamName != streamName {
			t.Errorf("Expected stream name '%s', got '%s'", streamName, msg.StreamName)
		}

		if msg.Type != "TestMessage" {
			t.Errorf("Expected type 'TestMessage', got '%s'", msg.Type)
		}

		if int(msg.Data["index"].(float64)) != i {
			t.Errorf("Expected index %d, got %v", i, msg.Data["index"])
		}
	}

	// Verify global positions are unique and increasing
	for i := 1; i < len(messages); i++ {
		if messages[i].GlobalPosition <= messages[i-1].GlobalPosition {
			t.Errorf("Expected increasing global positions, got %d -> %d",
				messages[i-1].GlobalPosition, messages[i].GlobalPosition)
		}
	}
}

// MDB001_5A_T7: Test GetStreamMessages with position offset and batch size
func TestMDB001_5A_T7_GetStreamMessages_WithPositionAndBatchSize(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r2", "hash_r2", "Test namespace r2")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r2")

	streamName := "account-pagination"

	// Write 10 test messages
	writeTestMessages(t, store, "test_ns_r2", streamName, 10)

	// Test 1: Read from position 0 with batch size 3
	opts1 := &storepkg.GetOpts{
		Position:  0,
		BatchSize: 3,
	}

	messages1, err := store.GetStreamMessages(ctx, "test_ns_r2", streamName, opts1)
	if err != nil {
		t.Fatalf("Failed to read messages (batch 1): %v", err)
	}

	if len(messages1) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages1))
	}

	if messages1[0].Position != 0 {
		t.Errorf("Expected first message position 0, got %d", messages1[0].Position)
	}

	// Test 2: Read from position 5 with batch size 3
	opts2 := &storepkg.GetOpts{
		Position:  5,
		BatchSize: 3,
	}

	messages2, err := store.GetStreamMessages(ctx, "test_ns_r2", streamName, opts2)
	if err != nil {
		t.Fatalf("Failed to read messages (batch 2): %v", err)
	}

	if len(messages2) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages2))
	}

	if messages2[0].Position != 5 {
		t.Errorf("Expected first message position 5, got %d", messages2[0].Position)
	}

	// Test 3: Read from position 8 with batch size 5 (should only get 2)
	opts3 := &storepkg.GetOpts{
		Position:  8,
		BatchSize: 5,
	}

	messages3, err := store.GetStreamMessages(ctx, "test_ns_r2", streamName, opts3)
	if err != nil {
		t.Fatalf("Failed to read messages (batch 3): %v", err)
	}

	if len(messages3) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages3))
	}

	// Test 4: Read all messages (batch size -1)
	opts4 := &storepkg.GetOpts{
		Position:  0,
		BatchSize: -1,
	}

	messages4, err := store.GetStreamMessages(ctx, "test_ns_r2", streamName, opts4)
	if err != nil {
		t.Fatalf("Failed to read all messages: %v", err)
	}

	if len(messages4) != 10 {
		t.Errorf("Expected 10 messages with batch size -1, got %d", len(messages4))
	}
}

// MDB001_5A_T8: Test GetCategoryMessages returns from multiple streams
func TestMDB001_5A_T8_GetCategoryMessages_MultipleStreams(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r3", "hash_r3", "Test namespace r3")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r3")

	// Write messages to multiple streams in same category
	writeTestMessages(t, store, "test_ns_r3", "account-123", 3)
	writeTestMessages(t, store, "test_ns_r3", "account-456", 2)
	writeTestMessages(t, store, "test_ns_r3", "account-789", 4)

	// Write messages to different category
	writeTestMessages(t, store, "test_ns_r3", "user-111", 2)

	// Read from "account" category
	opts := storepkg.NewCategoryOpts()
	messages, err := store.GetCategoryMessages(ctx, "test_ns_r3", "account", opts)
	if err != nil {
		t.Fatalf("Failed to read category messages: %v", err)
	}

	// Should get all 9 messages from account category
	if len(messages) != 9 {
		t.Errorf("Expected 9 messages from account category, got %d", len(messages))
	}

	// Verify messages are from account category
	accountCount := 0
	for _, msg := range messages {
		category := store.Category(msg.StreamName)
		if category != "account" {
			t.Errorf("Expected category 'account', got '%s'", category)
		}
		accountCount++
	}

	if accountCount != 9 {
		t.Errorf("Expected 9 account messages, got %d", accountCount)
	}

	// Verify messages are ordered by global_position
	for i := 1; i < len(messages); i++ {
		if messages[i].GlobalPosition <= messages[i-1].GlobalPosition {
			t.Errorf("Expected increasing global positions, got %d -> %d",
				messages[i-1].GlobalPosition, messages[i].GlobalPosition)
		}
	}
}

// MDB001_5A_T9: Test GetCategoryMessages with consumer groups
func TestMDB001_5A_T9_GetCategoryMessages_ConsumerGroups(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r4", "hash_r4", "Test namespace r4")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r4")

	// Write messages to multiple streams
	// Use streams with different IDs to test partitioning
	streams := []string{
		"account-111",
		"account-222",
		"account-333",
		"account-444",
		"account-555",
	}

	for _, stream := range streams {
		writeTestMessages(t, store, "test_ns_r4", stream, 2)
	}

	// Consumer group: 2 members
	consumerSize := int64(2)

	// Read for member 0
	member0 := int64(0)
	opts0 := &storepkg.CategoryOpts{
		Position:       1,
		BatchSize:      1000,
		ConsumerMember: &member0,
		ConsumerSize:   &consumerSize,
	}

	messages0, err := store.GetCategoryMessages(ctx, "test_ns_r4", "account", opts0)
	if err != nil {
		t.Fatalf("Failed to read messages for member 0: %v", err)
	}

	// Read for member 1
	member1 := int64(1)
	opts1 := &storepkg.CategoryOpts{
		Position:       1,
		BatchSize:      1000,
		ConsumerMember: &member1,
		ConsumerSize:   &consumerSize,
	}

	messages1, err := store.GetCategoryMessages(ctx, "test_ns_r4", "account", opts1)
	if err != nil {
		t.Fatalf("Failed to read messages for member 1: %v", err)
	}

	// Verify no overlap between members
	totalMessages := len(messages0) + len(messages1)
	expectedTotal := 10 // 5 streams × 2 messages each

	if totalMessages != expectedTotal {
		t.Errorf("Expected total %d messages across both members, got %d", expectedTotal, totalMessages)
	}

	// Verify messages are correctly partitioned
	for _, msg := range messages0 {
		if !storepkg.IsAssignedToConsumerMember(msg.StreamName, member0, consumerSize) {
			t.Errorf("Message %s should not be assigned to member 0", msg.StreamName)
		}
	}

	for _, msg := range messages1 {
		if !storepkg.IsAssignedToConsumerMember(msg.StreamName, member1, consumerSize) {
			t.Errorf("Message %s should not be assigned to member 1", msg.StreamName)
		}
	}

	t.Logf("Member 0 got %d messages, Member 1 got %d messages", len(messages0), len(messages1))
}

// MDB001_5A_T10: Test GetCategoryMessages hash partitioning matches Postgres
func TestMDB001_5A_T10_GetCategoryMessages_HashPartitioning(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r5", "hash_r5", "Test namespace r5")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r5")

	// Test compound IDs (cardinal ID extraction)
	compoundStreams := []string{
		"account-123+abc",
		"account-123+def",
		"account-456+ghi",
		"account-456+jkl",
	}

	for _, stream := range compoundStreams {
		writeTestMessages(t, store, "test_ns_r5", stream, 1)
	}

	// Consumer group: 2 members
	consumerSize := int64(2)
	member0 := int64(0)

	opts := &storepkg.CategoryOpts{
		Position:       1,
		BatchSize:      1000,
		ConsumerMember: &member0,
		ConsumerSize:   &consumerSize,
	}

	messages, err := store.GetCategoryMessages(ctx, "test_ns_r5", "account", opts)
	if err != nil {
		t.Fatalf("Failed to read messages: %v", err)
	}

	// Verify compound IDs with same cardinal ID are assigned to same member
	streamsByCardinalID := make(map[string][]string)
	for _, msg := range messages {
		cardinalID := store.CardinalID(msg.StreamName)
		streamsByCardinalID[cardinalID] = append(streamsByCardinalID[cardinalID], msg.StreamName)
	}

	// All messages with cardinal ID "123" should be together
	// All messages with cardinal ID "456" should be together
	for cardinalID, streams := range streamsByCardinalID {
		t.Logf("Cardinal ID %s: %v", cardinalID, streams)

		// Verify all streams with this cardinal ID are assigned to same member
		expectedMember := storepkg.Hash64(cardinalID) % consumerSize
		if expectedMember < 0 {
			expectedMember = -expectedMember
		}

		if expectedMember == member0 {
			// All streams with this cardinal ID should be in the results
			if cardinalID == "123" {
				// Should have both account-123+abc and account-123+def
				if len(streams) != 2 {
					t.Errorf("Expected 2 streams for cardinal ID 123, got %d", len(streams))
				}
			} else if cardinalID == "456" {
				// Should have both account-456+ghi and account-456+jkl
				if len(streams) != 2 {
					t.Errorf("Expected 2 streams for cardinal ID 456, got %d", len(streams))
				}
			}
		}
	}
}

// MDB001_5A_T11: Test GetLastStreamMessage returns last
func TestMDB001_5A_T11_GetLastStreamMessage_ReturnsLast(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r6", "hash_r6", "Test namespace r6")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r6")

	streamName := "account-last-test"

	// Write several messages
	writeTestMessages(t, store, "test_ns_r6", streamName, 5)

	// Get last message (no type filter)
	lastMsg, err := store.GetLastStreamMessage(ctx, "test_ns_r6", streamName, nil)
	if err != nil {
		t.Fatalf("Failed to get last message: %v", err)
	}

	if lastMsg == nil {
		t.Fatal("Expected last message, got nil")
	}

	// Last message should have position 4 (0-indexed, 5 messages)
	if lastMsg.Position != 4 {
		t.Errorf("Expected last message position 4, got %d", lastMsg.Position)
	}

	// Verify it's actually the last message
	if int(lastMsg.Data["index"].(float64)) != 4 {
		t.Errorf("Expected last message index 4, got %v", lastMsg.Data["index"])
	}

	// Test with non-existent stream
	emptyMsg, err := store.GetLastStreamMessage(ctx, "test_ns_r6", "nonexistent-stream", nil)
	if err != storepkg.ErrStreamNotFound {
		t.Fatalf("Expected ErrStreamNotFound for non-existent stream, got: %v", err)
	}

	if emptyMsg != nil {
		t.Errorf("Expected nil for non-existent stream, got message: %v", emptyMsg)
	}
}

// MDB001_5A_T12: Test GetLastStreamMessage with type filter
func TestMDB001_5A_T12_GetLastStreamMessage_WithTypeFilter(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r7", "hash_r7", "Test namespace r7")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r7")

	streamName := "account-type-filter"

	// Write messages with different types
	types := []string{"Created", "Updated", "Created", "Updated", "Closed"}
	for i, msgType := range types {
		msg := &storepkg.Message{
			StreamName: streamName,
			Type:       msgType,
			Data: map[string]interface{}{
				"index": i,
			},
		}

		_, err := store.WriteMessage(ctx, "test_ns_r7", streamName, msg)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Get last "Updated" message
	updatedType := "Updated"
	lastUpdated, err := store.GetLastStreamMessage(ctx, "test_ns_r7", streamName, &updatedType)
	if err != nil {
		t.Fatalf("Failed to get last Updated message: %v", err)
	}

	if lastUpdated == nil {
		t.Fatal("Expected last Updated message, got nil")
	}

	if lastUpdated.Type != "Updated" {
		t.Errorf("Expected type 'Updated', got '%s'", lastUpdated.Type)
	}

	// Should be the message at index 3 (position 3)
	if lastUpdated.Position != 3 {
		t.Errorf("Expected position 3, got %d", lastUpdated.Position)
	}

	// Get last "Created" message
	createdType := "Created"
	lastCreated, err := store.GetLastStreamMessage(ctx, "test_ns_r7", streamName, &createdType)
	if err != nil {
		t.Fatalf("Failed to get last Created message: %v", err)
	}

	if lastCreated == nil {
		t.Fatal("Expected last Created message, got nil")
	}

	if lastCreated.Type != "Created" {
		t.Errorf("Expected type 'Created', got '%s'", lastCreated.Type)
	}

	// Should be the message at index 2 (position 2)
	if lastCreated.Position != 2 {
		t.Errorf("Expected position 2, got %d", lastCreated.Position)
	}

	// Get last "Closed" message
	closedType := "Closed"
	lastClosed, err := store.GetLastStreamMessage(ctx, "test_ns_r7", streamName, &closedType)
	if err != nil {
		t.Fatalf("Failed to get last Closed message: %v", err)
	}

	if lastClosed == nil {
		t.Fatal("Expected last Closed message, got nil")
	}

	if lastClosed.Position != 4 {
		t.Errorf("Expected position 4, got %d", lastClosed.Position)
	}

	// Try to get non-existent type
	nonexistentType := "NonExistent"
	noMsg, err := store.GetLastStreamMessage(ctx, "test_ns_r7", streamName, &nonexistentType)
	if err != storepkg.ErrStreamNotFound {
		t.Fatalf("Expected ErrStreamNotFound for non-existent type, got: %v", err)
	}

	if noMsg != nil {
		t.Errorf("Expected nil for non-existent type, got message: %v", noMsg)
	}
}

// MDB001_5A_T13: Test GetStreamVersion returns correct version
func TestMDB001_5A_T13_GetStreamVersion_ReturnsCorrectVersion(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r8", "hash_r8", "Test namespace r8")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r8")

	streamName := "account-version-test"

	// Get version of non-existent stream
	version0, err := store.GetStreamVersion(ctx, "test_ns_r8", streamName)
	if err != nil {
		t.Fatalf("Failed to get version of non-existent stream: %v", err)
	}

	if version0 != -1 {
		t.Errorf("Expected version -1 for non-existent stream, got %d", version0)
	}

	// Write first message
	msg1 := &storepkg.Message{
		StreamName: streamName,
		Type:       "Created",
		Data:       map[string]interface{}{"index": 0},
	}

	_, err = store.WriteMessage(ctx, "test_ns_r8", streamName, msg1)
	if err != nil {
		t.Fatalf("Failed to write first message: %v", err)
	}

	// Get version after first message
	version1, err := store.GetStreamVersion(ctx, "test_ns_r8", streamName)
	if err != nil {
		t.Fatalf("Failed to get version after first message: %v", err)
	}

	if version1 != 0 {
		t.Errorf("Expected version 0 after first message, got %d", version1)
	}

	// Write 4 more messages
	writeTestMessages(t, store, "test_ns_r8", streamName, 4)

	// Get version after 5 messages total
	version5, err := store.GetStreamVersion(ctx, "test_ns_r8", streamName)
	if err != nil {
		t.Fatalf("Failed to get version after 5 messages: %v", err)
	}

	if version5 != 4 {
		t.Errorf("Expected version 4 after 5 messages, got %d", version5)
	}

	// Verify version matches position of last message
	lastMsg, err := store.GetLastStreamMessage(ctx, "test_ns_r8", streamName, nil)
	if err != nil {
		t.Fatalf("Failed to get last message: %v", err)
	}

	if lastMsg.Position != version5 {
		t.Errorf("Expected last message position to match version, got position=%d, version=%d",
			lastMsg.Position, version5)
	}
}

// MDB001_5A_T14: Test GetStreamMessages with GlobalPosition filter
func TestMDB001_5A_T14_GetStreamMessages_WithGlobalPosition(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r9", "hash_r9", "Test namespace r9")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r9")

	streamName := "account-gpos-test"

	// Write 10 test messages and track their global positions
	var globalPositions []int64
	for i := 0; i < 10; i++ {
		msg := &storepkg.Message{
			StreamName: streamName,
			Type:       "TestMessage",
			Data: map[string]interface{}{
				"index": i,
			},
		}

		result, err := store.WriteMessage(ctx, "test_ns_r9", streamName, msg)
		if err != nil {
			t.Fatalf("Failed to write test message %d: %v", i, err)
		}
		globalPositions = append(globalPositions, result.GlobalPosition)
	}

	// Get the global position of the 5th message (index 4)
	targetGPos := globalPositions[4]

	// Read messages from that global position
	opts := &storepkg.GetOpts{
		GlobalPosition: &targetGPos,
		BatchSize:      1000,
	}

	messages, err := store.GetStreamMessages(ctx, "test_ns_r9", streamName, opts)
	if err != nil {
		t.Fatalf("Failed to read messages with global position filter: %v", err)
	}

	// Should get messages from position 4 onwards (6 messages total)
	if len(messages) != 6 {
		t.Errorf("Expected 6 messages, got %d", len(messages))
	}

	// Verify all returned messages have global_position >= targetGPos
	for i, msg := range messages {
		if msg.GlobalPosition < targetGPos {
			t.Errorf("Message %d has global_position %d, expected >= %d",
				i, msg.GlobalPosition, targetGPos)
		}
	}

	// Verify first returned message has the target global position
	if messages[0].GlobalPosition != targetGPos {
		t.Errorf("First message should have global_position %d, got %d",
			targetGPos, messages[0].GlobalPosition)
	}
}

// MDB001_5A_T15: Test GetCategoryMessages with GlobalPosition filter
func TestMDB001_5A_T15_GetCategoryMessages_WithGlobalPosition(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_r10", "hash_r10", "Test namespace r10")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_r10")

	// Write messages to multiple streams and track global positions
	var allGlobalPositions []int64
	streams := []string{"account-111", "account-222", "account-333"}

	for _, stream := range streams {
		for i := 0; i < 3; i++ {
			msg := &storepkg.Message{
				StreamName: stream,
				Type:       "TestMessage",
				Data: map[string]interface{}{
					"stream": stream,
					"index":  i,
				},
			}

			result, err := store.WriteMessage(ctx, "test_ns_r10", stream, msg)
			if err != nil {
				t.Fatalf("Failed to write message: %v", err)
			}
			allGlobalPositions = append(allGlobalPositions, result.GlobalPosition)
		}
	}

	// Get the global position of the 5th message overall (index 4)
	targetGPos := allGlobalPositions[4]

	// Read category messages from that global position
	opts := &storepkg.CategoryOpts{
		GlobalPosition: &targetGPos,
		BatchSize:      1000,
	}

	messages, err := store.GetCategoryMessages(ctx, "test_ns_r10", "account", opts)
	if err != nil {
		t.Fatalf("Failed to read category messages with global position filter: %v", err)
	}

	// Should get messages from the 5th message onwards (5 messages remaining)
	expectedCount := 9 - 4 // Total 9 messages, skip first 4
	if len(messages) != expectedCount {
		t.Errorf("Expected %d messages, got %d", expectedCount, len(messages))
	}

	// Verify all returned messages have global_position >= targetGPos
	for i, msg := range messages {
		if msg.GlobalPosition < targetGPos {
			t.Errorf("Message %d has global_position %d, expected >= %d",
				i, msg.GlobalPosition, targetGPos)
		}
	}

	// Verify messages are still in global position order
	for i := 1; i < len(messages); i++ {
		if messages[i].GlobalPosition <= messages[i-1].GlobalPosition {
			t.Errorf("Expected increasing global positions, got %d -> %d",
				messages[i-1].GlobalPosition, messages[i].GlobalPosition)
		}
	}
}
