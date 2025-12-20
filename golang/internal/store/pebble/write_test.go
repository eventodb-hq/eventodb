package pebble

import (
	"context"
	"testing"

	"github.com/eventodb/eventodb/internal/store"
)

func TestWriteMessage_SingleStream(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Write first message
	msg1 := &store.Message{
		Type: "AccountCreated",
		Data: map[string]interface{}{"balance": 0},
	}
	result1, err := st.WriteMessage(ctx, "test", "account-123", msg1)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	if result1.Position != 0 {
		t.Errorf("Position = %d, want 0", result1.Position)
	}
	if result1.GlobalPosition != 1 {
		t.Errorf("GlobalPosition = %d, want 1", result1.GlobalPosition)
	}
	if msg1.ID == "" {
		t.Error("Message ID should be auto-generated")
	}

	// Write second message
	msg2 := &store.Message{
		Type: "BalanceUpdated",
		Data: map[string]interface{}{"balance": 100},
	}
	result2, err := st.WriteMessage(ctx, "test", "account-123", msg2)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	if result2.Position != 1 {
		t.Errorf("Position = %d, want 1", result2.Position)
	}
	if result2.GlobalPosition != 2 {
		t.Errorf("GlobalPosition = %d, want 2", result2.GlobalPosition)
	}
}

func TestWriteMessage_MultipleStreams(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Write to stream 1
	msg1 := &store.Message{Type: "Created", Data: map[string]interface{}{}}
	result1, err := st.WriteMessage(ctx, "test", "account-123", msg1)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	// Write to stream 2
	msg2 := &store.Message{Type: "Created", Data: map[string]interface{}{}}
	result2, err := st.WriteMessage(ctx, "test", "account-456", msg2)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	// Both should have position 0 (first in their streams)
	if result1.Position != 0 {
		t.Errorf("Stream 1 position = %d, want 0", result1.Position)
	}
	if result2.Position != 0 {
		t.Errorf("Stream 2 position = %d, want 0", result2.Position)
	}

	// But different global positions
	if result1.GlobalPosition != 1 {
		t.Errorf("Stream 1 global position = %d, want 1", result1.GlobalPosition)
	}
	if result2.GlobalPosition != 2 {
		t.Errorf("Stream 2 global position = %d, want 2", result2.GlobalPosition)
	}
}

func TestWriteMessage_OptimisticLocking(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Write first message
	msg1 := &store.Message{Type: "Created", Data: map[string]interface{}{}}
	_, err = st.WriteMessage(ctx, "test", "account-123", msg1)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	// Try to write with wrong expected version
	wrongVersion := int64(5)
	msg2 := &store.Message{
		Type:            "Updated",
		Data:            map[string]interface{}{},
		ExpectedVersion: &wrongVersion,
	}
	_, err = st.WriteMessage(ctx, "test", "account-123", msg2)
	if err != store.ErrVersionConflict {
		t.Errorf("expected ErrVersionConflict, got %v", err)
	}

	// Try with correct expected version
	correctVersion := int64(0)
	msg3 := &store.Message{
		Type:            "Updated",
		Data:            map[string]interface{}{},
		ExpectedVersion: &correctVersion,
	}
	result, err := st.WriteMessage(ctx, "test", "account-123", msg3)
	if err != nil {
		t.Fatalf("WriteMessage with correct version failed: %v", err)
	}
	if result.Position != 1 {
		t.Errorf("Position = %d, want 1", result.Position)
	}
}

func TestWriteMessage_EmptyStream(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Try to write with empty stream name
	msg := &store.Message{Type: "Created", Data: map[string]interface{}{}}
	_, err = st.WriteMessage(ctx, "test", "", msg)
	if err == nil {
		t.Error("expected error for empty stream name, got nil")
	}
}

func TestWriteMessage_InvalidNamespace(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Try to write to non-existent namespace
	msg := &store.Message{Type: "Created", Data: map[string]interface{}{}}
	_, err = st.WriteMessage(ctx, "nonexistent", "account-123", msg)
	if err == nil {
		t.Error("expected error for non-existent namespace, got nil")
	}
}

func TestGetNamespaceDB_LazyLoad(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Namespace DB should not be open yet
	st.mu.RLock()
	_, exists := st.namespaces["test"]
	st.mu.RUnlock()

	if exists {
		t.Error("namespace DB should not be open yet")
	}

	// Write message (should trigger lazy load)
	msg := &store.Message{Type: "Created", Data: map[string]interface{}{}}
	_, err = st.WriteMessage(ctx, "test", "account-123", msg)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	// Now namespace DB should be open
	st.mu.RLock()
	_, exists = st.namespaces["test"]
	st.mu.RUnlock()

	if !exists {
		t.Error("namespace DB should be open after write")
	}
}

func TestGetNamespaceDB_Caching(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Get handle twice
	handle1, err := st.getNamespaceDB(ctx, "test")
	if err != nil {
		t.Fatalf("getNamespaceDB failed: %v", err)
	}

	handle2, err := st.getNamespaceDB(ctx, "test")
	if err != nil {
		t.Fatalf("getNamespaceDB failed: %v", err)
	}

	// Should be the same handle
	if handle1 != handle2 {
		t.Error("getNamespaceDB should return cached handle")
	}
}

func TestWriteMessage_Atomicity(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Write message
	msg := &store.Message{Type: "Created", Data: map[string]interface{}{"balance": 0}}
	result, err := st.WriteMessage(ctx, "test", "account-123", msg)
	if err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	// Get namespace handle
	handle, err := st.getNamespaceDB(ctx, "test")
	if err != nil {
		t.Fatalf("getNamespaceDB failed: %v", err)
	}

	// Verify all 5 keys exist
	keys := [][]byte{
		formatMessageKey(result.GlobalPosition),
		formatStreamIndexKey("account-123", result.Position),
		formatCategoryIndexKey("account", result.GlobalPosition),
		formatVersionIndexKey("account-123"),
		formatGlobalPositionKey(),
	}

	for i, key := range keys {
		_, closer, err := handle.db.Get(key)
		if err != nil {
			t.Errorf("key %d (%s) not found: %v", i, string(key), err)
		} else {
			closer.Close()
		}
	}
}
