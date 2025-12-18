package migrate

import (
	"database/sql"
	"embed"
	"testing"

	_ "modernc.org/sqlite"
)

//go:embed testdata
var testFS embed.FS

// MDB001_1A_T1: Test AutoMigrate creates schema_migrations table
func TestMDB001_1A_T1_AutoMigrateCreatesMigrationsTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	migrator := New(db, "sqlite", testFS)

	// First, verify table doesn't exist
	var tableName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&tableName)
	if err == nil {
		t.Fatal("schema_migrations table should not exist yet")
	}

	// Run AutoMigrate
	if err := migrator.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Verify table now exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&tableName)
	if err != nil {
		t.Fatalf("schema_migrations table should exist: %v", err)
	}
	if tableName != "schema_migrations" {
		t.Fatalf("expected table name 'schema_migrations', got %s", tableName)
	}
}

// MDB001_1A_T2: Test AutoMigrate applies pending migrations
func TestMDB001_1A_T2_AutoMigrateAppliesPendingMigrations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	migrator := New(db, "sqlite", testFS)

	if err := migrator.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Check that migration was recorded
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}

	t.Logf("Migration count: %d", count)

	// List all migrations
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		t.Fatalf("failed to list migrations: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("failed to scan version: %v", err)
		}
		t.Logf("Applied migration: %s", version)
	}

	if count == 0 {
		t.Fatal("expected migrations to be recorded")
	}

	// Verify migration was actually applied (check for test table)
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='test_table'").Scan(&tableName)
	if err != nil {
		t.Fatalf("test table should exist after migration: %v", err)
	}
}

// MDB001_1A_T3: Test AutoMigrate skips already-applied migrations
func TestMDB001_1A_T3_AutoMigrateSkipsAppliedMigrations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	migrator := New(db, "sqlite", testFS)

	// First run
	if err := migrator.AutoMigrate(); err != nil {
		t.Fatalf("first AutoMigrate failed: %v", err)
	}

	var countAfterFirst int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&countAfterFirst)
	if err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}

	// Second run
	if err := migrator.AutoMigrate(); err != nil {
		t.Fatalf("second AutoMigrate failed: %v", err)
	}

	var countAfterSecond int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&countAfterSecond)
	if err != nil {
		t.Fatalf("failed to query migrations: %v", err)
	}

	if countAfterFirst != countAfterSecond {
		t.Fatalf("migration count changed: expected %d, got %d", countAfterFirst, countAfterSecond)
	}
}

// MDB001_1A_T4: Test ApplyNamespaceMigration substitutes {{SCHEMA_NAME}}
func TestMDB001_1A_T4_ApplyNamespaceMigrationSubstitutesTemplate(t *testing.T) {
	// This will be tested with actual Postgres in later phases
	// For now, test the template substitution logic
	content := "CREATE SCHEMA {{SCHEMA_NAME}}; CREATE TABLE {{SCHEMA_NAME}}.test (id INT);"
	expected := "CREATE SCHEMA test_schema; CREATE TABLE test_schema.test (id INT);"

	vars := map[string]string{
		"SCHEMA_NAME": "test_schema",
	}

	result := ApplyTemplate(content, vars)
	if result != expected {
		t.Fatalf("template substitution failed:\nexpected: %s\ngot: %s", expected, result)
	}
}

// MDB001_1A_T5: Test migration tracking records version and timestamp
func TestMDB001_1A_T5_MigrationTrackingRecordsVersionAndTimestamp(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	migrator := New(db, "sqlite", testFS)

	if err := migrator.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	var version string
	var appliedAt int64
	err := db.QueryRow("SELECT version, applied_at FROM schema_migrations LIMIT 1").Scan(&version, &appliedAt)
	if err != nil {
		t.Fatalf("failed to query migration record: %v", err)
	}

	if version == "" {
		t.Fatal("version should not be empty")
	}

	if appliedAt == 0 {
		t.Fatal("applied_at timestamp should be set")
	}
}

// Helper: setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db
}
