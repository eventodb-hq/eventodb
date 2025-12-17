package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/message-db/message-db/internal/migrate"
	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/migrations"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements the Store interface for SQLite
type SQLiteStore struct {
	metadataDB   *sql.DB
	namespaceDBs map[string]*sql.DB
	testMode     bool
	dataDir      string
	mu           sync.RWMutex
	ctx          context.Context
}

// Config contains configuration options for SQLiteStore
type Config struct {
	// TestMode enables in-memory databases for testing
	TestMode bool
	// DataDir specifies where to store namespace database files
	// Default: /tmp/messagedb
	DataDir string
}

// New creates a new SQLiteStore instance
// The provided metadataDB connection should already be connected
// testMode=true uses in-memory databases, testMode=false uses file-based databases
func New(metadataDB *sql.DB, config *Config) (*SQLiteStore, error) {
	if metadataDB == nil {
		return nil, fmt.Errorf("metadata database connection cannot be nil")
	}

	if config == nil {
		config = &Config{
			TestMode: false,
			DataDir:  "/tmp/messagedb",
		}
	}

	// Set default data directory if not specified
	if config.DataDir == "" {
		config.DataDir = "/tmp/messagedb"
	}

	s := &SQLiteStore{
		metadataDB:   metadataDB,
		namespaceDBs: make(map[string]*sql.DB),
		testMode:     config.TestMode,
		dataDir:      config.DataDir,
		ctx:          context.Background(),
	}

	// Run metadata migrations to ensure namespaces table exists
	migrator := migrate.New(metadataDB, "sqlite", migrations.MetadataSQLiteFS)
	if err := migrator.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to run metadata migrations: %w", err)
	}

	return s, nil
}

// Close closes all database connections
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close metadata database
	var firstErr error
	if s.metadataDB != nil {
		if err := s.metadataDB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close all namespace databases
	for id, db := range s.namespaceDBs {
		if db != nil {
			if err := db.Close(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("failed to close namespace %s: %w", id, err)
			}
		}
	}

	// Clear the map
	s.namespaceDBs = make(map[string]*sql.DB)

	return firstErr
}

// WithContext returns a new store with the given context
func (s *SQLiteStore) WithContext(ctx context.Context) *SQLiteStore {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &SQLiteStore{
		metadataDB:   s.metadataDB,
		namespaceDBs: s.namespaceDBs,
		testMode:     s.testMode,
		dataDir:      s.dataDir,
		mu:           sync.RWMutex{},
		ctx:          ctx,
	}
}

// getOrCreateNamespaceDB retrieves or creates a database connection for the given namespace
// This implements lazy loading - databases are only opened when first accessed
func (s *SQLiteStore) getOrCreateNamespaceDB(namespace string) (*sql.DB, error) {
	// Fast path: read lock to check if already exists
	s.mu.RLock()
	if db, exists := s.namespaceDBs[namespace]; exists {
		s.mu.RUnlock()
		return db, nil
	}
	s.mu.RUnlock()

	// Slow path: write lock to create connection
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if db, exists := s.namespaceDBs[namespace]; exists {
		return db, nil
	}

	// Get db_path from metadata
	var dbPath string
	query := `SELECT db_path FROM namespaces WHERE id = ?`
	err := s.metadataDB.QueryRowContext(s.ctx, query, namespace).Scan(&dbPath)
	if err == sql.ErrNoRows {
		return nil, store.ErrNamespaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace db_path: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open namespace database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // SQLite works best with single connection
	db.SetMaxIdleConns(1)

	// Apply namespace migrations
	migrator := migrate.New(db, "sqlite", migrations.NamespaceSQLiteFS)
	if err := migrator.AutoMigrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run namespace migrations: %w", err)
	}

	// Store in map
	s.namespaceDBs[namespace] = db

	return db, nil
}

// getDBPath generates the database path for a namespace
func (s *SQLiteStore) getDBPath(id string) string {
	if s.testMode {
		// In-memory database with shared cache
		return fmt.Sprintf("file:%s?mode=memory&cache=shared", id)
	}
	// File-based database
	return filepath.Join(s.dataDir, fmt.Sprintf("%s.db", id))
}

// Utility function implementations (delegate to store package utilities)

// Category extracts the category name from a stream name
func (s *SQLiteStore) Category(streamName string) string {
	return store.Category(streamName)
}

// ID extracts the ID portion from a stream name
func (s *SQLiteStore) ID(streamName string) string {
	return store.ID(streamName)
}

// CardinalID extracts the cardinal ID (before '+') from a stream name
func (s *SQLiteStore) CardinalID(streamName string) string {
	return store.CardinalID(streamName)
}

// IsCategory determines if a name represents a category (no ID part)
func (s *SQLiteStore) IsCategory(name string) bool {
	return store.IsCategory(name)
}

// Hash64 computes a 64-bit hash compatible with Message DB
func (s *SQLiteStore) Hash64(value string) int64 {
	return store.Hash64(value)
}

// Message operation implementations will be in write.go and read.go
