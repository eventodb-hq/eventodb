package integration

import (
	"context"
	"testing"

	"github.com/eventodb/eventodb/internal/auth"
)

// Test MDB002_2A_T10: Test default namespace created on first run
func TestMDB002_2A_T10_DefaultNamespaceCreatedOnFirstRun(t *testing.T) {
	t.Parallel()

	env := SetupTestEnvWithDefaultNamespace(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Verify namespace exists
	ns, err := env.Store.GetNamespace(ctx, env.Namespace)
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	// For Postgres, namespace name is unique per test, for SQLite it's "default"
	if ns.ID != env.Namespace {
		t.Errorf("Expected namespace ID '%s', got '%s'", env.Namespace, ns.ID)
	}

	// Verify token hash
	tokenHash := auth.HashToken(env.Token)
	if ns.TokenHash != tokenHash {
		t.Error("Token hash mismatch")
	}
}

// Test MDB002_2A_T11: Test default token can be validated
func TestMDB002_2A_T11_DefaultTokenCanBeValidated(t *testing.T) {
	t.Parallel()

	env := SetupTestEnvWithDefaultNamespace(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Parse token to extract namespace
	parsedNamespace, err := auth.ParseToken(env.Token)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if parsedNamespace != env.Namespace {
		t.Errorf("Expected namespace '%s', got '%s'", env.Namespace, parsedNamespace)
	}

	// Verify token hash matches stored hash
	ns, err := env.Store.GetNamespace(ctx, parsedNamespace)
	if err != nil {
		t.Fatalf("Failed to get namespace: %v", err)
	}

	computedHash := auth.HashToken(env.Token)
	if ns.TokenHash != computedHash {
		t.Error("Token hash verification failed")
	}
}

// Test that multiple namespaces can coexist
func TestMDB002_2A_MultipleNamespacesCoexist(t *testing.T) {
	t.Parallel()

	env := SetupTestEnvWithDefaultNamespace(t)
	defer env.Cleanup()

	ctx := context.Background()

	// Create additional namespaces (default already exists from setup)
	additionalNamespaces := []string{"tenant_a", "tenant_b"}
	tokens := make(map[string]string)

	// Use the token returned from setup
	tokens[env.Namespace] = env.Token

	for _, ns := range additionalNamespaces {
		token, err := auth.GenerateToken(ns)
		if err != nil {
			t.Fatalf("Failed to generate token for %s: %v", ns, err)
		}
		tokens[ns] = token

		tokenHash := auth.HashToken(token)
		err = env.Store.CreateNamespace(ctx, ns, tokenHash, ns+" namespace")
		if err != nil {
			t.Fatalf("Failed to create namespace %s: %v", ns, err)
		}

		// Eagerly initialize
		_, err = env.Store.GetNamespace(ctx, ns)
		if err != nil {
			t.Fatalf("Failed to initialize namespace %s: %v", ns, err)
		}
	}

	// Verify all namespaces exist
	nsList, err := env.Store.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list namespaces: %v", err)
	}

	expectedCount := 1 + len(additionalNamespaces) // default + additional
	if len(nsList) < expectedCount {
		t.Errorf("Expected at least %d namespaces, got %d", expectedCount, len(nsList))
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

		ns, err := env.Store.GetNamespace(ctx, parsedNS)
		if err != nil {
			t.Errorf("Failed to get namespace %s: %v", parsedNS, err)
		}

		if ns.TokenHash != auth.HashToken(token) {
			t.Errorf("Token hash mismatch for namespace %s", nsID)
		}
	}

	// Cleanup additional namespaces
	for _, ns := range additionalNamespaces {
		_ = env.Store.DeleteNamespace(ctx, ns)
	}
}
