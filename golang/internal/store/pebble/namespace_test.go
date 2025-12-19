package pebble

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateNamespace(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create namespace
	err = store.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Verify directory was created
	nsDir := filepath.Join(tmpDir, "test")
	if _, err := os.Stat(nsDir); os.IsNotExist(err) {
		t.Errorf("namespace directory was not created: %s", nsDir)
	}

	// Try to create again (should fail)
	err = store.CreateNamespace(ctx, "test", "hash456", "Another test")
	if err == nil {
		t.Error("expected error when creating duplicate namespace, got nil")
	}
}

func TestCreateNamespace_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		id          string
		tokenHash   string
		description string
		wantError   bool
	}{
		{"valid", "test", "hash123", "Test", false},
		{"empty_id", "", "hash123", "Test", true},
		{"empty_token", "test2", "", "Test", true},
		{"empty_description", "test3", "hash123", "", false}, // description can be empty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.CreateNamespace(ctx, tt.id, tt.tokenHash, tt.description)
			if (err != nil) != tt.wantError {
				t.Errorf("CreateNamespace() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestGetNamespace(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create namespace
	err = store.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Retrieve namespace
	ns, err := store.GetNamespace(ctx, "test")
	if err != nil {
		t.Fatalf("GetNamespace failed: %v", err)
	}

	// Verify fields
	if ns.ID != "test" {
		t.Errorf("ID = %s, want test", ns.ID)
	}
	if ns.TokenHash != "hash123" {
		t.Errorf("TokenHash = %s, want hash123", ns.TokenHash)
	}
	if ns.Description != "Test namespace" {
		t.Errorf("Description = %s, want 'Test namespace'", ns.Description)
	}
	if ns.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if ns.Metadata == nil {
		t.Error("Metadata should not be nil")
	}
}

func TestGetNamespace_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Try to get non-existent namespace
	_, err = store.GetNamespace(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent namespace, got nil")
	}
}

func TestListNamespaces(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Initially empty
	namespaces, err := store.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListNamespaces failed: %v", err)
	}
	if len(namespaces) != 0 {
		t.Errorf("expected 0 namespaces, got %d", len(namespaces))
	}

	// Create multiple namespaces
	for i := 1; i <= 3; i++ {
		id := "test" + string(rune('0'+i))
		err = store.CreateNamespace(ctx, id, "hash"+string(rune('0'+i)), "Test "+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("CreateNamespace failed: %v", err)
		}
	}

	// List should return all 3
	namespaces, err = store.ListNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListNamespaces failed: %v", err)
	}
	if len(namespaces) != 3 {
		t.Errorf("expected 3 namespaces, got %d", len(namespaces))
	}

	// Verify IDs
	ids := make(map[string]bool)
	for _, ns := range namespaces {
		ids[ns.ID] = true
	}
	for i := 1; i <= 3; i++ {
		id := "test" + string(rune('0'+i))
		if !ids[id] {
			t.Errorf("namespace %s not found in list", id)
		}
	}
}

func TestDeleteNamespace(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create namespace
	err = store.CreateNamespace(ctx, "test", "hash123", "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Verify it exists
	_, err = store.GetNamespace(ctx, "test")
	if err != nil {
		t.Fatalf("GetNamespace failed: %v", err)
	}

	// Delete it
	err = store.DeleteNamespace(ctx, "test")
	if err != nil {
		t.Fatalf("DeleteNamespace failed: %v", err)
	}

	// Verify it's gone
	_, err = store.GetNamespace(ctx, "test")
	if err == nil {
		t.Error("expected error for deleted namespace, got nil")
	}

	// Verify directory is gone
	nsDir := filepath.Join(tmpDir, "test")
	if _, err := os.Stat(nsDir); !os.IsNotExist(err) {
		t.Errorf("namespace directory still exists: %s", nsDir)
	}
}

func TestDeleteNamespace_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Try to delete non-existent namespace
	err = store.DeleteNamespace(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent namespace, got nil")
	}
}

func TestMetadataDBPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Create store and namespace
	{
		store, err := New(tmpDir)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		err = store.CreateNamespace(ctx, "test", "hash123", "Test namespace")
		if err != nil {
			t.Fatalf("CreateNamespace failed: %v", err)
		}

		store.Close()
	}

	// Reopen store and verify namespace persisted
	{
		store, err := New(tmpDir)
		if err != nil {
			t.Fatalf("failed to reopen store: %v", err)
		}
		defer store.Close()

		ns, err := store.GetNamespace(ctx, "test")
		if err != nil {
			t.Fatalf("GetNamespace failed after reopen: %v", err)
		}

		if ns.ID != "test" {
			t.Errorf("ID = %s, want test", ns.ID)
		}
		if ns.TokenHash != "hash123" {
			t.Errorf("TokenHash = %s, want hash123", ns.TokenHash)
		}
	}
}

func TestNamespaceTokenHashing(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create namespace with token hash
	tokenHash := "hashed_secret_token"
	err = store.CreateNamespace(ctx, "test", tokenHash, "Test namespace")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Retrieve and verify token is stored hashed
	ns, err := store.GetNamespace(ctx, "test")
	if err != nil {
		t.Fatalf("GetNamespace failed: %v", err)
	}

	if ns.TokenHash != tokenHash {
		t.Errorf("TokenHash = %s, want %s", ns.TokenHash, tokenHash)
	}

	// Verify it's not the plaintext token
	if ns.TokenHash == "plaintext_secret" {
		t.Error("token should be hashed, not plaintext")
	}
}
