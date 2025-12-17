package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/message-db/message-db/internal/migrate"
	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/migrations"
)

// PostgresStore implements the Store interface for PostgreSQL
type PostgresStore struct {
	db  *sql.DB
	ctx context.Context
}

// New creates a new PostgresStore instance
// The provided db connection should already be connected to the database
// and have the message_store metadata schema initialized
func New(db *sql.DB) (*PostgresStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection cannot be nil")
	}

	s := &PostgresStore{
		db:  db,
		ctx: context.Background(),
	}

	// Run metadata migrations to ensure message_store schema exists
	migrator := migrate.New(db, "postgres", migrations.MetadataPostgresFS)
	if err := migrator.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to run metadata migrations: %w", err)
	}

	return s, nil
}

// Close closes the database connection
func (s *PostgresStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// WithContext returns a new store with the given context
func (s *PostgresStore) WithContext(ctx context.Context) *PostgresStore {
	return &PostgresStore{
		db:  s.db,
		ctx: ctx,
	}
}

// getSchemaName retrieves the schema name for a given namespace
func (s *PostgresStore) getSchemaName(namespace string) (string, error) {
	var schemaName string
	query := `SELECT schema_name FROM message_store.namespaces WHERE id = $1`

	err := s.db.QueryRowContext(s.ctx, query, namespace).Scan(&schemaName)
	if err == sql.ErrNoRows {
		return "", store.ErrNamespaceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get schema name: %w", err)
	}

	return schemaName, nil
}

// sanitizeSchemaName ensures the schema name is safe for use in SQL
// It prefixes with "messagedb_" and sanitizes the namespace ID
func (s *PostgresStore) sanitizeSchemaName(namespace string) string {
	sanitized := migrate.SanitizeSchemaName(namespace)
	return fmt.Sprintf("messagedb_%s", sanitized)
}

// Utility function implementations (delegate to store package utilities)

// Category extracts the category name from a stream name
func (s *PostgresStore) Category(streamName string) string {
	return store.Category(streamName)
}

// ID extracts the ID portion from a stream name
func (s *PostgresStore) ID(streamName string) string {
	return store.ID(streamName)
}

// CardinalID extracts the cardinal ID (before '+') from a stream name
func (s *PostgresStore) CardinalID(streamName string) string {
	return store.CardinalID(streamName)
}

// IsCategory determines if a name represents a category (no ID part)
func (s *PostgresStore) IsCategory(name string) bool {
	return store.IsCategory(name)
}

// Hash64 computes a 64-bit hash compatible with Message DB
func (s *PostgresStore) Hash64(value string) int64 {
	return store.Hash64(value)
}

// Message operation implementations are in write.go and read.go
