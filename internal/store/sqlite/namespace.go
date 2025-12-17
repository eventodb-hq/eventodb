package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/message-db/message-db/internal/store"
)

// CreateNamespace creates a new namespace with its own database file
func (s *SQLiteStore) CreateNamespace(ctx context.Context, id, tokenHash, description string) error {
	// Start transaction
	tx, err := s.metadataDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Generate db_path
	dbPath := s.getDBPath(id)

	// Check if namespace already exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM namespaces WHERE id = ? OR db_path = ?)`
	if err := tx.QueryRowContext(ctx, checkQuery, id, dbPath).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}
	if exists {
		return store.ErrNamespaceExists
	}

	// Insert into namespaces table
	insertQuery := `
		INSERT INTO namespaces (id, token_hash, db_path, description, created_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	createdAt := time.Now().UTC().Unix()
	metadata := "{}" // Empty JSON object

	if _, err := tx.ExecContext(ctx, insertQuery, id, tokenHash, dbPath, description, createdAt, metadata); err != nil {
		return fmt.Errorf("failed to insert namespace: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Note: The actual database file will be created lazily on first access
	// via getOrCreateNamespaceDB(), which will also apply migrations

	return nil
}

// DeleteNamespace deletes a namespace and all its data
func (s *SQLiteStore) DeleteNamespace(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get db_path
	var dbPath string
	query := `SELECT db_path FROM namespaces WHERE id = ?`
	err := s.metadataDB.QueryRowContext(ctx, query, id).Scan(&dbPath)
	if err == sql.ErrNoRows {
		return store.ErrNamespaceNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get namespace db_path: %w", err)
	}

	// Close connection if open
	if db, exists := s.namespaceDBs[id]; exists {
		if err := db.Close(); err != nil {
			return fmt.Errorf("failed to close namespace database: %w", err)
		}
		delete(s.namespaceDBs, id)
	}

	// Delete database file (if not in-memory)
	if !s.testMode && !strings.HasPrefix(dbPath, "file:") && !strings.Contains(dbPath, "mode=memory") {
		// Only delete physical files, not in-memory databases
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete database file: %w", err)
		}
	}

	// Remove from metadata
	deleteQuery := `DELETE FROM namespaces WHERE id = ?`
	if _, err := s.metadataDB.ExecContext(ctx, deleteQuery, id); err != nil {
		return fmt.Errorf("failed to delete namespace from registry: %w", err)
	}

	return nil
}

// GetNamespace retrieves a namespace by ID
func (s *SQLiteStore) GetNamespace(ctx context.Context, id string) (*store.Namespace, error) {
	query := `
		SELECT id, token_hash, db_path, description, created_at, metadata
		FROM namespaces
		WHERE id = ?
	`

	var ns store.Namespace
	var createdAtUnix int64
	var metadataJSON string

	err := s.metadataDB.QueryRowContext(ctx, query, id).Scan(
		&ns.ID,
		&ns.TokenHash,
		&ns.DBPath,
		&ns.Description,
		&createdAtUnix,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, store.ErrNamespaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	// Convert Unix timestamp to time.Time
	ns.CreatedAt = time.Unix(createdAtUnix, 0).UTC()

	// Parse metadata JSON
	if metadataJSON != "" && metadataJSON != "{}" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse metadata: %w", err)
		}
		ns.Metadata = metadata
	}

	return &ns, nil
}

// ListNamespaces retrieves all namespaces
func (s *SQLiteStore) ListNamespaces(ctx context.Context) ([]*store.Namespace, error) {
	query := `
		SELECT id, token_hash, db_path, description, created_at, metadata
		FROM namespaces
		ORDER BY created_at DESC
	`

	rows, err := s.metadataDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query namespaces: %w", err)
	}
	defer rows.Close()

	var namespaces []*store.Namespace

	for rows.Next() {
		var ns store.Namespace
		var createdAtUnix int64
		var metadataJSON string

		if err := rows.Scan(
			&ns.ID,
			&ns.TokenHash,
			&ns.DBPath,
			&ns.Description,
			&createdAtUnix,
			&metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan namespace: %w", err)
		}

		// Convert Unix timestamp to time.Time
		ns.CreatedAt = time.Unix(createdAtUnix, 0).UTC()

		// Parse metadata JSON
		if metadataJSON != "" && metadataJSON != "{}" {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
				return nil, fmt.Errorf("failed to parse metadata: %w", err)
			}
			ns.Metadata = metadata
		}

		namespaces = append(namespaces, &ns)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating namespaces: %w", err)
	}

	return namespaces, nil
}
