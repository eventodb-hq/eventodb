package pebble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	"github.com/message-db/message-db/internal/store"
)

const (
	metadataDBName = "_metadata"
)

// Config contains configuration options for PebbleStore
type Config struct {
	TestMode bool // Use reduced memory settings optimized for tests
	InMemory bool // Use in-memory storage (faster, no disk persistence)
}

// PebbleStore implements store.Store using Pebble key-value store
type PebbleStore struct {
	metadataDB *pebble.DB                  // Namespace registry (opened first)
	namespaces map[string]*namespaceHandle // Lazy-loaded namespace DBs
	dataDir    string                      // Base directory for all databases
	config     *Config                     // Configuration options
	mu         sync.RWMutex                // Protects namespaces map
}

// namespaceHandle holds a Pebble DB instance for a namespace
type namespaceHandle struct {
	db      *pebble.DB // Actual namespace Pebble DB
	writeMu sync.Mutex // Serializes writes for GP counter
}

// New creates a new PebbleStore
func New(dataDir string) (*PebbleStore, error) {
	return NewWithConfig(dataDir, nil)
}

// NewWithConfig creates a new PebbleStore with custom configuration
func NewWithConfig(dataDir string, config *Config) (*PebbleStore, error) {
	if config == nil {
		config = &Config{
			TestMode: false,
			InMemory: false,
		}
	}
	// Create data directory if not in-memory mode
	if !config.InMemory {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}
	}

	// Get options based on config
	metadataOpts := getMetadataDBOptions(config)
	
	// Open metadata DB
	var metadataPath string
	if config.InMemory {
		metadataPath = "" // Empty path triggers in-memory mode
	} else {
		metadataPath = filepath.Join(dataDir, metadataDBName)
	}
	
	metadataDB, err := pebble.Open(metadataPath, metadataOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open metadata DB: %w", err)
	}

	return &PebbleStore{
		metadataDB: metadataDB,
		namespaces: make(map[string]*namespaceHandle),
		dataDir:    dataDir,
		config:     config,
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

	// Get options based on config
	namespaceOpts := getNamespaceDBOptions(s.config)
	
	// Open namespace Pebble DB
	var dbPath string
	if s.config.InMemory {
		dbPath = "" // Empty path triggers in-memory mode
	} else {
		dbPath = filepath.Join(s.dataDir, nsID)
	}
	
	db, err := pebble.Open(dbPath, namespaceOpts)
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

// getMetadataDBOptions returns Pebble options for metadata DB based on config
func getMetadataDBOptions(config *Config) *pebble.Options {
	if config.InMemory {
		// In-memory mode: minimal settings, no persistence
		return &pebble.Options{
			Cache:                       pebble.NewCache(8 << 20),    // 8MB cache
			MemTableSize:                4 << 20,                     // 4MB memtable
			MemTableStopWritesThreshold: 2,
			L0CompactionThreshold:       2,
			L0StopWritesThreshold:       4,
			MaxConcurrentCompactions:    func() int { return 1 },
			DisableWAL:                  true,  // No WAL in memory mode
			FS:                          vfs.NewMem(), // In-memory filesystem
		}
	}
	
	if config.TestMode {
		// Test mode: reduced memory footprint, optimized for speed
		return &pebble.Options{
			Cache:                       pebble.NewCache(32 << 20),   // 32MB cache
			MemTableSize:                16 << 20,                    // 16MB memtable
			MemTableStopWritesThreshold: 2,
			L0CompactionThreshold:       2,
			L0StopWritesThreshold:       4,
			MaxConcurrentCompactions:    func() int { return 1 },
			DisableWAL:                  true,                        // Disable WAL for test speed
			WALBytesPerSync:             0,
			BytesPerSync:                1 << 20,
		}
	}
	
	// Production mode: optimized for durability and throughput
	return &pebble.Options{
		Cache:                       pebble.NewCache(256 << 20),  // 256MB cache
		MemTableSize:                128 << 20,                   // 128MB memtable
		MemTableStopWritesThreshold: 4,
		L0CompactionThreshold:       4,
		L0StopWritesThreshold:       12,
		MaxConcurrentCompactions:    func() int { return 3 },
		DisableWAL:                  false,                       // Keep WAL for durability
		WALBytesPerSync:             0,
		BytesPerSync:                512 << 10,                   // Sync SSTs every 512KB
	}
}

// getNamespaceDBOptions returns Pebble options for namespace DB based on config
func getNamespaceDBOptions(config *Config) *pebble.Options {
	if config.InMemory {
		// In-memory mode: minimal settings, no persistence
		return &pebble.Options{
			Cache:                       pebble.NewCache(16 << 20),   // 16MB cache
			MemTableSize:                8 << 20,                     // 8MB memtable
			MemTableStopWritesThreshold: 2,
			L0CompactionThreshold:       2,
			L0StopWritesThreshold:       4,
			MaxConcurrentCompactions:    func() int { return 1 },
			DisableWAL:                  true,  // No WAL in memory mode
			FS:                          vfs.NewMem(), // In-memory filesystem
		}
	}
	
	if config.TestMode {
		// Test mode: reduced memory footprint, optimized for speed
		return &pebble.Options{
			Cache:                       pebble.NewCache(64 << 20),   // 64MB cache
			MemTableSize:                32 << 20,                    // 32MB memtable
			MemTableStopWritesThreshold: 2,
			L0CompactionThreshold:       2,
			L0StopWritesThreshold:       4,
			MaxConcurrentCompactions:    func() int { return 2 },
			DisableWAL:                  true,                        // Disable WAL for test speed
			WALBytesPerSync:             0,
			BytesPerSync:                1 << 20,
			MaxOpenFiles:                100,
		}
	}
	
	// Production mode: optimized for durability and high throughput
	return &pebble.Options{
		Cache:                       pebble.NewCache(1 << 30),    // 1GB cache
		MemTableSize:                256 << 20,                   // 256MB memtable
		MemTableStopWritesThreshold: 4,
		L0CompactionThreshold:       4,
		L0StopWritesThreshold:       12,
		MaxConcurrentCompactions:    func() int { return 4 },
		DisableWAL:                  false,                       // Keep WAL for durability
		WALBytesPerSync:             0,
		BytesPerSync:                1 << 20,                     // Sync SSTs every 1MB
		MaxOpenFiles:                1000,
	}
}
