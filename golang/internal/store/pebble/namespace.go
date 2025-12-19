package pebble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/message-db/message-db/internal/store"
)

// CreateNamespace creates a new namespace with physical isolation
func (s *PebbleStore) CreateNamespace(ctx context.Context, id, tokenHash, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate inputs
	if id == "" {
		return fmt.Errorf("namespace ID cannot be empty")
	}
	if tokenHash == "" {
		return fmt.Errorf("namespace token hash cannot be empty")
	}

	// Check if namespace already exists
	key := formatNamespaceKey(id)
	_, closer, err := s.metadataDB.Get(key)
	if err == nil {
		closer.Close()
		return fmt.Errorf("namespace %s already exists", id)
	}
	if err != pebble.ErrNotFound {
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}

	// Create namespace directory
	nsDir := filepath.Join(s.dataDir, id)
	if err := os.MkdirAll(nsDir, 0755); err != nil {
		return fmt.Errorf("failed to create namespace directory: %w", err)
	}

	// Create namespace metadata
	ns := &store.Namespace{
		ID:          id,
		TokenHash:   tokenHash,
		Description: description,
		CreatedAt:   time.Now().UTC(),
		Metadata:    make(map[string]interface{}),
	}

	// Serialize to JSON
	value, err := json.Marshal(ns)
	if err != nil {
		return fmt.Errorf("failed to serialize namespace: %w", err)
	}

	// Write to metadata DB
	if err := s.metadataDB.Set(key, value, pebble.Sync); err != nil {
		// Clean up directory on failure
		os.RemoveAll(nsDir)
		return fmt.Errorf("failed to write namespace metadata: %w", err)
	}

	return nil
}

// GetNamespace retrieves namespace metadata
func (s *PebbleStore) GetNamespace(ctx context.Context, id string) (*store.Namespace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := formatNamespaceKey(id)
	value, closer, err := s.metadataDB.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, fmt.Errorf("namespace %s not found", id)
		}
		return nil, fmt.Errorf("failed to read namespace metadata: %w", err)
	}
	defer closer.Close()

	// Deserialize JSON
	var ns store.Namespace
	if err := json.Unmarshal(value, &ns); err != nil {
		return nil, fmt.Errorf("failed to deserialize namespace: %w", err)
	}

	return &ns, nil
}

// ListNamespaces returns all namespaces
func (s *PebbleStore) ListNamespaces(ctx context.Context) ([]*store.Namespace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var namespaces []*store.Namespace

	// Range scan with prefix "NS:"
	prefix := []byte(prefixNamespace)
	iter, err := s.metadataDB.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		value := iter.Value()

		var ns store.Namespace
		if err := json.Unmarshal(value, &ns); err != nil {
			return nil, fmt.Errorf("failed to deserialize namespace: %w", err)
		}

		namespaces = append(namespaces, &ns)
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	return namespaces, nil
}

// DeleteNamespace deletes a namespace and all its data
func (s *PebbleStore) DeleteNamespace(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if namespace exists
	key := formatNamespaceKey(id)
	_, closer, err := s.metadataDB.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return fmt.Errorf("namespace %s not found", id)
		}
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}
	closer.Close()

	// Close namespace DB if it's open
	if handle, ok := s.namespaces[id]; ok {
		if handle.db != nil {
			handle.db.Close()
		}
		delete(s.namespaces, id)
	}

	// Delete from metadata DB
	if err := s.metadataDB.Delete(key, pebble.Sync); err != nil {
		return fmt.Errorf("failed to delete namespace metadata: %w", err)
	}

	// Delete namespace directory
	nsDir := filepath.Join(s.dataDir, id)
	if err := os.RemoveAll(nsDir); err != nil {
		return fmt.Errorf("failed to delete namespace directory: %w", err)
	}

	return nil
}

// prefixUpperBound returns the upper bound for a prefix scan
func prefixUpperBound(prefix []byte) []byte {
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		end[i]++
		if end[i] != 0 {
			return end
		}
	}
	return nil // no upper bound if prefix is all 0xff
}
