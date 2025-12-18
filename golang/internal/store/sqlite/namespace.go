package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/message-db/message-db/internal/store"
)

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
