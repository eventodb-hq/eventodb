package timescale

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/migrations"
)

// CreateNamespace creates a new namespace with its own TimescaleDB hypertable
func (s *TimescaleStore) CreateNamespace(ctx context.Context, id, tokenHash, description string) error {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Generate schema name
	schemaName := s.sanitizeSchemaName(id)

	// Check if namespace already exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM message_store.namespaces WHERE id = $1 OR schema_name = $2)`
	if err := tx.QueryRowContext(ctx, checkQuery, id, schemaName).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}
	if exists {
		return store.ErrNamespaceExists
	}

	// Apply TimescaleDB namespace migrations with template substitution
	if err := applyTimescaleMigrations(ctx, tx, schemaName); err != nil {
		return fmt.Errorf("failed to apply TimescaleDB migrations: %w", err)
	}

	// Insert into message_store.namespaces
	insertQuery := `
		INSERT INTO message_store.namespaces (id, token_hash, schema_name, description, created_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	createdAt := time.Now().UTC().Unix()
	metadata := `{"backend": "timescaledb"}` // Mark as TimescaleDB backend

	if _, err := tx.ExecContext(ctx, insertQuery, id, tokenHash, schemaName, description, createdAt, metadata); err != nil {
		return fmt.Errorf("failed to insert namespace: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteNamespace deletes a namespace and all its data
// This will drop all chunks efficiently
func (s *TimescaleStore) DeleteNamespace(ctx context.Context, id string) error {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get schema name
	var schemaName string
	getSchemaQuery := `SELECT schema_name FROM message_store.namespaces WHERE id = $1`
	if err := tx.QueryRowContext(ctx, getSchemaQuery, id).Scan(&schemaName); err != nil {
		if err == sql.ErrNoRows {
			return store.ErrNamespaceNotFound
		}
		return fmt.Errorf("failed to get schema name: %w", err)
	}

	// Drop schema (CASCADE removes everything including hypertable chunks)
	dropSchemaSQL := fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schemaName)
	if _, err := tx.ExecContext(ctx, dropSchemaSQL); err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	// Remove from registry
	deleteQuery := `DELETE FROM message_store.namespaces WHERE id = $1`
	if _, err := tx.ExecContext(ctx, deleteQuery, id); err != nil {
		return fmt.Errorf("failed to delete namespace from registry: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetNamespace retrieves a namespace by ID
func (s *TimescaleStore) GetNamespace(ctx context.Context, id string) (*store.Namespace, error) {
	query := `
		SELECT id, token_hash, schema_name, description, created_at, metadata
		FROM message_store.namespaces
		WHERE id = $1
	`

	var ns store.Namespace
	var createdAtUnix int64
	var metadataJSON []byte

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&ns.ID,
		&ns.TokenHash,
		&ns.SchemaName,
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
	if len(metadataJSON) > 0 {
		var metadata map[string]interface{}
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse metadata: %w", err)
		}
		ns.Metadata = metadata
	}

	return &ns, nil
}

// ListNamespaces retrieves all namespaces
func (s *TimescaleStore) ListNamespaces(ctx context.Context) ([]*store.Namespace, error) {
	query := `
		SELECT id, token_hash, schema_name, description, created_at, metadata
		FROM message_store.namespaces
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query namespaces: %w", err)
	}
	defer rows.Close()

	var namespaces []*store.Namespace

	for rows.Next() {
		var ns store.Namespace
		var createdAtUnix int64
		var metadataJSON []byte

		if err := rows.Scan(
			&ns.ID,
			&ns.TokenHash,
			&ns.SchemaName,
			&ns.Description,
			&createdAtUnix,
			&metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan namespace: %w", err)
		}

		// Convert Unix timestamp to time.Time
		ns.CreatedAt = time.Unix(createdAtUnix, 0).UTC()

		// Parse metadata JSON
		if len(metadataJSON) > 0 {
			var metadata map[string]interface{}
			if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
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

// applyTimescaleMigrations applies TimescaleDB namespace migrations with template substitution
func applyTimescaleMigrations(ctx context.Context, tx *sql.Tx, schemaName string) error {
	baseDir := "namespace/timescale"

	// Load migration content from embedded FS
	migrationFiles, err := migrations.NamespaceTimescaleFS.ReadDir(baseDir)
	if err != nil {
		return fmt.Errorf("failed to read TimescaleDB migrations from %s: %w", baseDir, err)
	}

	for _, entry := range migrationFiles {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Read migration file with full path
		filePath := fmt.Sprintf("%s/%s", baseDir, entry.Name())
		content, err := migrations.NamespaceTimescaleFS.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", filePath, err)
		}

		// Replace template variable
		sqlContent := strings.ReplaceAll(string(content), "{{SCHEMA_NAME}}", schemaName)

		// Execute migration
		if _, err := tx.ExecContext(ctx, sqlContent); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// GetNamespaceMessageCount returns the total number of messages in a namespace
func (s *TimescaleStore) GetNamespaceMessageCount(ctx context.Context, namespace string) (int64, error) {
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return 0, err
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM "%s".messages`, schemaName)
	var count int64
	err = s.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return count, nil
}
