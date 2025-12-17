package integration

import (
	"context"
	"testing"

	"github.com/message-db/message-db/internal/auth"
)

// Test MDB002_2A_T10: Test default namespace created on first run
func TestMDB002_2A_T10_DefaultNamespaceCreatedOnFirstRun(t *testing.T) {
	t.Parallel()

	st, token, cleanup := setupIsolatedTestWithDefaultNamespace(t)
	defer cleanup()

	ctx := context.Background()

	// Verify namespace exists
	ns, err := st.GetNamespace(ctx, "default")
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	if ns.ID != "default" {
		t.Errorf("Expected namespace ID 'default', got '%s'", ns.ID)
	}

	// Verify token hash
	tokenHash := auth.HashToken(token)
	if ns.TokenHash != tokenHash {
		t.Error("Token hash mismatch")
	}

	if ns.Description != "Default namespace" {
		t.Errorf("Expected description 'Default namespace', got '%s'", ns.Description)
	}
}

// Test MDB002_2A_T11: Test default token can be validated
func TestMDB002_2A_T11_DefaultTokenCanBeValidated(t *testing.T) {
	t.Parallel()

	st, token, cleanup := setupIsolatedTestWithDefaultNamespace(t)
	defer cleanup()

	ctx := context.Background()

	// Parse token to extract namespace
	parsedNamespace, err := auth.ParseToken(token)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if parsedNamespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", parsedNamespace)
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
	t.Parallel()

	st, defaultToken, cleanup := setupIsolatedTestWithDefaultNamespace(t)
	defer cleanup()

	ctx := context.Background()

	// Create additional namespaces (default already exists from setup)
	additionalNamespaces := []string{"tenant-a", "tenant-b"}
	tokens := make(map[string]string)

	// Use the token returned from setup (this was used to create the default namespace)
	tokens["default"] = defaultToken

	for _, ns := range additionalNamespaces {
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

		// Eagerly initialize
		_, err = st.GetNamespace(ctx, ns)
		if err != nil {
			t.Fatalf("Failed to initialize namespace %s: %v", ns, err)
		}
	}

	// Verify all namespaces exist
	nsList, err := st.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list namespaces: %v", err)
	}

	expectedCount := 1 + len(additionalNamespaces) // default + additional
	if len(nsList) != expectedCount {
		t.Errorf("Expected %d namespaces, got %d", expectedCount, len(nsList))
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
