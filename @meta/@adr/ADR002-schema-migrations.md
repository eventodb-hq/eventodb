# ADR-002: Schema Migrations

**Date:** 2024-12-17  
**Status:** Accepted  
**Context:** Replace bash-based installation/migration scripts with proper Go-based migration system

---

## Problem Statement

Current Message DB installation approach:

### Issues
1. **Bash scripts everywhere**: `install.sh`, `update.sh`, multiple shell scripts
2. **No migration history**: No table tracking which migrations have run
3. **Version stored in function**: `message_store_version()` returns hardcoded string
4. **Fragile**: Requires bash, psql, specific environment variables
5. **No rollback**: Cannot undo migrations
6. **Poor developer experience**: Hard to debug, test, or extend

### Current Structure
```
database/
├── install.sh              # Main installer
├── update.sh               # Version updater
├── install-functions.sh    # Function installer
├── install-views.sh
├── install-privileges.sh
├── VERSION.txt             # "1.3.0"
├── update/
│   ├── 1.0.0.sh
│   ├── 1.2.2.sh
│   └── 1.3.0.sh
└── functions/*.sql
```

---

## Decision

Implement a **Go-based migration system** that runs **automatically on boot**.

### Architecture

1. **Auto-migrate on startup** - No manual migration commands needed
2. **Separate migrations for Postgres and SQLite** - Handle dialect differences
3. **Embedded SQL files** using Go 1.16+ `embed`
4. **Migration tracking table** in database
5. **Idempotent migrations** (safe to re-run)
6. **Transparent** - Developers don't think about migrations

---

## Implementation Design

### 1. Migration Tracking Table

```sql
CREATE TABLE IF NOT EXISTS message_store.schema_migrations (
  version VARCHAR(255) PRIMARY KEY,
  applied_at TIMESTAMP NOT NULL DEFAULT NOW(),
  checksum VARCHAR(64) NOT NULL,
  description TEXT
);
```

**Fields:**
- `version`: Migration version (e.g., "001", "002", "1.3.0")
- `applied_at`: When migration was applied
- `checksum`: SHA-256 of SQL content (detects tampering)
- `description`: Human-readable description

---

### 2. Migration File Structure

```
migrations/
├── embed.go
├── postgres/
│   ├── 001_initial_schema.sql
│   ├── 002_add_namespaces.sql
│   └── 003_statistics_cache.sql
└── sqlite/
    ├── 001_initial_schema.sql
    ├── 002_add_namespaces.sql
    └── 003_statistics_cache.sql
```

**Key differences:**
- **Separate directories** for Postgres vs SQLite
- **Sequential numbering** (001, 002, 003...)
- **No up/down** - Only forward migrations (event sourcing philosophy)
- **Idempotent** - Safe to re-run

---

### 3. Migration Content Format

**migrations/postgres/001_initial_schema.sql:**
```sql
-- Migration: 001_initial_schema
-- Description: Creates core message_store schema, tables, and functions (Postgres)

-- Schema migrations tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at BIGINT NOT NULL
);

-- Create schema
CREATE SCHEMA IF NOT EXISTS message_store;

-- Create message table
CREATE TABLE IF NOT EXISTS message_store.messages (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  stream_name VARCHAR NOT NULL,
  type VARCHAR NOT NULL,
  position BIGINT NOT NULL,
  global_position BIGSERIAL NOT NULL,
  data JSONB,
  metadata JSONB,
  time TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT (now() AT TIME ZONE 'utc'),
  PRIMARY KEY (global_position)
);

-- Create indexes
CREATE UNIQUE INDEX IF NOT EXISTS messages_id ON message_store.messages (id);
CREATE UNIQUE INDEX IF NOT EXISTS messages_stream ON message_store.messages (stream_name, position);
CREATE INDEX IF NOT EXISTS messages_category ON message_store.messages (
  (SPLIT_PART(stream_name, '-', 1)),
  global_position,
  (metadata->>'correlationStreamName')
);

-- Create write_message function (stored procedure)
CREATE OR REPLACE FUNCTION message_store.write_message(
  _id VARCHAR,
  _stream_name VARCHAR,
  _type VARCHAR,
  _data JSONB,
  _metadata JSONB DEFAULT NULL,
  _expected_version BIGINT DEFAULT NULL
) RETURNS BIGINT AS $$
DECLARE
  _position BIGINT;
  _stream_version BIGINT;
BEGIN
  -- Implementation from database/functions/write-message.sql
  -- ...
  RETURN _position;
END;
$$ LANGUAGE plpgsql;
```

**migrations/sqlite/001_initial_schema.sql:**
```sql
-- Migration: 001_initial_schema
-- Description: Creates core message_store tables (SQLite - no stored procedures)

-- Schema migrations tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at INTEGER NOT NULL
);

-- Create message table (SQLite doesn't support schemas)
CREATE TABLE IF NOT EXISTS messages (
  id TEXT NOT NULL,  -- UUID as TEXT
  stream_name TEXT NOT NULL,
  type TEXT NOT NULL,
  position INTEGER NOT NULL,
  global_position INTEGER PRIMARY KEY AUTOINCREMENT,
  data TEXT,  -- JSON as TEXT
  metadata TEXT,  -- JSON as TEXT
  time INTEGER NOT NULL  -- Unix timestamp
);

-- Create indexes
CREATE UNIQUE INDEX IF NOT EXISTS messages_id ON messages (id);
CREATE UNIQUE INDEX IF NOT EXISTS messages_stream ON messages (stream_name, position);
CREATE INDEX IF NOT EXISTS messages_category ON messages (
  substr(stream_name, 1, instr(stream_name || '-', '-') - 1),
  global_position
);

-- Note: No stored procedures in SQLite
-- write_message logic will be implemented in Go
```

---

### 4. Automatic Migration on Boot

**No CLI commands needed!** Migrations run automatically when the server starts.

```bash
# Just start the server
messagedb serve

# Output:
# [MIGRATE] Checking schema...
# [MIGRATE] Running migration 001_initial_schema.sql
# [MIGRATE] Running migration 002_add_namespaces.sql
# [MIGRATE] Schema up to date (version: 002)
# [SERVER] Starting on :8080
```

**Optional status command:**
```bash
messagedb migrate status

# Output:
# Version | Name              | Applied At
# --------|-------------------|----------------------
# 001     | initial_schema    | 2024-12-17 01:00:00
# 002     | add_namespaces    | 2024-12-17 01:00:01
```

---

### 5. Go Code Structure

```go
// migrations/embed.go
package migrations

import "embed"

//go:embed sqlite/*.sql
var SQLiteFS embed.FS

//go:embed postgres/*.sql
var PostgresFS embed.FS
```

```go
// internal/migrate/migrate.go
package migrate

import (
    "database/sql"
    "embed"
    "fmt"
    "io/fs"
    "sort"
    "strings"
    "time"
)

type Migration struct {
    Version int
    Name    string
    SQL     string
}

type Migrator struct {
    db      *sql.DB
    dialect string // "postgres" or "sqlite"
    fs      embed.FS
}

func New(db *sql.DB, dialect string, migrationsFS embed.FS) *Migrator {
    return &Migrator{
        db:      db,
        dialect: dialect,
        fs:      migrationsFS,
    }
}

// AutoMigrate runs all pending migrations automatically
func (m *Migrator) AutoMigrate() error {
    // 1. Ensure schema_migrations table exists
    if err := m.ensureMigrationsTable(); err != nil {
        return fmt.Errorf("ensure migrations table: %w", err)
    }

    // 2. Load all migrations from embedded FS
    migrations, err := m.loadMigrations()
    if err != nil {
        return fmt.Errorf("load migrations: %w", err)
    }

    // 3. Get applied migrations
    applied, err := m.getAppliedMigrations()
    if err != nil {
        return fmt.Errorf("get applied migrations: %w", err)
    }

    // 4. Find and apply pending migrations
    for _, mig := range migrations {
        if !applied[mig.Version] {
            if err := m.applyMigration(mig); err != nil {
                return fmt.Errorf("apply migration %d: %w", mig.Version, err)
            }
            fmt.Printf("[MIGRATE] Applied %03d_%s\n", mig.Version, mig.Name)
        }
    }

    return nil
}

func (m *Migrator) loadMigrations() ([]Migration, error) {
    dir := m.dialect // "postgres" or "sqlite"
    entries, err := fs.ReadDir(m.fs, dir)
    if err != nil {
        return nil, err
    }

    var migrations []Migration
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
            continue
        }

        // Parse version from filename: "001_initial_schema.sql"
        var version int
        var name string
        fmt.Sscanf(entry.Name(), "%d_%s", &version, &name)
        name = strings.TrimSuffix(name, ".sql")

        // Read SQL content
        content, err := fs.ReadFile(m.fs, dir+"/"+entry.Name())
        if err != nil {
            return nil, err
        }

        migrations = append(migrations, Migration{
            Version: version,
            Name:    name,
            SQL:     string(content),
        })
    }

    // Sort by version
    sort.Slice(migrations, func(i, j int) bool {
        return migrations[i].Version < migrations[j].Version
    })

    return migrations, nil
}

func (m *Migrator) applyMigration(mig Migration) error {
    tx, err := m.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Execute migration SQL
    if _, err := tx.Exec(mig.SQL); err != nil {
        return fmt.Errorf("exec sql: %w", err)
    }

    // Record in schema_migrations
    now := time.Now().Unix()
    _, err = tx.Exec(
        "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
        mig.Version, mig.Name, now,
    )
    if err != nil {
        return fmt.Errorf("record migration: %w", err)
    }

    return tx.Commit()
}
```

---

### 6. SQLite vs Postgres Differences

**Key differences to handle:**

| Feature | Postgres | SQLite | Solution |
|---------|----------|--------|----------|
| **Stored Procedures** | YES (PL/pgSQL) | NO | Implement in Go for SQLite |
| **Schemas** | `message_store.messages` | Single namespace | Use table prefix or omit |
| **UUID Type** | `UUID` | `TEXT` | Store as TEXT in SQLite |
| **JSONB** | `JSONB` | `TEXT` | Store as TEXT, parse in Go |
| **BIGSERIAL** | Auto-increment 64-bit | `INTEGER AUTOINCREMENT` | Different syntax |
| **Boolean** | `BOOLEAN` | `INTEGER` (0/1) | Map in Go |
| **Timestamp** | `TIMESTAMP` | `INTEGER` (Unix) | Convert in Go |
| **String Functions** | `SPLIT_PART()` | `substr()`, `instr()` | Different SQL |

**Example: Category extraction**

Postgres:
```sql
SPLIT_PART(stream_name, '-', 1)
```

SQLite:
```sql
substr(stream_name, 1, instr(stream_name || '-', '-') - 1)
```

---

### 7. Stored Procedures: Postgres vs Go

**Postgres approach:**
```sql
-- migrations/postgres/001_initial_schema.sql
CREATE OR REPLACE FUNCTION message_store.write_message(...)
RETURNS BIGINT AS $$
  -- Full implementation in PL/pgSQL
$$ LANGUAGE plpgsql;
```

**SQLite approach (no stored procedures):**
```go
// internal/store/sqlite/write.go
func (s *SQLiteStore) WriteMessage(ctx context.Context, msg *Message) (int64, error) {
    // Implement write_message logic in Go
    // This is the SAME logic as the Postgres stored procedure
    // Just written in Go instead of PL/pgSQL
    
    tx, err := s.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 1. Check expected version
    // 2. Calculate next position
    // 3. Insert message
    // 4. Commit
    
    return position, tx.Commit()
}
```

**Benefit:** SQLite backend shares same interface, just implements in Go instead of SQL.

### 8. Version Detection

**Query schema version:**
```sql
SELECT version, name, applied_at 
FROM schema_migrations 
ORDER BY version DESC 
LIMIT 1;
```

**API endpoint:**
```json
["sys.version"]

Response:
{
  "schema": 2,
  "schemaName": "add_namespaces",
  "appliedAt": "2024-12-17T01:00:00Z",
  "server": "0.1.0",
  "backend": "postgres"
}
```

---

## Migration Safety Features

### 1. Checksums
- Calculate SHA-256 of migration SQL
- Store in `schema_migrations` table
- Validate on each run (detect tampering)

### 2. Transactions
- Each migration runs in a transaction
- Rollback on failure
- Atomic apply/rollback

### 3. Dry Run
```bash
messagedb migrate up --dry-run
# Shows what would be executed without applying
```

### 4. Idempotency
```sql
-- Use IF NOT EXISTS everywhere
CREATE TABLE IF NOT EXISTS ...
CREATE INDEX IF NOT EXISTS ...
CREATE OR REPLACE FUNCTION ...
```

### 5. Locking
```sql
-- Acquire advisory lock during migration
SELECT pg_advisory_lock(123456789);
-- Prevents concurrent migrations
-- Released automatically on connection close
```

---

## Configuration

### Database Connection

**Environment variables:**
```bash
MESSAGEDB_DB_HOST=localhost
MESSAGEDB_DB_PORT=5432
MESSAGEDB_DB_NAME=message_store
MESSAGEDB_DB_USER=message_store
MESSAGEDB_DB_PASSWORD=secret
MESSAGEDB_DB_SSLMODE=disable
```

**Or connection string:**
```bash
messagedb migrate up --dsn="postgres://user:pass@localhost/message_store"
```

**Or config file:**
```yaml
# messagedb.yaml
database:
  host: localhost
  port: 5432
  name: message_store
  user: message_store
  password: secret
  sslmode: disable
```

---

## Migration Libraries

**Recommended: [golang-migrate/migrate](https://github.com/golang-migrate/migrate)**

Pros:
- Industry standard
- Supports Postgres, SQLite, etc.
- CLI + Go library
- Transaction support
- Checksums built-in
- Wide adoption

Alternative: **[pressly/goose](https://github.com/pressly/goose)**

Pros:
- Simpler
- Supports Go migrations (code + SQL)
- Good for complex data migrations

---

## Example Usage

### First-time Setup
```bash
# 1. Start server (migrations run automatically)
messagedb serve --backend=postgres

# Output:
# [MIGRATE] Checking schema...
# [MIGRATE] Applied 001_initial_schema
# [MIGRATE] Applied 002_add_namespaces
# [MIGRATE] Schema up to date (version: 002)
# [SERVER] Listening on :8080
```

### SQLite Mode (for tests/dev)
```bash
messagedb serve --backend=sqlite --db-path=./test.db

# Output:
# [MIGRATE] Checking schema...
# [MIGRATE] Applied 001_initial_schema (SQLite)
# [MIGRATE] Schema up to date (version: 001)
# [SERVER] Listening on :8080
```

### Add New Migration
```bash
# 1. Create files manually:
touch migrations/postgres/003_add_statistics.sql
touch migrations/sqlite/003_add_statistics.sql

# 2. Edit both files with appropriate SQL for each dialect

# 3. Restart server - migration runs automatically
messagedb serve
```

---

## Benefits

1. **Transparent**: Developers never think about migrations
2. **Automatic**: Runs on boot, no manual steps
3. **Dual-backend**: Postgres for production, SQLite for tests
4. **Portable**: Single binary, no bash/psql required
5. **Testable**: Easy to test migrations in CI with SQLite
6. **Reliable**: Transaction-based, atomic
7. **Auditable**: Full history in `schema_migrations` table
8. **Cross-platform**: Works on Windows, Linux, macOS

---

## Startup Flow

```go
// cmd/messagedb/main.go
func main() {
    // 1. Parse config (backend, connection string)
    cfg := loadConfig()
    
    // 2. Connect to database
    db, err := sql.Open(cfg.Backend, cfg.DSN)
    
    // 3. Auto-migrate (TRANSPARENT!)
    var migrationsFS embed.FS
    if cfg.Backend == "postgres" {
        migrationsFS = migrations.PostgresFS
    } else {
        migrationsFS = migrations.SQLiteFS
    }
    
    migrator := migrate.New(db, cfg.Backend, migrationsFS)
    if err := migrator.AutoMigrate(); err != nil {
        log.Fatalf("Migration failed: %v", err)
    }
    
    // 4. Initialize store
    var store store.Store
    if cfg.Backend == "postgres" {
        store = postgres.New(db)
    } else {
        store = sqlite.New(db)
    }
    
    // 5. Start HTTP server
    server := api.New(store)
    server.Listen(":8080")
}
```

---

## Future Enhancements

1. **Migration testing**: Run up+down in test database, validate
2. **Data migrations**: Go-based migrations for complex data transforms
3. **Multi-tenant migrations**: Apply to multiple namespaces
4. **Backup integration**: Auto-backup before migration
5. **Slack/webhook notifications**: Alert on migration success/failure

---

## References

- [golang-migrate/migrate](https://github.com/golang-migrate/migrate)
- [Postgres Advisory Locks](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS)
- [Database Migration Best Practices](https://www.prisma.io/dataguide/types/relational/what-are-database-migrations)
