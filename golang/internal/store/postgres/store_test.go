package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// getTestDB creates a new database connection for testing
func getTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Get connection info from environment or use defaults
	host := getEnv("POSTGRES_HOST", "localhost")
	port := getEnv("POSTGRES_PORT", "5432")
	user := getEnv("POSTGRES_USER", "postgres")
	password := getEnv("POSTGRES_PASSWORD", "postgres")
	dbname := getEnv("POSTGRES_DB", "postgres")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Ping to verify connection
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	return db
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// cleanupTestNamespaces removes all test namespaces
func cleanupTestNamespaces(t *testing.T, store *PostgresStore) {
	t.Helper()

	ctx := context.Background()

	// Get all namespaces
	namespaces, err := store.ListNamespaces(ctx)
	if err != nil {
		t.Logf("Warning: failed to list namespaces for cleanup: %v", err)
		return
	}

	// Delete all test namespaces (but only test ones)
	for _, ns := range namespaces {
		// Delete namespaces that start with "test_ns_" or "test-ns-"
		if (len(ns.ID) > 8 && ns.ID[:8] == "test_ns_") ||
			(len(ns.ID) > 8 && ns.ID[:8] == "test-ns-") {
			if err := store.DeleteNamespace(ctx, ns.ID); err != nil {
				t.Logf("Warning: failed to delete namespace %s: %v", ns.ID, err)
			}
		}
	}
}

// cleanupNamespace removes a specific namespace if it exists (for test cleanup)
func cleanupNamespace(t *testing.T, store *PostgresStore, namespace string) {
	t.Helper()
	ctx := context.Background()
	_ = store.DeleteNamespace(ctx, namespace) // Ignore error if doesn't exist
}

// MDB001_2A_T1: Test PostgresStore creation and connection
func TestMDB001_2A_T1_PostgresStore_Creation(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}

	if store.db == nil {
		t.Error("Expected db to be set")
	}

	// Verify metadata schema exists
	var schemaExists bool
	query := `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = 'message_store')`
	if err := db.QueryRow(query).Scan(&schemaExists); err != nil {
		t.Fatalf("Failed to check schema existence: %v", err)
	}

	if !schemaExists {
		t.Error("Expected message_store schema to exist")
	}
}

// MDB001_2A_T2: Test Close() cleanup
func TestMDB001_2A_T2_Close_Cleanup(t *testing.T) {
	// Create a separate connection for this test (don't use shared testDB)
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("POSTGRES_HOST", "localhost"),
		getEnv("POSTGRES_PORT", "5432"),
		getEnv("POSTGRES_USER", "postgres"),
		getEnv("POSTGRES_PASSWORD", "postgres"),
		getEnv("POSTGRES_DB", "postgres"))

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Verify connection works
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}

	// Close should not error
	if err := store.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Subsequent operations should fail because connection is closed
	ctx := context.Background()
	_, err = store.ListNamespaces(ctx)
	if err == nil {
		t.Error("Expected ListNamespaces to fail after close, but it succeeded")
	}
}

// MDB001_2A_T3: Test CreateNamespace creates schema
func TestMDB001_2A_T3_CreateNamespace_CreatesSchema(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}
	defer cleanupTestNamespaces(t, store)

	ctx := context.Background()
	namespaceID := "test_ns_schema"
	tokenHash := "test_token_hash_123"
	description := "Test namespace for schema creation"

	// Create namespace
	if err := store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Verify schema was created
	schemaName := store.sanitizeSchemaName(namespaceID)
	var schemaExists bool
	query := `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`
	if err := db.QueryRowContext(ctx, query, schemaName).Scan(&schemaExists); err != nil {
		t.Fatalf("Failed to check schema existence: %v", err)
	}

	if !schemaExists {
		t.Errorf("Expected schema %s to exist", schemaName)
	}
}

// MDB001_2A_T4: Test CreateNamespace applies migrations
func TestMDB001_2A_T4_CreateNamespace_AppliesMigrations(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}
	defer cleanupTestNamespaces(t, store)

	ctx := context.Background()
	namespaceID := "test_ns_migrations"
	tokenHash := "test_token_hash_456"
	description := "Test namespace for migration verification"

	// Create namespace
	if err := store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Verify messages table exists
	schemaName := store.sanitizeSchemaName(namespaceID)
	query := fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = $1 AND table_name = 'messages'
		)
	`)

	var tableExists bool
	if err := db.QueryRowContext(ctx, query, schemaName).Scan(&tableExists); err != nil {
		t.Fatalf("Failed to check table existence: %v", err)
	}

	if !tableExists {
		t.Errorf("Expected messages table to exist in schema %s", schemaName)
	}

	// Verify utility functions exist
	functions := []string{"hash_64", "category", "id", "cardinal_id", "is_category", "acquire_lock"}
	for _, funcName := range functions {
		query := `
			SELECT EXISTS(
				SELECT 1 FROM pg_proc p
				JOIN pg_namespace n ON p.pronamespace = n.oid
				WHERE n.nspname = $1 AND p.proname = $2
			)
		`
		var funcExists bool
		if err := db.QueryRowContext(ctx, query, schemaName, funcName).Scan(&funcExists); err != nil {
			t.Fatalf("Failed to check function %s existence: %v", funcName, err)
		}

		if !funcExists {
			t.Errorf("Expected function %s to exist in schema %s", funcName, schemaName)
		}
	}
}

// MDB001_2A_T5: Test CreateNamespace inserts into registry
func TestMDB001_2A_T5_CreateNamespace_InsertsIntoRegistry(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}
	defer cleanupTestNamespaces(t, store)

	ctx := context.Background()
	namespaceID := "test_ns_registry"
	tokenHash := "test_token_hash_789"
	description := "Test namespace for registry verification"

	// Create namespace
	if err := store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Verify entry in registry
	var count int
	query := `SELECT COUNT(*) FROM message_store.namespaces WHERE id = $1`
	if err := db.QueryRowContext(ctx, query, namespaceID).Scan(&count); err != nil {
		t.Fatalf("Failed to query registry: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 entry in registry, got %d", count)
	}

	// Verify all fields
	var id, storedTokenHash, schemaName, storedDescription string
	var createdAt int64
	query = `SELECT id, token_hash, schema_name, description, created_at FROM message_store.namespaces WHERE id = $1`
	if err := db.QueryRowContext(ctx, query, namespaceID).Scan(&id, &storedTokenHash, &schemaName, &storedDescription, &createdAt); err != nil {
		t.Fatalf("Failed to scan registry entry: %v", err)
	}

	if id != namespaceID {
		t.Errorf("Expected id %s, got %s", namespaceID, id)
	}
	if storedTokenHash != tokenHash {
		t.Errorf("Expected token_hash %s, got %s", tokenHash, storedTokenHash)
	}
	if storedDescription != description {
		t.Errorf("Expected description %s, got %s", description, storedDescription)
	}
	if createdAt == 0 {
		t.Error("Expected createdAt to be set")
	}
}

// MDB001_2A_T6: Test DeleteNamespace drops schema
func TestMDB001_2A_T6_DeleteNamespace_DropsSchema(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}

	ctx := context.Background()
	namespaceID := "test_ns_delete"
	tokenHash := "test_token_hash_delete"
	description := "Test namespace for deletion"

	// Create namespace
	if err := store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	schemaName := store.sanitizeSchemaName(namespaceID)

	// Verify schema exists before deletion
	var schemaExists bool
	query := `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`
	if err := db.QueryRowContext(ctx, query, schemaName).Scan(&schemaExists); err != nil {
		t.Fatalf("Failed to check schema existence: %v", err)
	}
	if !schemaExists {
		t.Fatal("Schema should exist before deletion")
	}

	// Delete namespace
	if err := store.DeleteNamespace(ctx, namespaceID); err != nil {
		t.Fatalf("DeleteNamespace failed: %v", err)
	}

	// Verify schema was dropped
	if err := db.QueryRowContext(ctx, query, schemaName).Scan(&schemaExists); err != nil {
		t.Fatalf("Failed to check schema existence after deletion: %v", err)
	}

	if schemaExists {
		t.Errorf("Expected schema %s to be dropped", schemaName)
	}
}

// MDB001_2A_T7: Test DeleteNamespace removes from registry
func TestMDB001_2A_T7_DeleteNamespace_RemovesFromRegistry(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}

	ctx := context.Background()
	namespaceID := "test_ns_registry_delete"
	tokenHash := "test_token_hash_registry_del"
	description := "Test namespace for registry deletion"

	// Create namespace
	if err := store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Delete namespace
	if err := store.DeleteNamespace(ctx, namespaceID); err != nil {
		t.Fatalf("DeleteNamespace failed: %v", err)
	}

	// Verify entry removed from registry
	var count int
	query := `SELECT COUNT(*) FROM message_store.namespaces WHERE id = $1`
	if err := db.QueryRowContext(ctx, query, namespaceID).Scan(&count); err != nil {
		t.Fatalf("Failed to query registry: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 entries in registry, got %d", count)
	}
}

// MDB001_2A_T8: Test GetNamespace returns correct data
func TestMDB001_2A_T8_GetNamespace_ReturnsCorrectData(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}
	defer cleanupTestNamespaces(t, store)

	ctx := context.Background()
	namespaceID := "test_ns_get"
	tokenHash := "test_token_hash_get"
	description := "Test namespace for retrieval"

	// Create namespace
	beforeCreate := time.Now().UTC()
	if err := store.CreateNamespace(ctx, namespaceID, tokenHash, description); err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}
	afterCreate := time.Now().UTC()

	// Get namespace
	ns, err := store.GetNamespace(ctx, namespaceID)
	if err != nil {
		t.Fatalf("GetNamespace failed: %v", err)
	}

	// Verify fields
	if ns.ID != namespaceID {
		t.Errorf("Expected ID %s, got %s", namespaceID, ns.ID)
	}
	if ns.TokenHash != tokenHash {
		t.Errorf("Expected TokenHash %s, got %s", tokenHash, ns.TokenHash)
	}
	if ns.Description != description {
		t.Errorf("Expected Description %s, got %s", description, ns.Description)
	}
	// Allow for some clock skew (10 seconds before/after)
	if ns.CreatedAt.Before(beforeCreate.Add(-10*time.Second)) || ns.CreatedAt.After(afterCreate.Add(10*time.Second)) {
		t.Errorf("CreatedAt %v is not within expected range [%v, %v]", ns.CreatedAt, beforeCreate, afterCreate)
	}
	if ns.SchemaName == "" {
		t.Error("Expected SchemaName to be set")
	}
}

// MDB001_2A_T9: Test ListNamespaces returns all namespaces
func TestMDB001_2A_T9_ListNamespaces_ReturnsAll(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	store, err := New(db)
	if err != nil {
		t.Fatalf("Failed to create PostgresStore: %v", err)
	}
	defer cleanupTestNamespaces(t, store)

	ctx := context.Background()

	// Create multiple namespaces
	namespaces := []struct {
		id          string
		tokenHash   string
		description string
	}{
		{"test_ns_list_1", "hash1", "First test namespace"},
		{"test_ns_list_2", "hash2", "Second test namespace"},
		{"test_ns_list_3", "hash3", "Third test namespace"},
	}

	for _, ns := range namespaces {
		if err := store.CreateNamespace(ctx, ns.id, ns.tokenHash, ns.description); err != nil {
			t.Fatalf("CreateNamespace failed for %s: %v", ns.id, err)
		}
	}

	// List all namespaces
	list, err := store.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListNamespaces failed: %v", err)
	}

	if len(list) < len(namespaces) {
		t.Errorf("Expected at least %d namespaces, got %d", len(namespaces), len(list))
	}

	// Verify all created namespaces are in the list
	found := make(map[string]bool)
	for _, ns := range list {
		found[ns.ID] = true
	}

	for _, ns := range namespaces {
		if !found[ns.id] {
			t.Errorf("Expected namespace %s to be in the list", ns.id)
		}
	}
}
