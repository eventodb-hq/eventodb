package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eventodb/eventodb/internal/store"
	"github.com/eventodb/eventodb/migrations"
)

// sqliteMigrations is a reference to the embedded SQLite namespace migrations
var sqliteMigrations embed.FS = migrations.NamespaceSQLiteFS

// CreateNamespace creates a new namespace
func (s *SQLiteStore) CreateNamespace(ctx context.Context, id, tokenHash, description string) error {
	dbPath := s.getDBPath(id)

	var exists bool
	err := s.metadataDB.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM namespaces WHERE id = ? OR db_path = ?)`,
		id, dbPath).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check existence: %w", err)
	}
	if exists {
		return store.ErrNamespaceExists
	}

	_, err = s.metadataDB.ExecContext(ctx,
		`INSERT INTO namespaces (id, token_hash, db_path, description, created_at, metadata) VALUES (?, ?, ?, ?, ?, ?)`,
		id, tokenHash, dbPath, description, time.Now().UTC().Unix(), "{}")
	if err != nil {
		return fmt.Errorf("failed to insert: %w", err)
	}

	return nil
}

// DeleteNamespace deletes a namespace
func (s *SQLiteStore) DeleteNamespace(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var dbPath string
	err := s.metadataDB.QueryRowContext(ctx, `SELECT db_path FROM namespaces WHERE id = ?`, id).Scan(&dbPath)
	if err == sql.ErrNoRows {
		return store.ErrNamespaceNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get db_path: %w", err)
	}

	if handle, exists := s.namespaces[id]; exists {
		handle.db.Close()
		delete(s.namespaces, id)
	}

	if !s.testMode && !strings.HasPrefix(dbPath, "file:") && !strings.Contains(dbPath, "mode=memory") {
		os.Remove(dbPath)
	}

	_, err = s.metadataDB.ExecContext(ctx, `DELETE FROM namespaces WHERE id = ?`, id)
	return err
}

// GetNamespace retrieves a namespace
func (s *SQLiteStore) GetNamespace(ctx context.Context, id string) (*store.Namespace, error) {
	var ns store.Namespace
	var createdAtUnix int64
	var metadataJSON string

	err := s.metadataDB.QueryRowContext(ctx,
		`SELECT id, token_hash, db_path, description, created_at, metadata FROM namespaces WHERE id = ?`,
		id).Scan(&ns.ID, &ns.TokenHash, &ns.DBPath, &ns.Description, &createdAtUnix, &metadataJSON)

	if err == sql.ErrNoRows {
		return nil, store.ErrNamespaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	ns.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
	if metadataJSON != "" && metadataJSON != "{}" {
		json.Unmarshal([]byte(metadataJSON), &ns.Metadata)
	}

	return &ns, nil
}

// ListNamespaces retrieves all namespaces
func (s *SQLiteStore) ListNamespaces(ctx context.Context) ([]*store.Namespace, error) {
	rows, err := s.metadataDB.QueryContext(ctx,
		`SELECT id, token_hash, db_path, description, created_at, metadata FROM namespaces ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	var namespaces []*store.Namespace
	for rows.Next() {
		var ns store.Namespace
		var createdAtUnix int64
		var metadataJSON string

		if err := rows.Scan(&ns.ID, &ns.TokenHash, &ns.DBPath, &ns.Description, &createdAtUnix, &metadataJSON); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}

		ns.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		if metadataJSON != "" && metadataJSON != "{}" {
			json.Unmarshal([]byte(metadataJSON), &ns.Metadata)
		}

		namespaces = append(namespaces, &ns)
	}

	return namespaces, rows.Err()
}

// GetNamespaceMessageCount returns the number of messages in a namespace
func (s *SQLiteStore) GetNamespaceMessageCount(ctx context.Context, namespace string) (int64, error) {
	handle, err := s.getNamespaceHandle(namespace)
	if err != nil {
		return 0, err
	}

	var count int64
	err = handle.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return count, nil
}

// MigrateNamespaces applies pending schema migrations to all existing namespaces
func (s *SQLiteStore) MigrateNamespaces(ctx context.Context) (int, error) {
	namespaces, err := s.ListNamespaces(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list namespaces: %w", err)
	}

	totalApplied := 0
	for _, ns := range namespaces {
		handle, err := s.getNamespaceHandle(ns.ID)
		if err != nil {
			return totalApplied, fmt.Errorf("failed to get handle for namespace %s: %w", ns.ID, err)
		}

		applied, err := s.migrateNamespaceDB(ctx, handle.db)
		if err != nil {
			return totalApplied, fmt.Errorf("failed to migrate namespace %s: %w", ns.ID, err)
		}
		totalApplied += applied
	}

	return totalApplied, nil
}

// migrateNamespaceDB applies pending migrations to a single namespace database
func (s *SQLiteStore) migrateNamespaceDB(ctx context.Context, db *sql.DB) (int, error) {
	baseDir := "namespace/sqlite"

	// Load migration files
	migrationFiles, err := sqliteMigrations.ReadDir(baseDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read namespace migrations: %w", err)
	}

	// Get current schema version
	currentVersion := getNamespaceSchemaVersion(ctx, db)

	applied := 0
	for _, entry := range migrationFiles {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Extract version from filename
		version := extractVersionFromFilename(entry.Name())
		if version <= currentVersion {
			continue // Already applied
		}

		// Read and apply migration
		filePath := fmt.Sprintf("%s/%s", baseDir, entry.Name())
		content, err := sqliteMigrations.ReadFile(filePath)
		if err != nil {
			return applied, fmt.Errorf("failed to read migration %s: %w", filePath, err)
		}

		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			return applied, fmt.Errorf("failed to apply migration %s: %w", entry.Name(), err)
		}
		applied++
	}

	return applied, nil
}

// getNamespaceSchemaVersion returns the current schema version for a namespace DB
func getNamespaceSchemaVersion(ctx context.Context, db *sql.DB) int {
	var version int
	err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM _schema_version").Scan(&version)
	if err != nil {
		// Table doesn't exist - treat as version 0
		return 0
	}
	return version
}

// extractVersionFromFilename extracts version number from filename like "001_xxx.sql"
func extractVersionFromFilename(filename string) int {
	var version int
	for _, ch := range filename {
		if ch >= '0' && ch <= '9' {
			version = version*10 + int(ch-'0')
		} else {
			break
		}
	}
	return version
}
