package sqlite

import (
	"context"
	"testing"
	"time"

	storepkg "github.com/message-db/message-db/internal/store"
)

// MDB001_5A_T1: Test WriteMessage writes to correct namespace DB
func TestMDB001_5A_T1_WriteMessage_WritesToCorrectNamespace(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_w1", "hash_w1", "Test namespace w1")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_w1")

	// Write message
	msg := &storepkg.Message{
		StreamName: "account-123",
		Type:       "AccountCreated",
		Data: map[string]interface{}{
			"name":  "Test Account",
			"email": "test@example.com",
		},
		Metadata: map[string]interface{}{
			"userId": "user-456",
		},
	}

	result, err := store.WriteMessage(ctx, "test_ns_w1", "account-123", msg)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	if result == nil {
		t.Fatal("Expected WriteResult, got nil")
	}

	if result.Position != 0 {
		t.Errorf("Expected first message position to be 0, got %d", result.Position)
	}

	if result.GlobalPosition <= 0 {
		t.Errorf("Expected global position > 0, got %d", result.GlobalPosition)
	}

	// Verify message was written by reading it back
	messages, err := store.GetStreamMessages(ctx, "test_ns_w1", "account-123", nil)
	if err != nil {
		t.Fatalf("Failed to read messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	readMsg := messages[0]
	if readMsg.StreamName != "account-123" {
		t.Errorf("Expected stream name 'account-123', got '%s'", readMsg.StreamName)
	}

	if readMsg.Type != "AccountCreated" {
		t.Errorf("Expected type 'AccountCreated', got '%s'", readMsg.Type)
	}

	if readMsg.Data["name"] != "Test Account" {
		t.Errorf("Expected name 'Test Account', got '%v'", readMsg.Data["name"])
	}
}

// MDB001_5A_T2: Test WriteMessage assigns correct position and global_position
func TestMDB001_5A_T2_WriteMessage_AssignsCorrectPositions(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_w2", "hash_w2", "Test namespace w2")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_w2")

	streamName := "account-456"

	// Write first message
	msg1 := &storepkg.Message{
		StreamName: streamName,
		Type:       "AccountCreated",
		Data:       map[string]interface{}{"name": "Account 1"},
	}

	result1, err := store.WriteMessage(ctx, "test_ns_w2", streamName, msg1)
	if err != nil {
		t.Fatalf("Failed to write first message: %v", err)
	}

	if result1.Position != 0 {
		t.Errorf("Expected first position to be 0, got %d", result1.Position)
	}

	// Write second message
	msg2 := &storepkg.Message{
		StreamName: streamName,
		Type:       "AccountUpdated",
		Data:       map[string]interface{}{"name": "Account 1 Updated"},
	}

	result2, err := store.WriteMessage(ctx, "test_ns_w2", streamName, msg2)
	if err != nil {
		t.Fatalf("Failed to write second message: %v", err)
	}

	if result2.Position != 1 {
		t.Errorf("Expected second position to be 1, got %d", result2.Position)
	}

	// Write third message
	msg3 := &storepkg.Message{
		StreamName: streamName,
		Type:       "AccountClosed",
		Data:       map[string]interface{}{"reason": "User request"},
	}

	result3, err := store.WriteMessage(ctx, "test_ns_w2", streamName, msg3)
	if err != nil {
		t.Fatalf("Failed to write third message: %v", err)
	}

	if result3.Position != 2 {
		t.Errorf("Expected third position to be 2, got %d", result3.Position)
	}

	// Verify global positions are monotonically increasing
	if result2.GlobalPosition <= result1.GlobalPosition {
		t.Errorf("Expected global position to increase: %d -> %d", result1.GlobalPosition, result2.GlobalPosition)
	}

	if result3.GlobalPosition <= result2.GlobalPosition {
		t.Errorf("Expected global position to increase: %d -> %d", result2.GlobalPosition, result3.GlobalPosition)
	}
}

// MDB001_5A_T3: Test WriteMessage with expected_version success
func TestMDB001_5A_T3_WriteMessage_ExpectedVersionSuccess(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_w3", "hash_w3", "Test namespace w3")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_w3")

	streamName := "account-789"

	// Write first message (no expected version)
	msg1 := &storepkg.Message{
		StreamName: streamName,
		Type:       "AccountCreated",
		Data:       map[string]interface{}{"name": "Account 789"},
	}

	result1, err := store.WriteMessage(ctx, "test_ns_w3", streamName, msg1)
	if err != nil {
		t.Fatalf("Failed to write first message: %v", err)
	}

	// Write second message with correct expected version
	expectedVersion := int64(0) // After first message, version should be 0
	msg2 := &storepkg.Message{
		StreamName:      streamName,
		Type:            "AccountUpdated",
		Data:            map[string]interface{}{"name": "Account 789 Updated"},
		ExpectedVersion: &expectedVersion,
	}

	result2, err := store.WriteMessage(ctx, "test_ns_w3", streamName, msg2)
	if err != nil {
		t.Fatalf("Failed to write second message with expected version: %v", err)
	}

	if result2.Position != 1 {
		t.Errorf("Expected position 1, got %d", result2.Position)
	}

	// Write to new stream with expected version -1 (stream doesn't exist)
	newStreamName := "account-999"
	expectedVersionNew := int64(-1)
	msg3 := &storepkg.Message{
		StreamName:      newStreamName,
		Type:            "AccountCreated",
		Data:            map[string]interface{}{"name": "New Account"},
		ExpectedVersion: &expectedVersionNew,
	}

	result3, err := store.WriteMessage(ctx, "test_ns_w3", newStreamName, msg3)
	if err != nil {
		t.Fatalf("Failed to write to new stream with expected version -1: %v", err)
	}

	if result3.Position != 0 {
		t.Errorf("Expected position 0 for new stream, got %d", result3.Position)
	}

	// Verify positions
	t.Logf("Message 1: Position=%d, GlobalPosition=%d", result1.Position, result1.GlobalPosition)
	t.Logf("Message 2: Position=%d, GlobalPosition=%d", result2.Position, result2.GlobalPosition)
	t.Logf("Message 3: Position=%d, GlobalPosition=%d", result3.Position, result3.GlobalPosition)
}

// MDB001_5A_T4: Test WriteMessage with expected_version conflict
func TestMDB001_5A_T4_WriteMessage_ExpectedVersionConflict(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_w4", "hash_w4", "Test namespace w4")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_w4")

	streamName := "account-conflict"

	// Write first message
	msg1 := &storepkg.Message{
		StreamName: streamName,
		Type:       "AccountCreated",
		Data:       map[string]interface{}{"name": "Account"},
	}

	_, err = store.WriteMessage(ctx, "test_ns_w4", streamName, msg1)
	if err != nil {
		t.Fatalf("Failed to write first message: %v", err)
	}

	// Try to write with wrong expected version (should be 0, but we say 5)
	wrongVersion := int64(5)
	msg2 := &storepkg.Message{
		StreamName:      streamName,
		Type:            "AccountUpdated",
		Data:            map[string]interface{}{"name": "Updated"},
		ExpectedVersion: &wrongVersion,
	}

	_, err = store.WriteMessage(ctx, "test_ns_w4", streamName, msg2)
	if err == nil {
		t.Fatal("Expected version conflict error, got nil")
	}

	if !storepkg.IsVersionConflict(err) {
		t.Errorf("Expected version conflict error, got: %v", err)
	}

	// Verify stream still has only 1 message
	messages, err := store.GetStreamMessages(ctx, "test_ns_w4", streamName, nil)
	if err != nil {
		t.Fatalf("Failed to read messages: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message after conflict, got %d", len(messages))
	}

	// Try to write to non-existent stream with wrong expected version
	newStreamName := "account-nonexistent"
	wrongVersionNew := int64(10)
	msg3 := &storepkg.Message{
		StreamName:      newStreamName,
		Type:            "AccountCreated",
		Data:            map[string]interface{}{"name": "New"},
		ExpectedVersion: &wrongVersionNew,
	}

	_, err = store.WriteMessage(ctx, "test_ns_w4", newStreamName, msg3)
	if err == nil {
		t.Fatal("Expected version conflict error for new stream, got nil")
	}

	if !storepkg.IsVersionConflict(err) {
		t.Errorf("Expected version conflict error for new stream, got: %v", err)
	}
}

// MDB001_5A_T5: Test WriteMessage serializes JSON correctly
func TestMDB001_5A_T5_WriteMessage_SerializesJSON(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_w5", "hash_w5", "Test namespace w5")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_w5")

	streamName := "account-json"

	// Write message with complex JSON data
	msg := &storepkg.Message{
		StreamName: streamName,
		Type:       "AccountCreated",
		Data: map[string]interface{}{
			"name":   "Complex Account",
			"email":  "complex@example.com",
			"age":    30,
			"active": true,
			"tags":   []interface{}{"premium", "verified"},
			"address": map[string]interface{}{
				"street": "123 Main St",
				"city":   "Springfield",
				"zip":    "12345",
			},
		},
		Metadata: map[string]interface{}{
			"userId":                 "user-123",
			"correlationStreamName":  "user-123",
			"timestamp":              time.Now().Unix(),
			"version":                1,
		},
	}

	result, err := store.WriteMessage(ctx, "test_ns_w5", streamName, msg)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Read back and verify
	messages, err := store.GetStreamMessages(ctx, "test_ns_w5", streamName, nil)
	if err != nil {
		t.Fatalf("Failed to read messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	readMsg := messages[0]

	// Verify data fields
	if readMsg.Data["name"] != "Complex Account" {
		t.Errorf("Expected name 'Complex Account', got '%v'", readMsg.Data["name"])
	}

	if readMsg.Data["age"].(float64) != 30 {
		t.Errorf("Expected age 30, got %v", readMsg.Data["age"])
	}

	if readMsg.Data["active"] != true {
		t.Errorf("Expected active true, got %v", readMsg.Data["active"])
	}

	// Verify nested objects
	address := readMsg.Data["address"].(map[string]interface{})
	if address["city"] != "Springfield" {
		t.Errorf("Expected city 'Springfield', got '%v'", address["city"])
	}

	// Verify arrays
	tags := readMsg.Data["tags"].([]interface{})
	if len(tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tags))
	}

	// Verify metadata
	if readMsg.Metadata["userId"] != "user-123" {
		t.Errorf("Expected userId 'user-123', got '%v'", readMsg.Metadata["userId"])
	}

	if readMsg.Metadata["correlationStreamName"] != "user-123" {
		t.Errorf("Expected correlationStreamName 'user-123', got '%v'", readMsg.Metadata["correlationStreamName"])
	}

	// Verify positions
	if result.Position != 0 {
		t.Errorf("Expected position 0, got %d", result.Position)
	}

	// Test with nil data and metadata
	msg2 := &storepkg.Message{
		StreamName: streamName,
		Type:       "AccountUpdated",
		Data:       nil,
		Metadata:   nil,
	}

	_, err = store.WriteMessage(ctx, "test_ns_w5", streamName, msg2)
	if err != nil {
		t.Fatalf("Failed to write message with nil data/metadata: %v", err)
	}

	// Verify it was written
	messages, err = store.GetStreamMessages(ctx, "test_ns_w5", streamName, nil)
	if err != nil {
		t.Fatalf("Failed to read messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}
}

// Helper function to check if error is version conflict
func isVersionConflict(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*storepkg.VersionConflictError)
	return ok || err == storepkg.ErrVersionConflict
}
