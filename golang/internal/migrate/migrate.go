package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// Migrator handles database migrations for both PostgreSQL and SQLite
type Migrator struct {
	db      *sql.DB
	dialect string // "postgres" or "sqlite"
	fs      embed.FS
	ctx     context.Context
}

// New creates a new Migrator instance
func New(db *sql.DB, dialect string, fs embed.FS) *Migrator {
	return &Migrator{
		db:      db,
		dialect: dialect,
		fs:      fs,
		ctx:     context.Background(),
	}
}

// WithContext returns a new Migrator with the given context
func (m *Migrator) WithContext(ctx context.Context) *Migrator {
	return &Migrator{
		db:      m.db,
		dialect: m.dialect,
		fs:      m.fs,
		ctx:     ctx,
	}
}

// AutoMigrate runs all pending migrations
func (m *Migrator) AutoMigrate() error {
	// 1. Ensure schema_migrations table exists
	if err := m.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// 2. Load migration files from embedded FS
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// If no migrations found, return early (not an error)
	if len(migrations) == 0 {
		return nil
	}

	// 3. Check which migrations already applied
	appliedMigrations, err := m.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// 4. Apply pending migrations in order
	for _, migration := range migrations {
		if appliedMigrations[migration.name] {
			continue // Skip already applied
		}

		if err := m.applyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.name, err)
		}
	}

	return nil
}

// ApplyNamespaceMigration applies namespace migration with template substitution
// This applies ALL migrations (used for new namespace creation)
func (m *Migrator) ApplyNamespaceMigration(schemaName string) error {
	// Load migration files from embedded FS
	migrations, err := m.loadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load namespace migrations: %w", err)
	}

	// Apply each migration with template substitution
	for _, migration := range migrations {
		sql := strings.ReplaceAll(migration.content, "{{SCHEMA_NAME}}", schemaName)

		if _, err := m.db.ExecContext(m.ctx, sql); err != nil {
			return fmt.Errorf("failed to apply namespace migration %s: %w", migration.name, err)
		}
	}

	return nil
}

// MigrateNamespaceSchema applies pending migrations to an existing namespace schema
// schemaName is the full schema name (e.g., "eventodb_default" for Postgres)
// Returns the number of migrations applied
func (m *Migrator) MigrateNamespaceSchema(schemaName string) (int, error) {
	// Load migration files from embedded FS
	migrations, err := m.loadMigrations()
	if err != nil {
		return 0, fmt.Errorf("failed to load namespace migrations: %w", err)
	}

	if len(migrations) == 0 {
		return 0, nil
	}

	// Get current schema version
	currentVersion := m.getNamespaceSchemaVersion(schemaName)

	// Apply pending migrations
	applied := 0
	for _, migration := range migrations {
		version := extractVersionFromFilename(migration.name)
		if version <= currentVersion {
			continue // Already applied
		}

		sql := strings.ReplaceAll(migration.content, "{{SCHEMA_NAME}}", schemaName)

		if _, err := m.db.ExecContext(m.ctx, sql); err != nil {
			return applied, fmt.Errorf("failed to apply namespace migration %s: %w", migration.name, err)
		}
		applied++
	}

	return applied, nil
}

// getNamespaceSchemaVersion returns the current schema version for a namespace
// Returns 0 if _schema_version table doesn't exist (legacy namespace)
func (m *Migrator) getNamespaceSchemaVersion(schemaName string) int {
	var query string
	if m.dialect == "sqlite" {
		query = "SELECT COALESCE(MAX(version), 0) FROM _schema_version"
	} else {
		query = fmt.Sprintf(`SELECT COALESCE(MAX(version), 0) FROM "%s"._schema_version`, schemaName)
	}

	var version int
	err := m.db.QueryRowContext(m.ctx, query).Scan(&version)
	if err != nil {
		// Table doesn't exist or other error - treat as version 0
		return 0
	}
	return version
}

// extractVersionFromFilename extracts version number from filename like "001_xxx.sql"
func extractVersionFromFilename(filename string) int {
	// Extract leading digits
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

// migration represents a single migration file
type migration struct {
	name    string
	content string
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist
func (m *Migrator) ensureMigrationsTable() error {
	var createSQL string

	if m.dialect == "postgres" {
		createSQL = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version TEXT PRIMARY KEY,
				applied_at BIGINT NOT NULL
			);
		`
	} else {
		createSQL = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version TEXT PRIMARY KEY,
				applied_at INTEGER NOT NULL
			);
		`
	}

	_, err := m.db.ExecContext(m.ctx, createSQL)
	return err
}

// loadMigrations loads all migration files from the embedded filesystem
func (m *Migrator) loadMigrations() ([]migration, error) {
	// Try different possible paths based on dialect
	var paths []string

	switch m.dialect {
	case "postgres":
		paths = []string{"metadata/postgres", "namespace/postgres", ".", "testdata"}
	case "sqlite":
		paths = []string{"metadata/sqlite", "namespace/sqlite", ".", "testdata"}
	case "timescale":
		paths = []string{"metadata/timescale", "namespace/timescale", ".", "testdata"}
	default:
		paths = []string{".", "testdata", "metadata/postgres", "metadata/sqlite", "metadata/timescale", "namespace/postgres", "namespace/sqlite", "namespace/timescale"}
	}

	for _, dir := range paths {
		migs, err := m.loadMigrationsFromDir(dir)
		if err == nil && len(migs) > 0 {
			return migs, nil
		}
	}

	// No migrations found
	return nil, nil
}

// loadMigrationsFromDir loads migrations from a specific directory
func (m *Migrator) loadMigrationsFromDir(dir string) ([]migration, error) {
	entries, err := m.fs.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var migrations []migration

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		filePath := entry.Name()
		if dir != "." && dir != "" {
			filePath = path.Join(dir, entry.Name())
		}

		content, err := m.fs.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", filePath, err)
		}

		migrations = append(migrations, migration{
			name:    entry.Name(),
			content: string(content),
		})
	}

	// Sort migrations by name (assumes naming like 001_xxx.sql, 002_xxx.sql)
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].name < migrations[j].name
	})

	return migrations, nil
}

// getAppliedMigrations returns a set of already applied migration names
func (m *Migrator) getAppliedMigrations() (map[string]bool, error) {
	rows, err := m.db.QueryContext(m.ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

// applyMigration applies a single migration and records it
func (m *Migrator) applyMigration(mig migration) error {
	// Start transaction
	tx, err := m.db.BeginTx(m.ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.ExecContext(m.ctx, mig.content); err != nil {
		return err
	}

	// Record migration - use correct placeholder for dialect
	timestamp := time.Now().Unix()
	var insertSQL string
	if m.dialect == "sqlite" {
		insertSQL = "INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)"
	} else {
		insertSQL = "INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)"
	}

	if _, err := tx.ExecContext(m.ctx, insertSQL, mig.name, timestamp); err != nil {
		return err
	}

	return tx.Commit()
}

// ApplyTemplate replaces template variables in SQL content
func ApplyTemplate(content string, vars map[string]string) string {
	result := content
	for key, value := range vars {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// SanitizeSchemaName ensures schema name is safe for SQL
func SanitizeSchemaName(name string) string {
	// Only allow alphanumeric and underscore
	var result strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			result.WriteRune(ch)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

// GetSubFS returns a sub-filesystem from the given path
func GetSubFS(fs embed.FS, dir string) (embed.FS, error) {
	// Check if directory exists
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return embed.FS{}, err
	}

	// For simplicity, we'll work with the full FS and adjust paths
	// This is a limitation of embed.FS - it doesn't support Sub() method
	_ = entries
	return fs, nil
}

// ReadMigrationFile reads a specific migration file from the embedded FS
func ReadMigrationFile(fs embed.FS, filePath string) (string, error) {
	content, err := fs.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// GetMigrationPath constructs the full path to a migration file
func GetMigrationPath(baseDir, fileName string) string {
	return path.Join(baseDir, fileName)
}
