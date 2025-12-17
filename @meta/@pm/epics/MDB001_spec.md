# EPIC MDB001: Core Storage & Migrations

## Overview

**Epic ID:** MDB001  
**Name:** Core Storage & Migrations  
**Duration:** 2-3 weeks  
**Status:** pending  
**Priority:** critical  
**Depends On:** None (Foundation Epic)

**Goal:** Establish reliable dual-backend storage layer with automatic migrations, namespace management, and physical data isolation.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│ Store Interface (internal/store/store.go)          │
│ - WriteMessage(namespace, stream, msg)             │
│ - GetStreamMessages(namespace, stream, opts)       │
│ - GetCategoryMessages(namespace, category, opts)   │
│ - CreateNamespace(id, token_hash)                  │
│ - DeleteNamespace(id)                              │
└─────────────────────────────────────────────────────┘
                        │
         ┌──────────────┴──────────────┐
         ▼                             ▼
┌────────────────────┐        ┌────────────────────┐
│ Postgres Backend   │        │ SQLite Backend     │
│                    │        │                    │
│ - Schema per NS    │        │ - DB file per NS   │
│ - Stored procs     │        │ - Go logic         │
│ - Templates        │        │ - In-memory mode   │
└────────────────────┘        └────────────────────┘
         │                             │
         ▼                             ▼
┌────────────────────┐        ┌────────────────────┐
│ PostgreSQL         │        │ SQLite Files       │
│                    │        │                    │
│ message_store      │        │ metadata.db        │
│ ├─ namespaces      │        │ default.db         │
│                    │        │ tenant-a.db        │
│ messagedb_default  │        └────────────────────┘
│ ├─ messages        │
│ ├─ write_message() │
│                    │
│ messagedb_tenant_a │
│ ├─ messages        │
│ ├─ write_message() │
└────────────────────┘
```

## Technical Requirements

### Database Structures

**Postgres - Metadata Schema:**
```sql
CREATE SCHEMA IF NOT EXISTS message_store;

CREATE TABLE message_store.namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  schema_name TEXT NOT NULL UNIQUE,
  description TEXT,
  created_at BIGINT NOT NULL,
  metadata JSONB
);
```

**Postgres - Namespace Schema (Template):**
```sql
-- Applied with {{SCHEMA_NAME}} replacement

CREATE SCHEMA IF NOT EXISTS "{{SCHEMA_NAME}}";

CREATE TABLE "{{SCHEMA_NAME}}".messages (
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

CREATE UNIQUE INDEX messages_id ON "{{SCHEMA_NAME}}".messages (id);
CREATE UNIQUE INDEX messages_stream ON "{{SCHEMA_NAME}}".messages (stream_name, position);
CREATE INDEX messages_category ON "{{SCHEMA_NAME}}".messages (
  (SPLIT_PART(stream_name, '-', 1)),
  global_position,
  (metadata->>'correlationStreamName')
);

-- Stored procedures
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".write_message(...) RETURNS BIGINT AS $$
  -- Implementation
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_stream_messages(...) RETURNS TABLE(...) AS $$
  -- Implementation
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_category_messages(...) RETURNS TABLE(...) AS $$
  -- Implementation
$$ LANGUAGE plpgsql;
```

**SQLite - Metadata Database:**
```sql
CREATE TABLE IF NOT EXISTS namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  db_path TEXT NOT NULL UNIQUE,
  description TEXT,
  created_at INTEGER NOT NULL,
  metadata TEXT  -- JSON as TEXT
);
```

**SQLite - Namespace Database:**
```sql
CREATE TABLE IF NOT EXISTS messages (
  id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  type TEXT NOT NULL,
  position INTEGER NOT NULL,
  global_position INTEGER PRIMARY KEY AUTOINCREMENT,
  data TEXT,  -- JSON as TEXT
  metadata TEXT,  -- JSON as TEXT
  time INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS messages_id ON messages (id);
CREATE UNIQUE INDEX IF NOT EXISTS messages_stream ON messages (stream_name, position);
CREATE INDEX IF NOT EXISTS messages_category ON messages (
  substr(stream_name, 1, instr(stream_name || '-', '-') - 1),
  global_position
);
```

## Functional Requirements

### FR-1: Store Interface

```go
package store

type Store interface {
    // Message Operations
    WriteMessage(ctx context.Context, namespace, streamName string, msg *Message) (*WriteResult, error)
    GetStreamMessages(ctx context.Context, namespace, streamName string, opts *GetOpts) ([]*Message, error)
    GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *CategoryOpts) ([]*Message, error)
    GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*Message, error)
    GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error)
    
    // Namespace Operations
    CreateNamespace(ctx context.Context, id, tokenHash, description string) error
    DeleteNamespace(ctx context.Context, id string) error
    GetNamespace(ctx context.Context, id string) (*Namespace, error)
    ListNamespaces(ctx context.Context) ([]*Namespace, error)
    
    // Lifecycle
    Close() error
}

type Message struct {
    ID             string
    StreamName     string
    Type           string
    Position       int64
    GlobalPosition int64
    Data           map[string]interface{}
    Metadata       map[string]interface{}
    Time           time.Time
}

type WriteResult struct {
    Position       int64
    GlobalPosition int64
}

type GetOpts struct {
    Position       int64
    GlobalPosition *int64
    BatchSize      int64
    Condition      *string
}

type CategoryOpts struct {
    Position       int64
    GlobalPosition *int64
    BatchSize      int64
    Correlation    *string
    ConsumerMember *int64
    ConsumerSize   *int64
    Condition      *string
}

type Namespace struct {
    ID          string
    Description string
    CreatedAt   time.Time
    Metadata    map[string]interface{}
}
```

### FR-2: Migration System

```go
package migrate

type Migrator struct {
    db      interface{} // *sql.DB or connection
    dialect string      // "postgres" or "sqlite"
    fs      embed.FS
}

func New(db interface{}, dialect string, migrationsFS embed.FS) *Migrator

// AutoMigrate runs all pending migrations
func (m *Migrator) AutoMigrate() error {
    // 1. Ensure schema_migrations table exists
    // 2. Load migration files from embedded FS
    // 3. Check which migrations already applied
    // 4. Apply pending migrations in order
    // 5. Record in schema_migrations table
}

// For namespace migrations with template substitution
func (m *Migrator) ApplyNamespaceMigration(schemaName string) error {
    // 1. Load namespace migration template
    // 2. Replace {{SCHEMA_NAME}} with actual name
    // 3. Execute SQL
}
```

### FR-3: Postgres Backend

```go
package postgres

type PostgresStore struct {
    db           *sql.DB
    metadataConn *sql.DB
}

func New(metadataDB *sql.DB) *PostgresStore

func (s *PostgresStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *Message) (*WriteResult, error) {
    // 1. Get schema name for namespace
    schemaName, err := s.getSchemaName(namespace)
    
    // 2. Call stored procedure in namespace schema
    var position, globalPosition int64
    err = s.db.QueryRowContext(ctx,
        fmt.Sprintf(`SELECT "%s".write_message($1, $2, $3, $4, $5, $6)`, schemaName),
        msg.ID, streamName, msg.Type, msg.Data, msg.Metadata, msg.ExpectedVersion,
    ).Scan(&position)
    
    // 3. Query global_position
    err = s.db.QueryRowContext(ctx,
        fmt.Sprintf(`SELECT global_position FROM "%s".messages WHERE stream_name = $1 AND position = $2`, schemaName),
        streamName, position,
    ).Scan(&globalPosition)
    
    return &WriteResult{Position: position, GlobalPosition: globalPosition}, nil
}

func (s *PostgresStore) CreateNamespace(ctx context.Context, id, tokenHash, description string) error {
    tx, _ := s.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 1. Generate schema name
    schemaName := fmt.Sprintf("messagedb_%s", sanitize(id))
    
    // 2. Create schema
    _, err := tx.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schemaName))
    
    // 3. Apply namespace migrations (template substitution)
    migrator := migrate.New(tx, "postgres", migrations.NamespaceFS)
    err = migrator.ApplyNamespaceMigration(schemaName)
    
    // 4. Insert into message_store.namespaces
    _, err = tx.ExecContext(ctx,
        `INSERT INTO message_store.namespaces (id, token_hash, schema_name, description, created_at)
         VALUES ($1, $2, $3, $4, $5)`,
        id, tokenHash, schemaName, description, time.Now().Unix(),
    )
    
    return tx.Commit()
}

func (s *PostgresStore) DeleteNamespace(ctx context.Context, id string) error {
    tx, _ := s.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 1. Get schema name
    var schemaName string
    err := tx.QueryRowContext(ctx,
        `SELECT schema_name FROM message_store.namespaces WHERE id = $1`,
        id,
    ).Scan(&schemaName)
    
    // 2. Drop schema (CASCADE removes everything)
    _, err = tx.ExecContext(ctx, fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schemaName))
    
    // 3. Remove from registry
    _, err = tx.ExecContext(ctx,
        `DELETE FROM message_store.namespaces WHERE id = $1`,
        id,
    )
    
    return tx.Commit()
}
```

### FR-4: SQLite Backend

```go
package sqlite

type SQLiteStore struct {
    metadataDB   *sql.DB
    namespaceDBs map[string]*sql.DB
    testMode     bool
    mu           sync.RWMutex
}

func New(metadataDB *sql.DB, testMode bool) *SQLiteStore

func (s *SQLiteStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *Message) (*WriteResult, error) {
    // 1. Get or create namespace DB
    nsDB, err := s.getOrCreateNamespaceDB(namespace)
    
    // 2. Implement write_message logic in Go (no stored procedures)
    tx, _ := nsDB.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 3. Get current stream version
    var currentVersion int64
    tx.QueryRowContext(ctx,
        `SELECT COALESCE(MAX(position), -1) FROM messages WHERE stream_name = ?`,
        streamName,
    ).Scan(&currentVersion)
    
    // 4. Check expected version if provided
    if msg.ExpectedVersion != nil && *msg.ExpectedVersion != currentVersion {
        return nil, ErrVersionConflict
    }
    
    // 5. Calculate next position
    nextPosition := currentVersion + 1
    
    // 6. Serialize JSON data
    dataJSON, _ := json.Marshal(msg.Data)
    metaJSON, _ := json.Marshal(msg.Metadata)
    
    // 7. Insert message
    result, err := tx.ExecContext(ctx,
        `INSERT INTO messages (id, stream_name, type, position, data, metadata, time)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
        msg.ID, streamName, msg.Type, nextPosition, string(dataJSON), string(metaJSON), time.Now().Unix(),
    )
    
    globalPosition, _ := result.LastInsertId()
    
    tx.Commit()
    
    return &WriteResult{Position: nextPosition, GlobalPosition: globalPosition}, nil
}

func (s *SQLiteStore) getOrCreateNamespaceDB(namespace string) (*sql.DB, error) {
    s.mu.RLock()
    if db, exists := s.namespaceDBs[namespace]; exists {
        s.mu.RUnlock()
        return db, nil
    }
    s.mu.RUnlock()
    
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Check again after acquiring write lock
    if db, exists := s.namespaceDBs[namespace]; exists {
        return db, nil
    }
    
    // Determine connection string
    var connString string
    if s.testMode {
        connString = fmt.Sprintf("file:%s?mode=memory&cache=shared", namespace)
    } else {
        connString = fmt.Sprintf("/tmp/messagedb-%s.db", namespace)
    }
    
    // Open database
    db, err := sql.Open("sqlite3", connString)
    if err != nil {
        return nil, err
    }
    
    // Apply namespace migrations
    migrator := migrate.New(db, "sqlite", migrations.NamespaceFS)
    if err := migrator.AutoMigrate(); err != nil {
        db.Close()
        return nil, err
    }
    
    s.namespaceDBs[namespace] = db
    return db, nil
}

func (s *SQLiteStore) CreateNamespace(ctx context.Context, id, tokenHash, description string) error {
    // 1. Insert into metadata database
    var dbPath string
    if s.testMode {
        dbPath = fmt.Sprintf("memory:%s", id)
    } else {
        dbPath = fmt.Sprintf("/tmp/messagedb-%s.db", id)
    }
    
    _, err := s.metadataDB.ExecContext(ctx,
        `INSERT INTO namespaces (id, token_hash, db_path, description, created_at)
         VALUES (?, ?, ?, ?, ?)`,
        id, tokenHash, dbPath, description, time.Now().Unix(),
    )
    
    // 2. Create namespace DB (lazy-loaded on first use)
    // Just inserting into metadata is enough; DB created on first access
    
    return err
}

func (s *SQLiteStore) DeleteNamespace(ctx context.Context, id string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // 1. Get db_path
    var dbPath string
    err := s.metadataDB.QueryRowContext(ctx,
        `SELECT db_path FROM namespaces WHERE id = ?`,
        id,
    ).Scan(&dbPath)
    
    // 2. Close connection if open
    if db, exists := s.namespaceDBs[id]; exists {
        db.Close()
        delete(s.namespaceDBs, id)
    }
    
    // 3. Delete file (if not in-memory)
    if !s.testMode && !strings.HasPrefix(dbPath, "memory:") {
        os.Remove(dbPath)
    }
    
    // 4. Remove from metadata
    _, err = s.metadataDB.ExecContext(ctx,
        `DELETE FROM namespaces WHERE id = ?`,
        id,
    )
    
    return err
}
```

## Implementation Strategy

### Phase 1: Migration System (2-3 days)
- Create migration directory structure
- Implement embed.go for FS
- Build Migrator with AutoMigrate
- Add schema_migrations tracking table
- Support template substitution for namespaces
- Test with both Postgres and SQLite

### Phase 2: Store Interface & Types (1 day)
- Define Store interface
- Define Message, WriteResult, Opts types
- Define Namespace type
- Export from package

### Phase 3: Postgres Backend (4-5 days)
- Implement PostgresStore struct
- Metadata schema migrations
- Namespace schema template migrations
- Implement all Message operations (use stored procedures)
- Implement all Namespace operations
- Connection management
- Error handling

### Phase 4: SQLite Backend (4-5 days)
- Implement SQLiteStore struct
- Metadata database migrations
- Namespace database migrations
- Implement all Message operations (Go logic)
- Implement all Namespace operations
- In-memory mode support
- File-based mode support
- Connection pooling per namespace
- Error handling

### Phase 5: Integration & Testing (3-4 days)
- Unit tests for both backends
- Integration tests (same test suite for both)
- Test namespace isolation
- Test optimistic locking
- Test category queries with consumer groups
- Performance benchmarking
- Documentation

## Acceptance Criteria

### AC-1: Auto-Migration Works
- **GIVEN** Fresh database
- **WHEN** Server starts
- **THEN** All migrations applied automatically

### AC-2: Namespace Creation
- **GIVEN** Valid namespace ID and token
- **WHEN** CreateNamespace is called
- **THEN** Namespace schema/DB created with full Message DB structure

### AC-3: Physical Isolation
- **GIVEN** Multiple namespaces
- **WHEN** Writing to namespace A
- **THEN** Data not visible in namespace B

### AC-4: Write Message Works
- **GIVEN** Namespace and stream
- **WHEN** WriteMessage is called
- **THEN** Message stored with correct position and global_position

### AC-5: Optimistic Locking
- **GIVEN** Stream at version 5
- **WHEN** WriteMessage with expected_version=4
- **THEN** Returns version conflict error

### AC-6: Category Queries Work
- **GIVEN** Multiple streams in same category
- **WHEN** GetCategoryMessages is called
- **THEN** Returns messages from all streams in category

### AC-7: Consumer Groups Work
- **GIVEN** Category query with consumer_member=0, consumer_size=2
- **WHEN** GetCategoryMessages is called
- **THEN** Returns only messages for that consumer partition

### AC-8: Namespace Deletion
- **GIVEN** Namespace with data
- **WHEN** DeleteNamespace is called
- **THEN** Schema/DB dropped, all data removed

### AC-9: Backend Parity
- **GIVEN** Same operations on Postgres and SQLite
- **WHEN** Both backends used
- **THEN** Identical results (within type conversions)

### AC-10: Test Mode Works
- **GIVEN** SQLite backend with testMode=true
- **WHEN** Operations performed
- **THEN** All data in-memory, fast execution

## Definition of Done

- [ ] Migration system with AutoMigrate implemented
- [ ] Metadata migrations for both backends
- [ ] Namespace migrations with template support
- [ ] Store interface defined
- [ ] Postgres backend fully implemented
- [ ] SQLite backend fully implemented
- [ ] All Message operations work (write, read, last, version)
- [ ] All Category operations work (query, consumer groups)
- [ ] All Namespace operations work (create, delete, list, get)
- [ ] Optimistic locking enforced
- [ ] Physical isolation verified
- [ ] Test mode (in-memory SQLite) working
- [ ] Both backends pass same test suite
- [ ] Error handling comprehensive
- [ ] Performance benchmarks meet targets
- [ ] Code documented with comments
- [ ] Integration tests passing

## Performance Expectations

| Operation | Postgres | SQLite (file) | SQLite (memory) |
|-----------|----------|---------------|-----------------|
| WriteMessage | <10ms | <5ms | <1ms |
| GetStreamMessages (10 msgs) | <15ms | <8ms | <2ms |
| GetCategoryMessages (100 msgs) | <50ms | <30ms | <10ms |
| CreateNamespace | <100ms | <50ms | <20ms |
| DeleteNamespace | <200ms | <100ms | <50ms |

## Non-Goals

- ❌ Clustering / Replication
- ❌ Query optimization beyond indexes
- ❌ Custom stored procedures (only Message DB functions)
- ❌ Schema versioning/migrations beyond initial
- ❌ Backup/restore functionality
- ❌ Cross-namespace queries
