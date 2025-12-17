package api

import (
	"context"
	"database/sql"
	"testing"

	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

// TestNamespaceDeleteWithMessages tests that deleting a namespace with messages works
func TestNamespaceDeleteWithMessages(t *testing.T) {
	// Setup in-memory store
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Create namespace
	err = st.CreateNamespace(ctx, "test-ns", "token-hash", "Test namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Write some messages
	for i := 0; i < 5; i++ {
		msg := &store.Message{
			StreamName: "account-123",
			Type:       "TestEvent",
			Data:       map[string]interface{}{"i": i},
		}
		_, err := st.WriteMessage(ctx, "test-ns", "account-123", msg)
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}
	}

	// Delete namespace
	err = st.DeleteNamespace(ctx, "test-ns")
	if err != nil {
		t.Fatalf("Failed to delete namespace: %v", err)
	}

	// Verify namespace is gone
	_, err = st.GetNamespace(ctx, "test-ns")
	if err == nil {
		t.Fatal("Expected error getting deleted namespace")
	}
	if err != store.ErrNamespaceNotFound {
		t.Fatalf("Expected ErrNamespaceNotFound, got: %v", err)
	}

	// Create new namespace with same ID should work
	err = st.CreateNamespace(ctx, "test-ns", "token-hash-2", "New namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace after delete: %v", err)
	}
}

// TestNamespaceErrorTypes tests that error types are correctly identified
func TestNamespaceErrorTypes(t *testing.T) {
	// Setup in-memory store
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Test ErrNamespaceNotFound
	_, err = st.GetNamespace(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent namespace")
	}
	if err != store.ErrNamespaceNotFound {
		t.Errorf("Expected ErrNamespaceNotFound, got: %v (type: %T)", err, err)
	}

	// Create namespace
	err = st.CreateNamespace(ctx, "test-ns", "hash", "desc")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Test ErrNamespaceExists
	err = st.CreateNamespace(ctx, "test-ns", "hash2", "desc2")
	if err == nil {
		t.Fatal("Expected error for duplicate namespace")
	}
	if err != store.ErrNamespaceExists {
		t.Errorf("Expected ErrNamespaceExists, got: %v (type: %T)", err, err)
	}

	// Delete nonexistent namespace
	err = st.DeleteNamespace(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Expected error deleting nonexistent namespace")
	}
	if err != store.ErrNamespaceNotFound {
		t.Errorf("Expected ErrNamespaceNotFound, got: %v (type: %T)", err, err)
	}
}

// TestRapidNamespaceCreateDelete tests rapid create/delete cycles
func TestRapidNamespaceCreateDelete(t *testing.T) {
	// Setup in-memory store
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Rapid create/delete cycles
	for i := 0; i < 10; i++ {
		nsID := "rapid-test"

		// Create
		err := st.CreateNamespace(ctx, nsID, "hash", "desc")
		if err != nil {
			t.Fatalf("Cycle %d: Failed to create: %v", i, err)
		}

		// Write a message
		msg := &store.Message{
			StreamName: "stream-1",
			Type:       "Event",
			Data:       map[string]interface{}{"cycle": i},
		}
		_, err = st.WriteMessage(ctx, nsID, "stream-1", msg)
		if err != nil {
			t.Fatalf("Cycle %d: Failed to write: %v", i, err)
		}

		// Delete
		err = st.DeleteNamespace(ctx, nsID)
		if err != nil {
			t.Fatalf("Cycle %d: Failed to delete: %v", i, err)
		}
	}
}
