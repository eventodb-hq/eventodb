package postgres

import (
	"context"
	"testing"

	"github.com/eventodb/eventodb/internal/store"
)

// MDB001_3A_T1: Test WriteMessage writes to correct schema
func TestMDB001_3A_T1_WriteMessageWritesToCorrectSchema(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-1"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-1", "Test Namespace 1")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Write a message
	msg := &store.Message{
		StreamName: "account-123",
		Type:       "AccountOpened",
		Data: map[string]interface{}{
			"accountId": "123",
			"name":      "Test Account",
		},
	}

	result, err := pgStore.WriteMessage(ctx, namespace, msg.StreamName, msg)
	if err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	if result.Position != 0 {
		t.Errorf("expected position 0, got %d", result.Position)
	}

	if result.GlobalPosition <= 0 {
		t.Errorf("expected positive global position, got %d", result.GlobalPosition)
	}

	// Verify message ID was generated
	if msg.ID == "" {
		t.Error("expected message ID to be generated")
	}
}

// MDB001_3A_T2: Test WriteMessage assigns correct position and global_position
func TestMDB001_3A_T2_WriteMessageAssignsCorrectPositions(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-2"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-2", "Test Namespace 2")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "account-456"

	// Write multiple messages
	for i := 0; i < 5; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "AccountUpdated",
			Data: map[string]interface{}{
				"sequence": i,
			},
		}

		result, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}

		if result.Position != int64(i) {
			t.Errorf("message %d: expected position %d, got %d", i, i, result.Position)
		}

		if result.GlobalPosition <= 0 {
			t.Errorf("message %d: expected positive global position, got %d", i, result.GlobalPosition)
		}
	}
}

// MDB001_3A_T3: Test WriteMessage with expected_version success
func TestMDB001_3A_T3_WriteMessageWithExpectedVersionSuccess(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-3"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-3", "Test Namespace 3")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "account-789"

	// Write first message with expected version -1 (empty stream)
	expectedVersion := int64(-1)
	msg1 := &store.Message{
		StreamName:      streamName,
		Type:            "AccountOpened",
		Data:            map[string]interface{}{"id": "789"},
		ExpectedVersion: &expectedVersion,
	}

	result1, err := pgStore.WriteMessage(ctx, namespace, streamName, msg1)
	if err != nil {
		t.Fatalf("failed to write first message: %v", err)
	}

	if result1.Position != 0 {
		t.Errorf("expected position 0, got %d", result1.Position)
	}

	// Write second message with expected version 0
	expectedVersion = 0
	msg2 := &store.Message{
		StreamName:      streamName,
		Type:            "AccountUpdated",
		Data:            map[string]interface{}{"status": "active"},
		ExpectedVersion: &expectedVersion,
	}

	result2, err := pgStore.WriteMessage(ctx, namespace, streamName, msg2)
	if err != nil {
		t.Fatalf("failed to write second message: %v", err)
	}

	if result2.Position != 1 {
		t.Errorf("expected position 1, got %d", result2.Position)
	}
}

// MDB001_3A_T4: Test WriteMessage with expected_version conflict
func TestMDB001_3A_T4_WriteMessageWithExpectedVersionConflict(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-4"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-4", "Test Namespace 4")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "account-999"

	// Write first message
	msg1 := &store.Message{
		StreamName: streamName,
		Type:       "AccountOpened",
		Data:       map[string]interface{}{"id": "999"},
	}

	_, err = pgStore.WriteMessage(ctx, namespace, streamName, msg1)
	if err != nil {
		t.Fatalf("failed to write first message: %v", err)
	}

	// Try to write with wrong expected version
	wrongVersion := int64(5)
	msg2 := &store.Message{
		StreamName:      streamName,
		Type:            "AccountUpdated",
		Data:            map[string]interface{}{"status": "active"},
		ExpectedVersion: &wrongVersion,
	}

	_, err = pgStore.WriteMessage(ctx, namespace, streamName, msg2)
	if err != store.ErrVersionConflict {
		t.Errorf("expected ErrVersionConflict, got: %v", err)
	}
}

// MDB001_3A_T5: Test WriteMessage increments position correctly
func TestMDB001_3A_T5_WriteMessageIncrementsPositionCorrectly(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	pgStore, err := New(db)
	if err != nil {
		t.Fatalf("failed to create postgres store: %v", err)
	}
	defer pgStore.Close()

	ctx := context.Background()
	namespace := "test-ns-5"
	cleanupNamespace(t, pgStore, namespace)
	defer cleanupNamespace(t, pgStore, namespace)

	// Create namespace
	err = pgStore.CreateNamespace(ctx, namespace, "token-hash-5", "Test Namespace 5")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	streamName := "account-increment"

	// Write 10 messages and verify position increments
	for i := 0; i < 10; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "TestEvent",
			Data:       map[string]interface{}{"sequence": i},
		}

		result, err := pgStore.WriteMessage(ctx, namespace, streamName, msg)
		if err != nil {
			t.Fatalf("failed to write message %d: %v", i, err)
		}

		if result.Position != int64(i) {
			t.Errorf("message %d: expected position %d, got %d", i, i, result.Position)
		}
	}

	// Verify stream version
	version, err := pgStore.GetStreamVersion(ctx, namespace, streamName)
	if err != nil {
		t.Fatalf("failed to get stream version: %v", err)
	}

	if version != 9 {
		t.Errorf("expected stream version 9, got %d", version)
	}
}
