package pebble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/message-db/message-db/internal/store"
)

const (
	metadataDBName = "_metadata"
)

// PebbleStore implements store.Store using Pebble key-value store
type PebbleStore struct {
	metadataDB *pebble.DB                  // Namespace registry (opened first)
	namespaces map[string]*namespaceHandle // Lazy-loaded namespace DBs
	dataDir    string                      // Base directory for all databases
	mu         sync.RWMutex                // Protects namespaces map
}

// namespaceHandle holds a Pebble DB instance for a namespace
type namespaceHandle struct {
	db      *pebble.DB // Actual namespace Pebble DB
	writeMu sync.Mutex // Serializes writes for GP counter
}

// New creates a new PebbleStore
func New(dataDir string) (*PebbleStore, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open metadata DB
	metadataPath := filepath.Join(dataDir, metadataDBName)
	metadataDB, err := pebble.Open(metadataPath, &pebble.Options{
		Cache:        pebble.NewCache(64 << 20), // 64MB cache
		MemTableSize: 32 << 20,                  // 32MB memtable
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open metadata DB: %w", err)
	}

	return &PebbleStore{
		metadataDB: metadataDB,
		namespaces: make(map[string]*namespaceHandle),
		dataDir:    dataDir,
	}, nil
}

// Close closes the store and all open databases
func (s *PebbleStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close all namespace DBs
	for _, handle := range s.namespaces {
		if handle.db != nil {
			handle.db.Close()
		}
	}
	s.namespaces = nil

	// Close metadata DB
	if s.metadataDB != nil {
		if err := s.metadataDB.Close(); err != nil {
			return fmt.Errorf("failed to close metadata DB: %w", err)
		}
		s.metadataDB = nil
	}

	return nil
}

// getNamespaceDB lazy loads and returns a namespace Pebble DB handle
func (s *PebbleStore) getNamespaceDB(ctx context.Context, nsID string) (*namespaceHandle, error) {
	// Fast path: check if already open
	s.mu.RLock()
	if handle, ok := s.namespaces[nsID]; ok {
		s.mu.RUnlock()
		return handle, nil
	}
	s.mu.RUnlock()

	// Verify namespace exists in metadata DB (before acquiring write lock)
	key := formatNamespaceKey(nsID)
	_, closer, err := s.metadataDB.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, fmt.Errorf("namespace %s not found", nsID)
		}
		return nil, fmt.Errorf("failed to check namespace existence: %w", err)
	}
	closer.Close()

	// Slow path: open namespace DB
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check (another goroutine might have opened it)
	if handle, ok := s.namespaces[nsID]; ok {
		return handle, nil
	}

	// Open namespace Pebble DB
	dbPath := filepath.Join(s.dataDir, nsID)
	db, err := pebble.Open(dbPath, &pebble.Options{
		Cache:        pebble.NewCache(128 << 20), // 128MB cache
		MemTableSize: 64 << 20,                   // 64MB memtable
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open namespace DB: %w", err)
	}

	// Cache handle
	handle := &namespaceHandle{db: db}
	s.namespaces[nsID] = handle
	return handle, nil
}

// Verify PebbleStore implements store.Store interface
var _ store.Store = (*PebbleStore)(nil)

// Utility methods for stream name parsing

func (s *PebbleStore) Category(streamName string) string {
	return extractCategory(streamName)
}

func (s *PebbleStore) ID(streamName string) string {
	return extractCardinalID(streamName)
}

func (s *PebbleStore) CardinalID(streamName string) string {
	// Extract cardinal ID before '+'
	id := extractCardinalID(streamName)
	if len(id) > 0 {
		for i := 0; i < len(id); i++ {
			if id[i] == '+' {
				return id[:i]
			}
		}
	}
	return id
}

func (s *PebbleStore) IsCategory(name string) bool {
	return extractCardinalID(name) == ""
}

func (s *PebbleStore) Hash64(value string) int64 {
	return int64(hashCardinalID(value))
}
