package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	storepkg "github.com/eventodb/eventodb/internal/store"
	_ "modernc.org/sqlite"
)

// getTestMetadataDB creates a new in-memory metadata database for testing
func getTestMetadataDB(t *testing.T) *sql.DB {
	t.Helper()

	// Use in-memory database for tests
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Failed to create in-memory database: %v", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	return db
}

// getTestStore creates a new SQLiteStore for testing
func getTestStore(t *testing.T, testMode bool) (*SQLiteStore, func()) {
	t.Helper()

	db := getTestMetadataDB(t)

	config := &Config{
		TestMode: testMode,
		DataDir:  t.TempDir(), // Use test temp directory
	}

	store, err := New(db, config)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create SQLiteStore: %v", err)
	}

	cleanup := func() {
		if err := store.Close(); err != nil {
			t.Logf("Warning: failed to close store: %v", err)
		}
	}

	return store, cleanup
}

// cleanupTestNamespaces removes all test namespaces
func cleanupTestNamespaces(t *testing.T, store *SQLiteStore) {
	t.Helper()

	ctx := context.Background()

	// Get all namespaces
	namespaces, err := store.ListNamespaces(ctx)
	if err != nil {
		t.Logf("Warning: failed to list namespaces for cleanup: %v", err)
		return
	}

	// Delete all test namespaces
	for _, ns := range namespaces {
		if err := store.DeleteNamespace(ctx, ns.ID); err != nil {
			t.Logf("Warning: failed to delete namespace %s: %v", ns.ID, err)
		}
	}
}

// cleanupNamespace removes a specific namespace if it exists (for test cleanup)
func cleanupNamespace(t *testing.T, store *SQLiteStore, namespace string) {
	t.Helper()
	ctx := context.Background()
	_ = store.DeleteNamespace(ctx, namespace) // Ignore error if doesn't exist
}

// MDB001_4A_T1: Test SQLiteStore creation
func TestMDB001_4A_T1_SQLiteStore_Creation(t *testing.T) {
	db := getTestMetadataDB(t)
	defer db.Close()

	config := &Config{
		TestMode: true,
		DataDir:  t.TempDir(),
	}

	store, err := New(db, config)
	if err != nil {
		t.Fatalf("Failed to create SQLiteStore: %v", err)
	}
	defer store.Close()

	if store.metadataDB == nil {
		t.Error("Expected metadataDB to be set")
	}

	if store.namespaces == nil {
		t.Error("Expected namespaces map to be initialized")
	}

	if !store.testMode {
		t.Error("Expected testMode to be true")
	}

	// Verify metadata table exists
	var tableExists bool
	query := `SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='namespaces'`
	if err := db.QueryRow(query).Scan(&tableExists); err != nil {
		t.Fatalf("Failed to check table existence: %v", err)
	}

	if !tableExists {
		t.Error("Expected namespaces table to exist after initialization")
	}
}

// MDB001_4A_T2: Test SQLiteStore with testMode=true (in-memory)
func TestMDB001_4A_T2_SQLiteStore_TestMode_InMemory(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	if !store.testMode {
		t.Error("Expected testMode to be true")
	}

	// Create a test namespace
	ctx := context.Background()
	err := store.CreateNamespace(ctx, "test_ns_1", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_1")

	// Verify db_path is in-memory
	ns, err := store.GetNamespace(ctx, "test_ns_1")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	expectedPath := "file:test_ns_1?mode=memory&cache=shared"
	if ns.DBPath != expectedPath {
		t.Errorf("Expected in-memory path %s, got %s", expectedPath, ns.DBPath)
	}
}

// MDB001_4A_T3: Test SQLiteStore with testMode=false (file-based)
func TestMDB001_4A_T3_SQLiteStore_TestMode_FileBased(t *testing.T) {
	store, cleanup := getTestStore(t, false)
	defer cleanup()

	if store.testMode {
		t.Error("Expected testMode to be false")
	}

	// Create a test namespace
	ctx := context.Background()
	err := store.CreateNamespace(ctx, "test_ns_2", "hash456", "Test namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_2")

	// Verify db_path is file-based
	ns, err := store.GetNamespace(ctx, "test_ns_2")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	expectedPath := filepath.Join(store.dataDir, "test_ns_2.db")
	if ns.DBPath != expectedPath {
		t.Errorf("Expected file path %s, got %s", expectedPath, ns.DBPath)
	}
}

// MDB001_4A_T4: Test Close() cleanup
func TestMDB001_4A_T4_Close_Cleanup(t *testing.T) {
	store, _ := getTestStore(t, true)

	ctx := context.Background()

	// Create a namespace to open a connection
	err := store.CreateNamespace(ctx, "test_ns_3", "hash789", "Test namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Access the namespace to create a connection
	_, err = store.getNamespaceHandle("test_ns_3")
	if err != nil {
		t.Fatalf("Failed to get namespace DB: %v", err)
	}

	// Verify connection exists
	if len(store.namespaces) != 1 {
		t.Errorf("Expected 1 namespace connection, got %d", len(store.namespaces))
	}

	// Close store
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Verify all connections closed and map cleared
	if len(store.namespaces) != 0 {
		t.Errorf("Expected 0 namespace connections after close, got %d", len(store.namespaces))
	}
}

// MDB001_4A_T5: Test CreateNamespace in test mode (in-memory)
func TestMDB001_4A_T5_CreateNamespace_TestMode(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_4", "hash_test_4", "Test namespace 4")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_4")

	// Verify namespace was created
	ns, err := store.GetNamespace(ctx, "test_ns_4")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	if ns.ID != "test_ns_4" {
		t.Errorf("Expected ID 'test_ns_4', got '%s'", ns.ID)
	}

	if ns.TokenHash != "hash_test_4" {
		t.Errorf("Expected TokenHash 'hash_test_4', got '%s'", ns.TokenHash)
	}

	if ns.Description != "Test namespace 4" {
		t.Errorf("Expected Description 'Test namespace 4', got '%s'", ns.Description)
	}

	// Verify CreatedAt is recent
	if time.Since(ns.CreatedAt) > 5*time.Second {
		t.Errorf("CreatedAt timestamp seems too old: %v", ns.CreatedAt)
	}

	// Verify db_path is in-memory
	expectedPath := "file:test_ns_4?mode=memory&cache=shared"
	if ns.DBPath != expectedPath {
		t.Errorf("Expected in-memory path %s, got %s", expectedPath, ns.DBPath)
	}
}

// MDB001_4A_T6: Test CreateNamespace in file mode
func TestMDB001_4A_T6_CreateNamespace_FileMode(t *testing.T) {
	store, cleanup := getTestStore(t, false)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_5", "hash_test_5", "Test namespace 5")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_5")

	// Verify namespace was created
	ns, err := store.GetNamespace(ctx, "test_ns_5")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	if ns.ID != "test_ns_5" {
		t.Errorf("Expected ID 'test_ns_5', got '%s'", ns.ID)
	}

	// Verify db_path is file-based
	expectedPath := filepath.Join(store.dataDir, "test_ns_5.db")
	if ns.DBPath != expectedPath {
		t.Errorf("Expected file path %s, got %s", expectedPath, ns.DBPath)
	}

	// Test that duplicate namespace creation fails
	err = store.CreateNamespace(ctx, "test_ns_5", "hash_different", "Duplicate namespace")
	if err == nil {
		t.Error("Expected error when creating duplicate namespace, got nil")
	}
}

// MDB001_4A_T7: Test DeleteNamespace closes connection and deletes file
func TestMDB001_4A_T7_DeleteNamespace_ClosesConnection(t *testing.T) {
	store, cleanup := getTestStore(t, false)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_6", "hash_test_6", "Test namespace 6")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Get the db_path before deletion
	ns, err := store.GetNamespace(ctx, "test_ns_6")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}
	dbPath := ns.DBPath

	// Open a connection to the namespace
	_, err = store.getNamespaceHandle("test_ns_6")
	if err != nil {
		t.Fatalf("Failed to get namespace DB: %v", err)
	}

	// Verify connection exists
	if _, exists := store.namespaces["test_ns_6"]; !exists {
		t.Error("Expected namespace connection to exist")
	}

	// Delete namespace
	err = store.DeleteNamespace(ctx, "test_ns_6")
	if err != nil {
		t.Fatalf("Failed to delete namespace: %v", err)
	}

	// Verify connection was closed and removed
	if _, exists := store.namespaces["test_ns_6"]; exists {
		t.Error("Expected namespace connection to be removed")
	}

	// Verify database file was deleted
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Error("Expected database file to be deleted")
	}

	// Verify namespace was removed from metadata
	_, err = store.GetNamespace(ctx, "test_ns_6")
	if err == nil {
		t.Error("Expected error when getting deleted namespace, got nil")
	}
}

// MDB001_4A_T8: Test getNamespaceHandle lazy-loads
func TestMDB001_4A_T8_GetOrCreateNamespaceDB_LazyLoads(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_7", "hash_test_7", "Test namespace 7")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_7")

	// Verify no connection exists yet
	if len(store.namespaces) != 0 {
		t.Errorf("Expected 0 namespace connections initially, got %d", len(store.namespaces))
	}

	// Access namespace - should create connection
	handle, err := store.getNamespaceHandle("test_ns_7")
	if err != nil {
		t.Fatalf("Failed to get namespace handle: %v", err)
	}

	if handle == nil {
		t.Error("Expected namespace handle, got nil")
	}

	// Verify connection was cached
	if len(store.namespaces) != 1 {
		t.Errorf("Expected 1 namespace connection, got %d", len(store.namespaces))
	}

	// Access again - should return cached connection
	handle2, err := store.getNamespaceHandle("test_ns_7")
	if err != nil {
		t.Fatalf("Failed to get namespace handle (cached): %v", err)
	}

	if handle != handle2 {
		t.Error("Expected same namespace handle to be returned")
	}

	// Try to access non-existent namespace
	_, err = store.getNamespaceHandle("nonexistent")
	if err == nil {
		t.Error("Expected error when accessing non-existent namespace, got nil")
	}
}

// MDB001_4A_T9: Test GetNamespace returns correct data
func TestMDB001_4A_T9_GetNamespace_ReturnsCorrectData(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	testID := "test_ns_8"
	testHash := "hash_test_8"
	testDesc := "Test namespace 8 description"

	err := store.CreateNamespace(ctx, testID, testHash, testDesc)
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, testID)

	// Get namespace
	ns, err := store.GetNamespace(ctx, testID)
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	// Verify all fields
	if ns.ID != testID {
		t.Errorf("Expected ID '%s', got '%s'", testID, ns.ID)
	}

	if ns.TokenHash != testHash {
		t.Errorf("Expected TokenHash '%s', got '%s'", testHash, ns.TokenHash)
	}

	if ns.Description != testDesc {
		t.Errorf("Expected Description '%s', got '%s'", testDesc, ns.Description)
	}

	if ns.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}

	if ns.DBPath == "" {
		t.Error("Expected DBPath to be set")
	}

	// Try to get non-existent namespace
	_, err = store.GetNamespace(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent namespace, got nil")
	}
}

// MDB001_4A_T10: Test ListNamespaces returns all namespaces
func TestMDB001_4A_T10_ListNamespaces_ReturnsAll(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()
	defer cleanupTestNamespaces(t, store)

	ctx := context.Background()

	// Initially should be empty
	namespaces, err := store.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list namespaces: %v", err)
	}

	if len(namespaces) != 0 {
		t.Errorf("Expected 0 namespaces initially, got %d", len(namespaces))
	}

	// Create multiple namespaces
	testNamespaces := []struct {
		id   string
		hash string
		desc string
	}{
		{"test_ns_9a", "hash_9a", "Test namespace 9a"},
		{"test_ns_9b", "hash_9b", "Test namespace 9b"},
		{"test_ns_9c", "hash_9c", "Test namespace 9c"},
	}

	for _, tn := range testNamespaces {
		err := store.CreateNamespace(ctx, tn.id, tn.hash, tn.desc)
		if err != nil {
			t.Fatalf("Failed to create namespace %s: %v", tn.id, err)
		}
	}

	// List all namespaces
	namespaces, err = store.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list namespaces: %v", err)
	}

	if len(namespaces) != len(testNamespaces) {
		t.Errorf("Expected %d namespaces, got %d", len(testNamespaces), len(namespaces))
	}

	// Verify namespaces are ordered by created_at DESC (most recent first)
	for i := 0; i < len(namespaces)-1; i++ {
		if namespaces[i].CreatedAt.Before(namespaces[i+1].CreatedAt) {
			t.Error("Expected namespaces to be ordered by created_at DESC")
			break
		}
	}

	// Verify all expected namespaces are present
	nsMap := make(map[string]*storepkg.Namespace)
	for _, ns := range namespaces {
		nsMap[ns.ID] = ns
	}

	for _, tn := range testNamespaces {
		if ns, exists := nsMap[tn.id]; !exists {
			t.Errorf("Expected namespace %s to be in list", tn.id)
		} else {
			if ns.TokenHash != tn.hash {
				t.Errorf("Expected TokenHash '%s', got '%s'", tn.hash, ns.TokenHash)
			}
			if ns.Description != tn.desc {
				t.Errorf("Expected Description '%s', got '%s'", tn.desc, ns.Description)
			}
		}
	}
}

// MDB001_4A_T11: Test GetNamespaceMessageCount returns correct count
func TestMDB001_4A_T11_GetNamespaceMessageCount_ReturnsCorrectCount(t *testing.T) {
	store, cleanup := getTestStore(t, true)
	defer cleanup()

	ctx := context.Background()

	// Create namespace
	err := store.CreateNamespace(ctx, "test_ns_count", "hash_count", "Test namespace for counting")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}
	defer cleanupNamespace(t, store, "test_ns_count")

	// Initially should have 0 messages
	count0, err := store.GetNamespaceMessageCount(ctx, "test_ns_count")
	if err != nil {
		t.Fatalf("Failed to get initial message count: %v", err)
	}
	if count0 != 0 {
		t.Errorf("Expected 0 messages initially, got %d", count0)
	}

	// Write 5 messages to stream 1
	for i := 0; i < 5; i++ {
		msg := &storepkg.Message{
			StreamName: "account-111",
			Type:       "TestMessage",
			Data:       map[string]interface{}{"index": i},
		}
		_, err := store.WriteMessage(ctx, "test_ns_count", "account-111", msg)
		if err != nil {
			t.Fatalf("Failed to write message %d: %v", i, err)
		}
	}

	// Should have 5 messages
	count1, err := store.GetNamespaceMessageCount(ctx, "test_ns_count")
	if err != nil {
		t.Fatalf("Failed to get message count after 5 messages: %v", err)
	}
	if count1 != 5 {
		t.Errorf("Expected 5 messages, got %d", count1)
	}

	// Write 3 messages to stream 2
	for i := 0; i < 3; i++ {
		msg := &storepkg.Message{
			StreamName: "account-222",
			Type:       "TestMessage",
			Data:       map[string]interface{}{"index": i},
		}
		_, err := store.WriteMessage(ctx, "test_ns_count", "account-222", msg)
		if err != nil {
			t.Fatalf("Failed to write message %d to stream 2: %v", i, err)
		}
	}

	// Should have 8 messages total
	count2, err := store.GetNamespaceMessageCount(ctx, "test_ns_count")
	if err != nil {
		t.Fatalf("Failed to get message count after 8 messages: %v", err)
	}
	if count2 != 8 {
		t.Errorf("Expected 8 messages, got %d", count2)
	}

	// Test with non-existent namespace
	_, err = store.GetNamespaceMessageCount(ctx, "nonexistent_namespace")
	if err == nil {
		t.Error("Expected error for non-existent namespace")
	}
	if err != storepkg.ErrNamespaceNotFound {
		t.Errorf("Expected ErrNamespaceNotFound, got: %v", err)
	}
}
