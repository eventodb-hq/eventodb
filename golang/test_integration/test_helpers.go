package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

// setupIsolatedTest creates a completely isolated test environment
// Each test gets its own SQLite database and unique namespace
func setupIsolatedTest(t *testing.T) (*sqlite.SQLiteStore, string, string, func()) {
	t.Helper()

	// Create unique in-memory database for this test
	// Each test gets a completely isolated database (no cache=shared to avoid locking issues)
	dbName := fmt.Sprintf("file:test-%s-%s?mode=memory",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create store with test mode
	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create unique namespace for this test
	namespace := fmt.Sprintf("test-%s-%s",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Test namespace")
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// EAGERLY initialize the namespace database to avoid lazy-loading race conditions
	_, err = st.GetNamespace(ctx, namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to initialize namespace: %v", err)
	}

	cleanup := func() {
		// Clean up namespace
		_ = st.DeleteNamespace(context.Background(), namespace)
		// Close connections
		_ = st.Close()
		_ = db.Close()
	}

	return st, namespace, token, cleanup
}

// setupIsolatedTestWithDefaultNamespace creates an isolated test environment
// with a "default" namespace for backward compatibility with some tests
func setupIsolatedTestWithDefaultNamespace(t *testing.T) (*sqlite.SQLiteStore, string, func()) {
	t.Helper()

	// Create unique in-memory database for this test
	dbName := fmt.Sprintf("file:test-%s-%s?mode=memory",
		sanitizeName(t.Name()),
		uuid.New().String()[:8])

	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create store with test mode
	st, err := sqlite.New(db, &sqlite.Config{TestMode: true})
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create "default" namespace
	namespace := "default"
	token, err := auth.GenerateToken(namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	ctx := context.Background()
	err = st.CreateNamespace(ctx, namespace, tokenHash, "Default namespace")
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// EAGERLY initialize the namespace database
	_, err = st.GetNamespace(ctx, namespace)
	if err != nil {
		st.Close()
		db.Close()
		t.Fatalf("Failed to initialize namespace: %v", err)
	}

	cleanup := func() {
		_ = st.DeleteNamespace(context.Background(), namespace)
		_ = st.Close()
		_ = db.Close()
	}

	return st, token, cleanup
}

// sanitizeName removes special characters from test names for use in database names
func sanitizeName(name string) string {
	// Remove slashes and other special characters
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result += string(r)
		} else if r == '/' || r == ' ' {
			result += "_"
		}
	}
	return result
}
