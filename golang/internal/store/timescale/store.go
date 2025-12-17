// Package timescale provides a TimescaleDB-backed implementation of the message store.
//
// This driver uses TimescaleDB's hypertable features for:
//   - Time-based partitioning (chunks) for efficient data lifecycle management
//   - Native compression (10-20x storage savings)
//   - Efficient data retention (drop chunks, not rows)
//   - S3 tiering preparation (export chunks to object storage)
//
// The schema is compatible with the standard Postgres driver but optimized for
// large-scale deployments with long data retention requirements.
package timescale

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/message-db/message-db/internal/migrate"
	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/migrations"
)

// TimescaleStore implements the Store interface for TimescaleDB
type TimescaleStore struct {
	db  *sql.DB
	ctx context.Context
}

// New creates a new TimescaleStore instance
// The provided db connection should already be connected to a TimescaleDB database
func New(db *sql.DB) (*TimescaleStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection cannot be nil")
	}

	s := &TimescaleStore{
		db:  db,
		ctx: context.Background(),
	}

	// Verify TimescaleDB extension is available
	var extExists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`).Scan(&extExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check TimescaleDB extension: %w", err)
	}
	if !extExists {
		return nil, fmt.Errorf("TimescaleDB extension is not installed. Run: CREATE EXTENSION IF NOT EXISTS timescaledb")
	}

	// Run metadata migrations to ensure message_store schema exists
	migrator := migrate.New(db, "timescale", migrations.MetadataTimescaleFS)
	if err := migrator.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to run metadata migrations: %w", err)
	}

	return s, nil
}

// Close closes the database connection
func (s *TimescaleStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// WithContext returns a new store with the given context
func (s *TimescaleStore) WithContext(ctx context.Context) *TimescaleStore {
	return &TimescaleStore{
		db:  s.db,
		ctx: ctx,
	}
}

// getSchemaName retrieves the schema name for a given namespace
func (s *TimescaleStore) getSchemaName(namespace string) (string, error) {
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
// It prefixes with "tsdb_" to distinguish from regular Postgres schemas
func (s *TimescaleStore) sanitizeSchemaName(namespace string) string {
	sanitized := migrate.SanitizeSchemaName(namespace)
	return fmt.Sprintf("tsdb_%s", sanitized)
}

// Utility function implementations (delegate to store package utilities)

// Category extracts the category name from a stream name
func (s *TimescaleStore) Category(streamName string) string {
	return store.Category(streamName)
}

// ID extracts the ID portion from a stream name
func (s *TimescaleStore) ID(streamName string) string {
	return store.ID(streamName)
}

// CardinalID extracts the cardinal ID (before '+') from a stream name
func (s *TimescaleStore) CardinalID(streamName string) string {
	return store.CardinalID(streamName)
}

// IsCategory determines if a name represents a category (no ID part)
func (s *TimescaleStore) IsCategory(name string) bool {
	return store.IsCategory(name)
}

// Hash64 computes a 64-bit hash compatible with Message DB
func (s *TimescaleStore) Hash64(value string) int64 {
	return store.Hash64(value)
}

// TimescaleDB-specific methods

// GetChunks returns information about all chunks for a namespace
func (s *TimescaleStore) GetChunks(ctx context.Context, namespace string) ([]*ChunkInfo, error) {
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT * FROM "%s".get_chunks()`, schemaName)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}
	defer rows.Close()

	var chunks []*ChunkInfo
	for rows.Next() {
		var c ChunkInfo
		if err := rows.Scan(&c.ChunkName, &c.RangeStart, &c.RangeEnd, &c.IsCompressed, &c.SizeBytes, &c.MessageCount); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		chunks = append(chunks, &c)
	}

	return chunks, rows.Err()
}

// CompressChunksOlderThan manually compresses chunks older than the given interval
func (s *TimescaleStore) CompressChunksOlderThan(ctx context.Context, namespace, interval string) ([]string, error) {
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT chunk_name FROM "%s".compress_chunks_older_than($1::interval)`, schemaName)
	rows, err := s.db.QueryContext(ctx, query, interval)
	if err != nil {
		return nil, fmt.Errorf("failed to compress chunks: %w", err)
	}
	defer rows.Close()

	var compressed []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan chunk name: %w", err)
		}
		compressed = append(compressed, name)
	}

	return compressed, rows.Err()
}

// DropChunksOlderThan drops chunks older than the given interval
// WARNING: This permanently deletes data! Ensure you have backups.
func (s *TimescaleStore) DropChunksOlderThan(ctx context.Context, namespace, interval string) ([]string, error) {
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT * FROM "%s".drop_chunks_older_than($1::interval)`, schemaName)
	rows, err := s.db.QueryContext(ctx, query, interval)
	if err != nil {
		return nil, fmt.Errorf("failed to drop chunks: %w", err)
	}
	defer rows.Close()

	var dropped []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan chunk name: %w", err)
		}
		dropped = append(dropped, name)
	}

	return dropped, rows.Err()
}

// Message operation implementations are in write.go, read.go, and namespace.go
