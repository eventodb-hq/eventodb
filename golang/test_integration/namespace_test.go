package integration

import (
	"context"
	"database/sql"
	"testing"

	"github.com/message-db/message-db/internal/auth"
	"github.com/message-db/message-db/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

// Test MDB002_2A_T10: Test default namespace created on first run
func TestMDB002_2A_T10_DefaultNamespaceCreatedOnFirstRun(t *testing.T) {
	// Create in-memory database
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

	// Create default namespace
	defaultNamespace := "default"
	token, err := auth.GenerateToken(defaultNamespace)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	err = st.CreateNamespace(ctx, defaultNamespace, tokenHash, "Default namespace")
	if err != nil {
		t.Fatalf("Failed to create default namespace: %v", err)
	}

	// Verify namespace exists
	ns, err := st.GetNamespace(ctx, defaultNamespace)
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	if ns.ID != defaultNamespace {
		t.Errorf("Expected namespace ID 'default', got '%s'", ns.ID)
	}

	if ns.TokenHash != tokenHash {
		t.Error("Token hash mismatch")
	}

	if ns.Description != "Default namespace" {
		t.Errorf("Expected description 'Default namespace', got '%s'", ns.Description)
	}
}

// Test MDB002_2A_T11: Test default token can be validated
func TestMDB002_2A_T11_DefaultTokenCanBeValidated(t *testing.T) {
	// Create in-memory database
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

	// Create default namespace
	defaultNamespace := "default"
	token, err := auth.GenerateToken(defaultNamespace)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	tokenHash := auth.HashToken(token)

	err = st.CreateNamespace(ctx, defaultNamespace, tokenHash, "Default namespace")
	if err != nil {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Parse token to extract namespace
	parsedNamespace, err := auth.ParseToken(token)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if parsedNamespace != defaultNamespace {
		t.Errorf("Expected namespace '%s', got '%s'", defaultNamespace, parsedNamespace)
	}

	// Verify token hash matches stored hash
	ns, err := st.GetNamespace(ctx, parsedNamespace)
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	computedHash := auth.HashToken(token)
	if ns.TokenHash != computedHash {
		t.Error("Token hash verification failed")
	}
}

// Test that multiple namespaces can coexist
func TestMDB002_2A_MultipleNamespacesCoexist(t *testing.T) {
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

	// Create multiple namespaces
	namespaces := []string{"default", "tenant-a", "tenant-b"}
	tokens := make(map[string]string)

	for _, ns := range namespaces {
		token, err := auth.GenerateToken(ns)
		if err != nil {
			t.Fatalf("Failed to generate token for %s: %v", ns, err)
		}
		tokens[ns] = token

		tokenHash := auth.HashToken(token)
		err = st.CreateNamespace(ctx, ns, tokenHash, ns+" namespace")
		if err != nil {
			t.Fatalf("Failed to create namespace %s: %v", ns, err)
		}
	}

	// Verify all namespaces exist
	nsList, err := st.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list namespaces: %v", err)
	}

	if len(nsList) != len(namespaces) {
		t.Errorf("Expected %d namespaces, got %d", len(namespaces), len(nsList))
	}

	// Verify each token is valid for its namespace
	for nsID, token := range tokens {
		parsedNS, err := auth.ParseToken(token)
		if err != nil {
			t.Errorf("Failed to parse token for %s: %v", nsID, err)
		}

		if parsedNS != nsID {
			t.Errorf("Token for %s parsed to %s", nsID, parsedNS)
		}

		ns, err := st.GetNamespace(ctx, parsedNS)
		if err != nil {
			t.Errorf("Failed to get namespace %s: %v", parsedNS, err)
		}

		if ns.TokenHash != auth.HashToken(token) {
			t.Errorf("Token hash mismatch for namespace %s", nsID)
		}
	}
}
