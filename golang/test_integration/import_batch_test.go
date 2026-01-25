package integration

import (
	"context"
	"testing"
	"time"

	"github.com/eventodb/eventodb/internal/store"
	"github.com/google/uuid"
)

// TestMDB004_1A_T1_ImportBatch_PreservesPositions_SQLite tests that ImportBatch preserves positions in SQLite
func TestMDB004_1A_T1_ImportBatch_PreservesPositions_SQLite(t *testing.T) {
	env := SetupTestEnvWithBackend(t, BackendSQLite)
	defer env.Cleanup()
	testImportBatchPreservesPositions(t, env)
}

// TestMDB004_1A_T2_ImportBatch_PreservesPositions_Postgres tests that ImportBatch preserves positions in PostgreSQL
func TestMDB004_1A_T2_ImportBatch_PreservesPositions_Postgres(t *testing.T) {
	if GetTestBackend() != BackendPostgres {
		t.Skip("PostgreSQL not available")
	}
	env := SetupTestEnvWithBackend(t, BackendPostgres)
	defer env.Cleanup()
	testImportBatchPreservesPositions(t, env)
}

// TestMDB004_1A_T3_ImportBatch_PreservesPositions_Pebble tests that ImportBatch preserves positions in Pebble
func TestMDB004_1A_T3_ImportBatch_PreservesPositions_Pebble(t *testing.T) {
	env := SetupTestEnvWithBackend(t, BackendPebble)
	defer env.Cleanup()
	testImportBatchPreservesPositions(t, env)
}

// TestMDB004_1A_T4_ImportBatch_RejectsDuplicate_SQLite tests duplicate rejection in SQLite
func TestMDB004_1A_T4_ImportBatch_RejectsDuplicate_SQLite(t *testing.T) {
	env := SetupTestEnvWithBackend(t, BackendSQLite)
	defer env.Cleanup()
	testImportBatchRejectsDuplicate(t, env)
}

// TestMDB004_1A_T5_ImportBatch_RejectsDuplicate_Postgres tests duplicate rejection in PostgreSQL
func TestMDB004_1A_T5_ImportBatch_RejectsDuplicate_Postgres(t *testing.T) {
	if GetTestBackend() != BackendPostgres {
		t.Skip("PostgreSQL not available")
	}
	env := SetupTestEnvWithBackend(t, BackendPostgres)
	defer env.Cleanup()
	testImportBatchRejectsDuplicate(t, env)
}

// TestMDB004_1A_T6_ImportBatch_RejectsDuplicate_Pebble tests duplicate rejection in Pebble
func TestMDB004_1A_T6_ImportBatch_RejectsDuplicate_Pebble(t *testing.T) {
	env := SetupTestEnvWithBackend(t, BackendPebble)
	defer env.Cleanup()
	testImportBatchRejectsDuplicate(t, env)
}

// TestMDB004_1A_T7_ImportBatch_HandlesGaps tests importing with sparse/gapped positions
func TestMDB004_1A_T7_ImportBatch_HandlesGaps(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Import messages with gaps in global position
	messages := []*store.Message{
		{
			ID:             uuid.New().String(),
			StreamName:     "account-123",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 10, // Gap from 0
			Data:           map[string]interface{}{"name": "test"},
			Time:           time.Now().UTC(),
		},
		{
			ID:             uuid.New().String(),
			StreamName:     "account-123",
			Type:           "Updated",
			Position:       1,
			GlobalPosition: 50, // Gap from 10
			Data:           map[string]interface{}{"name": "updated"},
			Time:           time.Now().UTC(),
		},
		{
			ID:             uuid.New().String(),
			StreamName:     "account-456",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 100, // Gap from 50
			Data:           map[string]interface{}{"name": "another"},
			Time:           time.Now().UTC(),
		},
	}

	err := env.Store.ImportBatch(ctx, env.Namespace, messages)
	if err != nil {
		t.Fatalf("ImportBatch failed: %v", err)
	}

	// Verify messages can be retrieved with correct positions
	msgs, err := env.Store.GetStreamMessages(ctx, env.Namespace, "account-123", nil)
	if err != nil {
		t.Fatalf("GetStreamMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages in stream, got %d", len(msgs))
	}
	if msgs[0].GlobalPosition != 10 {
		t.Errorf("Expected first message gpos=10, got %d", msgs[0].GlobalPosition)
	}
	if msgs[1].GlobalPosition != 50 {
		t.Errorf("Expected second message gpos=50, got %d", msgs[1].GlobalPosition)
	}

	// Verify category messages return in global position order
	catMsgs, err := env.Store.GetCategoryMessages(ctx, env.Namespace, "account", nil)
	if err != nil {
		t.Fatalf("GetCategoryMessages failed: %v", err)
	}
	if len(catMsgs) != 3 {
		t.Errorf("Expected 3 messages in category, got %d", len(catMsgs))
	}
	if catMsgs[0].GlobalPosition != 10 {
		t.Errorf("Expected first category message gpos=10, got %d", catMsgs[0].GlobalPosition)
	}
	if catMsgs[1].GlobalPosition != 50 {
		t.Errorf("Expected second category message gpos=50, got %d", catMsgs[1].GlobalPosition)
	}
	if catMsgs[2].GlobalPosition != 100 {
		t.Errorf("Expected third category message gpos=100, got %d", catMsgs[2].GlobalPosition)
	}
}

// TestMDB004_1A_T8_ImportBatch_TransactionRollback tests that entire batch fails atomically
func TestMDB004_1A_T8_ImportBatch_TransactionRollback(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	ctx := context.Background()

	// First import some messages
	initialMessages := []*store.Message{
		{
			ID:             uuid.New().String(),
			StreamName:     "order-100",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 5,
			Data:           map[string]interface{}{"amount": 100},
			Time:           time.Now().UTC(),
		},
	}
	err := env.Store.ImportBatch(ctx, env.Namespace, initialMessages)
	if err != nil {
		t.Fatalf("Initial ImportBatch failed: %v", err)
	}

	// Now try to import a batch where the second message has a duplicate position
	badBatch := []*store.Message{
		{
			ID:             uuid.New().String(),
			StreamName:     "order-200",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 10, // This should work
			Data:           map[string]interface{}{"amount": 200},
			Time:           time.Now().UTC(),
		},
		{
			ID:             uuid.New().String(),
			StreamName:     "order-300",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 5, // This duplicates existing gpos=5
			Data:           map[string]interface{}{"amount": 300},
			Time:           time.Now().UTC(),
		},
	}

	err = env.Store.ImportBatch(ctx, env.Namespace, badBatch)
	if err == nil {
		t.Fatal("Expected error for duplicate position, got nil")
	}

	// Verify the first message from the failed batch was NOT inserted (rollback)
	msgs, err := env.Store.GetStreamMessages(ctx, env.Namespace, "order-200", nil)
	if err != nil && err != store.ErrStreamNotFound {
		t.Fatalf("GetStreamMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Expected no messages in order-200 stream after rollback, got %d", len(msgs))
	}

	// Verify original message still exists
	origMsgs, err := env.Store.GetStreamMessages(ctx, env.Namespace, "order-100", nil)
	if err != nil {
		t.Fatalf("GetStreamMessages for original failed: %v", err)
	}
	if len(origMsgs) != 1 {
		t.Errorf("Expected 1 original message, got %d", len(origMsgs))
	}
}

// testImportBatchPreservesPositions is a shared test for preserving positions
func testImportBatchPreservesPositions(t *testing.T, env *TestEnv) {
	t.Helper()

	ctx := context.Background()

	// Create messages with specific positions
	messages := []*store.Message{
		{
			ID:             uuid.New().String(),
			StreamName:     "workflow-123",
			Type:           "TaskRequested",
			Position:       0,
			GlobalPosition: 47,
			Data:           map[string]interface{}{"task": "process"},
			Metadata:       nil,
			Time:           time.Now().UTC(),
		},
		{
			ID:             uuid.New().String(),
			StreamName:     "order-456",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 52,
			Data:           map[string]interface{}{"amount": 100},
			Metadata:       map[string]interface{}{"correlationStreamName": "workflow-123"},
			Time:           time.Now().UTC(),
		},
		{
			ID:             uuid.New().String(),
			StreamName:     "workflow-123",
			Type:           "TaskCompleted",
			Position:       1,
			GlobalPosition: 89,
			Data:           map[string]interface{}{"result": "done"},
			Time:           time.Now().UTC(),
		},
	}

	err := env.Store.ImportBatch(ctx, env.Namespace, messages)
	if err != nil {
		t.Fatalf("ImportBatch failed: %v", err)
	}

	// Verify positions are preserved
	workflowMsgs, err := env.Store.GetStreamMessages(ctx, env.Namespace, "workflow-123", nil)
	if err != nil {
		t.Fatalf("GetStreamMessages failed: %v", err)
	}

	if len(workflowMsgs) != 2 {
		t.Fatalf("Expected 2 workflow messages, got %d", len(workflowMsgs))
	}

	if workflowMsgs[0].Position != 0 || workflowMsgs[0].GlobalPosition != 47 {
		t.Errorf("First message: expected pos=0, gpos=47, got pos=%d, gpos=%d",
			workflowMsgs[0].Position, workflowMsgs[0].GlobalPosition)
	}

	if workflowMsgs[1].Position != 1 || workflowMsgs[1].GlobalPosition != 89 {
		t.Errorf("Second message: expected pos=1, gpos=89, got pos=%d, gpos=%d",
			workflowMsgs[1].Position, workflowMsgs[1].GlobalPosition)
	}

	// Verify order messages
	orderMsgs, err := env.Store.GetStreamMessages(ctx, env.Namespace, "order-456", nil)
	if err != nil {
		t.Fatalf("GetStreamMessages for order failed: %v", err)
	}

	if len(orderMsgs) != 1 {
		t.Fatalf("Expected 1 order message, got %d", len(orderMsgs))
	}

	if orderMsgs[0].GlobalPosition != 52 {
		t.Errorf("Order message: expected gpos=52, got %d", orderMsgs[0].GlobalPosition)
	}

	// Verify data is preserved
	if workflowMsgs[0].Type != "TaskRequested" {
		t.Errorf("Expected type TaskRequested, got %s", workflowMsgs[0].Type)
	}
	if workflowMsgs[0].Data["task"] != "process" {
		t.Errorf("Expected data.task='process', got %v", workflowMsgs[0].Data["task"])
	}
}

// testImportBatchRejectsDuplicate is a shared test for rejecting duplicate positions
func testImportBatchRejectsDuplicate(t *testing.T, env *TestEnv) {
	t.Helper()

	ctx := context.Background()

	// First import
	messages := []*store.Message{
		{
			ID:             uuid.New().String(),
			StreamName:     "test-stream",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 47,
			Data:           map[string]interface{}{"value": 1},
			Time:           time.Now().UTC(),
		},
	}

	err := env.Store.ImportBatch(ctx, env.Namespace, messages)
	if err != nil {
		t.Fatalf("First ImportBatch failed: %v", err)
	}

	// Second import with same global position
	duplicateMessages := []*store.Message{
		{
			ID:             uuid.New().String(),
			StreamName:     "another-stream",
			Type:           "Created",
			Position:       0,
			GlobalPosition: 47, // Duplicate!
			Data:           map[string]interface{}{"value": 2},
			Time:           time.Now().UTC(),
		},
	}

	err = env.Store.ImportBatch(ctx, env.Namespace, duplicateMessages)
	if err == nil {
		t.Fatal("Expected ErrPositionExists, got nil")
	}

	// Verify error type (should wrap ErrPositionExists)
	if !contains(err.Error(), "already exists") && !contains(err.Error(), "47") {
		t.Errorf("Expected error about position 47 already existing, got: %v", err)
	}
}

// TestMDB004_1A_ImportBatch_EmptyBatch tests that empty batch succeeds without error
func TestMDB004_1A_ImportBatch_EmptyBatch(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	ctx := context.Background()
	err := env.Store.ImportBatch(ctx, env.Namespace, []*store.Message{})
	if err != nil {
		t.Errorf("ImportBatch with empty batch should succeed, got: %v", err)
	}
}

// TestMDB004_1A_ImportBatch_WritesAfterImport tests that normal writes work after import
func TestMDB004_1A_ImportBatch_WritesAfterImport(t *testing.T) {
	env := SetupTestEnv(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Import messages with high global positions
	messages := []*store.Message{
		{
			ID:             uuid.New().String(),
			StreamName:     "stream-A",
			Type:           "Imported",
			Position:       0,
			GlobalPosition: 100,
			Data:           map[string]interface{}{"imported": true},
			Time:           time.Now().UTC(),
		},
	}

	err := env.Store.ImportBatch(ctx, env.Namespace, messages)
	if err != nil {
		t.Fatalf("ImportBatch failed: %v", err)
	}

	// Now write a new message normally
	newMsg := &store.Message{
		Type: "Regular",
		Data: map[string]interface{}{"imported": false},
	}
	result, err := env.Store.WriteMessage(ctx, env.Namespace, "stream-B", newMsg)
	if err != nil {
		t.Fatalf("WriteMessage after import failed: %v", err)
	}

	// The new message should have a global position > 100
	if result.GlobalPosition <= 100 {
		t.Errorf("Expected new message gpos > 100, got %d", result.GlobalPosition)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
