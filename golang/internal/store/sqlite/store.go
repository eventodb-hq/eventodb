package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/eventodb/eventodb/internal/migrate"
	"github.com/eventodb/eventodb/internal/store"
	"github.com/eventodb/eventodb/migrations"
	_ "modernc.org/sqlite"
)

// namespaceHandle holds database and write mutex for a namespace
type namespaceHandle struct {
	db      *sql.DB
	writeMu sync.Mutex // Serializes all writes to this namespace
}

// SQLiteStore implements the Store interface for SQLite
type SQLiteStore struct {
	metadataDB *sql.DB
	namespaces map[string]*namespaceHandle
	testMode   bool
	dataDir    string
	mu         sync.RWMutex
}

// Config contains configuration options for SQLiteStore
type Config struct {
	TestMode bool
	DataDir  string
}

// New creates a new SQLiteStore instance
func New(metadataDB *sql.DB, config *Config) (*SQLiteStore, error) {
	if metadataDB == nil {
		return nil, fmt.Errorf("metadata database connection cannot be nil")
	}

	if config == nil {
		config = &Config{
			TestMode: false,
			DataDir:  "/tmp/eventodb",
		}
	}

	if config.DataDir == "" {
		config.DataDir = "/tmp/eventodb"
	}

	s := &SQLiteStore{
		metadataDB: metadataDB,
		namespaces: make(map[string]*namespaceHandle),
		testMode:   config.TestMode,
		dataDir:    config.DataDir,
	}

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

	var firstErr error

	for id, handle := range s.namespaces {
		if handle.db != nil {
			if err := handle.db.Close(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("failed to close namespace %s: %w", id, err)
			}
		}
	}
	s.namespaces = make(map[string]*namespaceHandle)

	if s.metadataDB != nil {
		if err := s.metadataDB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// getNamespaceHandle retrieves or creates a namespace handle
func (s *SQLiteStore) getNamespaceHandle(namespace string) (*namespaceHandle, error) {
	// Fast path
	s.mu.RLock()
	if handle, exists := s.namespaces[namespace]; exists {
		s.mu.RUnlock()
		return handle, nil
	}
	s.mu.RUnlock()

	// Slow path
	s.mu.Lock()
	defer s.mu.Unlock()

	if handle, exists := s.namespaces[namespace]; exists {
		return handle, nil
	}

	// Get db_path from metadata
	var dbPath string
	query := `SELECT db_path FROM namespaces WHERE id = ?`
	err := s.metadataDB.QueryRowContext(context.Background(), query, namespace).Scan(&dbPath)
	if err == sql.ErrNoRows {
		return nil, store.ErrNamespaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace db_path: %w", err)
	}

	// Open with WAL mode and busy timeout
	dsn := dbPath
	if s.testMode {
		dsn = dbPath + "&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	} else {
		dsn = dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open namespace database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	migrator := migrate.New(db, "sqlite", migrations.NamespaceSQLiteFS)
	if err := migrator.AutoMigrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run namespace migrations: %w", err)
	}

	handle := &namespaceHandle{db: db}
	s.namespaces[namespace] = handle
	return handle, nil
}

// getDBPath generates the database path for a namespace
func (s *SQLiteStore) getDBPath(id string) string {
	if s.testMode {
		return fmt.Sprintf("file:%s?mode=memory&cache=shared", id)
	}
	return filepath.Join(s.dataDir, fmt.Sprintf("%s.db", id))
}

// Utility functions

func (s *SQLiteStore) Category(streamName string) string {
	return store.Category(streamName)
}

func (s *SQLiteStore) ID(streamName string) string {
	return store.ID(streamName)
}

func (s *SQLiteStore) CardinalID(streamName string) string {
	return store.CardinalID(streamName)
}

func (s *SQLiteStore) IsCategory(name string) bool {
	return store.IsCategory(name)
}

func (s *SQLiteStore) Hash64(value string) int64 {
	return store.Hash64(value)
}
