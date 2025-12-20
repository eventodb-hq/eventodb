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
│                                                     │
│ Utility Functions:                                  │
│ - Category(streamName) → category                  │
│ - ID(streamName) → id                              │
│ - CardinalID(streamName) → cardinalId              │
│ - IsCategory(name) → bool                          │
│ - Hash64(value) → int64                            │
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
│ - Advisory locks   │        │ - Tx-level locks   │
└────────────────────┘        └────────────────────┘
         │                             │
         ▼                             ▼
┌────────────────────┐        ┌────────────────────┐
│ PostgreSQL         │        │ SQLite Files       │
│                    │        │                    │
│ message_store      │        │ metadata.db        │
│ ├─ namespaces      │        │ default.db         │
│                    │        │ tenant-a.db        │
│ eventodb_default  │        └────────────────────┘
│ ├─ messages        │
│ ├─ write_message() │
│ ├─ hash_64()       │
│ ├─ acquire_lock()  │
│                    │
│ eventodb_tenant_a │
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

-- Utility Functions (compatible with EventoDB)
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".hash_64(value VARCHAR) RETURNS BIGINT AS $$
DECLARE
  _hash BIGINT;
BEGIN
  -- Uses MD5, takes left 64 bits (8 bytes), converts to bigint
  -- Compatible with EventoDB hash implementation
  SELECT left('x' || md5(value), 17)::bit(64)::bigint INTO _hash;
  RETURN _hash;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".category(stream_name VARCHAR) RETURNS VARCHAR AS $$
BEGIN
  RETURN SPLIT_PART(stream_name, '-', 1);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".id(stream_name VARCHAR) RETURNS VARCHAR AS $$
DECLARE
  _id_separator_position INTEGER;
BEGIN
  _id_separator_position := STRPOS(stream_name, '-');
  IF _id_separator_position = 0 THEN
    RETURN NULL;
  END IF;
  RETURN SUBSTRING(stream_name, _id_separator_position + 1);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".cardinal_id(stream_name VARCHAR) RETURNS VARCHAR AS $$
DECLARE
  _id VARCHAR;
BEGIN
  _id := "{{SCHEMA_NAME}}".id(stream_name);
  IF _id IS NULL THEN
    RETURN NULL;
  END IF;
  -- Extract part before '+' for compound IDs (e.g., '123+456' -> '123')
  RETURN SPLIT_PART(_id, '+', 1);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".is_category(stream_name VARCHAR) RETURNS BOOLEAN AS $$
BEGIN
  RETURN STRPOS(stream_name, '-') = 0;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".acquire_lock(stream_name VARCHAR) RETURNS BIGINT AS $$
DECLARE
  _category VARCHAR;
  _category_name_hash BIGINT;
BEGIN
  _category := "{{SCHEMA_NAME}}".category(stream_name);
  _category_name_hash := "{{SCHEMA_NAME}}".hash_64(_category);
  -- Advisory lock at CATEGORY level (not stream level)
  PERFORM pg_advisory_xact_lock(_category_name_hash);
  RETURN _category_name_hash;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- Core Message Operations
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".write_message(...) RETURNS BIGINT AS $$
  -- Implementation includes acquire_lock() call
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_stream_messages(...) RETURNS TABLE(...) AS $$
  -- Implementation
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_category_messages(...) RETURNS TABLE(...) AS $$
  -- Implementation uses cardinal_id() for consumer groups
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
    
    // Utility Functions (EventoDB compatible)
    Category(streamName string) string
    ID(streamName string) string
    CardinalID(streamName string) string
    IsCategory(name string) bool
    Hash64(value string) int64
    
    // Lifecycle
    Close() error
}

type Message struct {
    ID             string                 // UUID v4 (RFC 4122)
    StreamName     string                 // Format: category-id or category-cardinalId+compoundPart
    Type           string                 // Message type name
    Position       int64                  // Stream position (gapless, 0-indexed)
    GlobalPosition int64                  // Global position (may have gaps)
    Data           map[string]interface{} // Message payload (JSON)
    Metadata       map[string]interface{} // Message metadata (JSON)
    Time           time.Time              // UTC timestamp (no timezone)
}

// Standard metadata fields (EventoDB compatible)
type StandardMetadata struct {
    CorrelationStreamName string `json:"correlationStreamName,omitempty"` // For category correlation filtering
    // Other standard fields can be added here
}

type WriteResult struct {
    Position       int64
    GlobalPosition int64
}

type GetOpts struct {
    Position       int64   // Stream position (default: 0)
    GlobalPosition *int64  // Alternative: global position (mutually exclusive with Position)
    BatchSize      int64   // Number of messages (default: 1000, -1 for unlimited)
    Condition      *string // DEPRECATED: SQL condition (do not implement - security risk)
}

type CategoryOpts struct {
    Position       int64   // Global position for category (default: 1)
    GlobalPosition *int64  // Alternative (same as Position for categories)
    BatchSize      int64   // Number of messages (default: 1000, -1 for unlimited)
    Correlation    *string // Filter by metadata.correlationStreamName category
    ConsumerMember *int64  // Consumer group member number (0-indexed)
    ConsumerSize   *int64  // Consumer group total size
    Condition      *string // DEPRECATED: SQL condition (do not implement - security risk)
}

// Consumer group assignment: MOD(ABS(hash_64(cardinal_id(stream_name))), consumer_size) = consumer_member

type Namespace struct {
    ID          string
    Description string
    CreatedAt   time.Time
    Metadata    map[string]interface{}
}
```

### FR-2: Utility Functions (EventoDB Compatible)

```go
package store

// Category extracts the category name from a stream name
// Examples:
//   Category("account-123") → "account"
//   Category("account-123+456") → "account"
//   Category("account") → "account"
func Category(streamName string) string {
    parts := strings.SplitN(streamName, "-", 2)
    return parts[0]
}

// ID extracts the ID portion from a stream name
// Examples:
//   ID("account-123") → "123"
//   ID("account-123+456") → "123+456"
//   ID("account") → ""
func ID(streamName string) string {
    parts := strings.SplitN(streamName, "-", 2)
    if len(parts) < 2 {
        return ""
    }
    return parts[1]
}

// CardinalID extracts the cardinal ID (before '+') from a stream name
// Used for consumer group partitioning with compound IDs
// Examples:
//   CardinalID("account-123") → "123"
//   CardinalID("account-123+456") → "123"
//   CardinalID("account") → ""
func CardinalID(streamName string) string {
    id := ID(streamName)
    if id == "" {
        return ""
    }
    // Extract part before '+' for compound IDs
    parts := strings.SplitN(id, "+", 2)
    return parts[0]
}

// IsCategory determines if a name represents a category (no ID part)
// Examples:
//   IsCategory("account") → true
//   IsCategory("account-123") → false
func IsCategory(name string) bool {
    return !strings.Contains(name, "-")
}

// Hash64 computes a 64-bit hash compatible with EventoDB
// Uses MD5, takes first 8 bytes, converts to int64
// CRITICAL: Must produce identical results to EventoDB for consumer group compatibility
func Hash64(value string) int64 {
    hash := md5.Sum([]byte(value))
    // Take first 8 bytes of MD5 hash
    return int64(binary.BigEndian.Uint64(hash[:8]))
}

// ConsumerGroupMember determines which consumer group member should handle a stream
// Returns true if the given stream should be handled by the specified consumer member
func IsAssignedToConsumerMember(streamName string, member, size int64) bool {
    if size <= 0 || member < 0 || member >= size {
        return false
    }
    cardinalID := CardinalID(streamName)
    if cardinalID == "" {
        return false
    }
    hash := Hash64(cardinalID)
    // Use absolute value to handle negative hashes
    if hash < 0 {
        hash = -hash
    }
    return (hash % size) == member
}
```

### FR-3: Migration System

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
    // 3. Execute SQL (includes utility functions + message operations)
}
```

### FR-4: Postgres Backend

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
    
    // 2. Generate UUID if not provided
    if msg.ID == "" {
        msg.ID = uuid.New().String()
    }
    
    // 3. Call stored procedure in namespace schema
    // Note: write_message() internally calls acquire_lock() for category-level locking
    var position, globalPosition int64
    err = s.db.QueryRowContext(ctx,
        fmt.Sprintf(`SELECT "%s".write_message($1, $2, $3, $4, $5, $6)`, schemaName),
        msg.ID, streamName, msg.Type, msg.Data, msg.Metadata, msg.ExpectedVersion,
    ).Scan(&position)
    
    // 4. Query global_position
    err = s.db.QueryRowContext(ctx,
        fmt.Sprintf(`SELECT global_position FROM "%s".messages WHERE stream_name = $1 AND position = $2`, schemaName),
        streamName, position,
    ).Scan(&globalPosition)
    
    return &WriteResult{Position: position, GlobalPosition: globalPosition}, nil
}

// Utility function implementations (delegate to SQL for Postgres)
func (s *PostgresStore) Category(streamName string) string {
    return Category(streamName) // Use Go implementation
}

func (s *PostgresStore) ID(streamName string) string {
    return ID(streamName)
}

func (s *PostgresStore) CardinalID(streamName string) string {
    return CardinalID(streamName)
}

func (s *PostgresStore) IsCategory(name string) bool {
    return IsCategory(name)
}

func (s *PostgresStore) Hash64(value string) int64 {
    return Hash64(value)
}

func (s *PostgresStore) CreateNamespace(ctx context.Context, id, tokenHash, description string) error {
    tx, _ := s.db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 1. Generate schema name
    schemaName := fmt.Sprintf("eventodb_%s", sanitize(id))
    
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

### FR-5: SQLite Backend

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
    
    // 2. Generate UUID if not provided
    if msg.ID == "" {
        msg.ID = uuid.New().String()
    }
    
    // 3. Implement write_message logic in Go (no stored procedures)
    // SQLite uses transaction-level locking (simpler than Postgres advisory locks)
    tx, _ := nsDB.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 4. Get current stream version
    var currentVersion int64
    tx.QueryRowContext(ctx,
        `SELECT COALESCE(MAX(position), -1) FROM messages WHERE stream_name = ?`,
        streamName,
    ).Scan(&currentVersion)
    
    // 5. Check expected version if provided (optimistic locking)
    if msg.ExpectedVersion != nil && *msg.ExpectedVersion != currentVersion {
        return nil, ErrVersionConflict
    }
    
    // 6. Calculate next position
    nextPosition := currentVersion + 1
    
    // 7. Serialize JSON data
    dataJSON, _ := json.Marshal(msg.Data)
    metaJSON, _ := json.Marshal(msg.Metadata)
    
    // 8. Insert message
    result, err := tx.ExecContext(ctx,
        `INSERT INTO messages (id, stream_name, type, position, data, metadata, time)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
        msg.ID, streamName, msg.Type, nextPosition, string(dataJSON), string(metaJSON), time.Now().Unix(),
    )
    
    globalPosition, _ := result.LastInsertId()
    
    tx.Commit()
    
    return &WriteResult{Position: nextPosition, GlobalPosition: globalPosition}, nil
}

// Utility function implementations (pure Go)
func (s *SQLiteStore) Category(streamName string) string {
    return Category(streamName)
}

func (s *SQLiteStore) ID(streamName string) string {
    return ID(streamName)
}

func (s *SQLiteStore) CardinalID(streamName string) string {
    return CardinalID(streamName)
}

func (s *SQLiteStore) IsCategory(name string) bool {
    return IsCategory(name)
}

func (s *SQLiteStore) Hash64(value string) int64 {
    return Hash64(value)
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
        connString = fmt.Sprintf("/tmp/eventodb-%s.db", namespace)
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
        dbPath = fmt.Sprintf("/tmp/eventodb-%s.db", id)
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

### FR-6: Consumer Group Implementation

Consumer groups partition streams across multiple consumers using deterministic hashing.

**Algorithm (EventoDB Compatible):**
```
For each message in a category:
  1. Extract cardinal_id from stream_name
     - "account-123" → "123"
     - "account-123+456" → "123" (compound ID)
  2. hash = hash_64(cardinal_id)
  3. member = ABS(hash) MOD consumer_group_size
  4. Include message if member == consumer_group_member
```

**Postgres Implementation:**
```sql
-- In get_category_messages stored procedure
WHERE
  category(stream_name) = $1 AND
  global_position >= $2 AND
  MOD(@hash_64(cardinal_id(stream_name)), $6) = $5
```

**SQLite Implementation (Go):**
```go
func (s *SQLiteStore) GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *CategoryOpts) ([]*Message, error) {
    nsDB, _ := s.getOrCreateNamespaceDB(namespace)
    
    query := `
        SELECT id, stream_name, type, position, global_position, data, metadata, time
        FROM messages
        WHERE substr(stream_name, 1, instr(stream_name || '-', '-') - 1) = ?
          AND global_position >= ?
    `
    
    args := []interface{}{categoryName, opts.Position}
    
    // Add consumer group filtering if specified
    if opts.ConsumerMember != nil && opts.ConsumerSize != nil {
        // Filter in Go after query (or use WHERE clause with custom function)
    }
    
    rows, _ := nsDB.QueryContext(ctx, query, args...)
    defer rows.Close()
    
    messages := []*Message{}
    for rows.Next() {
        msg := &Message{}
        // Scan into message
        
        // Apply consumer group filter if specified
        if opts.ConsumerMember != nil && opts.ConsumerSize != nil {
            if !IsAssignedToConsumerMember(msg.StreamName, *opts.ConsumerMember, *opts.ConsumerSize) {
                continue // Skip this message
            }
        }
        
        messages = append(messages, msg)
        if len(messages) >= int(opts.BatchSize) {
            break
        }
    }
    
    return messages, nil
}
```

**Critical Compatibility Notes:**
1. Hash function MUST produce identical results to EventoDB
2. Cardinal ID extraction MUST handle compound IDs (`+` separator)
3. Consumer assignment MUST be deterministic across restarts
4. Test with EventoDB reference data to verify compatibility

## Implementation Strategy

### Phase 1: Utility Functions & Hashing (2 days)
- Implement Category, ID, CardinalID, IsCategory functions
- Implement Hash64 with MD5-based algorithm
- Test against EventoDB reference data
- Verify consumer group assignments match

### Phase 2: Migration System (2-3 days)
- Create migration directory structure
- Implement embed.go for FS
- Build Migrator with AutoMigrate
- Add schema_migrations tracking table
- Support template substitution for namespaces
- Include utility functions in namespace migrations
- Test with both Postgres and SQLite

### Phase 3: Store Interface & Types (1 day)
- Define Store interface with utility functions
- Define Message, WriteResult, Opts types
- Define Namespace type
- Document compound ID format
- Export from package

### Phase 4: Postgres Backend (4-5 days)
- Implement PostgresStore struct
- Metadata schema migrations
- Namespace schema template migrations (with utility functions)
- Implement all Message operations (use stored procedures)
- Implement advisory lock support
- Implement all Namespace operations
- Connection management
- Error handling

### Phase 5: SQLite Backend (4-5 days)
- Implement SQLiteStore struct
- Metadata database migrations
- Namespace database migrations
- Implement all Message operations (Go logic)
- Implement consumer group filtering in Go
- Implement all Namespace operations
- In-memory mode support
- File-based mode support
- Connection pooling per namespace
- Error handling

### Phase 6: Integration & Testing (3-4 days)
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
- **THEN** Namespace schema/DB created with full EventoDB structure

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

### AC-11: Utility Functions Work
- **GIVEN** Stream name "account-123+456"
- **WHEN** Utility functions called
- **THEN** Category() → "account", ID() → "123+456", CardinalID() → "123"

### AC-12: Hash Function Compatible
- **GIVEN** Same input value
- **WHEN** Hash64 called in Go vs EventoDB Postgres
- **THEN** Produces identical int64 result

### AC-13: Compound IDs with Consumer Groups
- **GIVEN** Streams "account-123+abc" and "account-123+def"
- **WHEN** Consumer group query executed
- **THEN** Both streams assigned to SAME consumer (based on cardinal ID "123")

### AC-14: Advisory Locks Prevent Conflicts
- **GIVEN** Concurrent writes to same category
- **WHEN** Multiple WriteMessage calls in parallel
- **THEN** Category-level lock prevents race conditions

## Definition of Done

- [ ] Utility functions implemented (Category, ID, CardinalID, IsCategory, Hash64)
- [ ] Hash64 produces identical results to EventoDB (verified with test data)
- [ ] Migration system with AutoMigrate implemented
- [ ] Metadata migrations for both backends
- [ ] Namespace migrations with template support (includes utility functions)
- [ ] Store interface defined with utility function methods
- [ ] Postgres backend fully implemented
- [ ] Postgres advisory locks working (category-level)
- [ ] SQLite backend fully implemented
- [ ] SQLite transaction locking working
- [ ] All Message operations work (write, read, last, version)
- [ ] All Category operations work (query, consumer groups)
- [ ] Consumer groups use cardinal_id for compound ID support
- [ ] All Namespace operations work (create, delete, list, get)
- [ ] Optimistic locking enforced
- [ ] Physical isolation verified
- [ ] Compound ID support tested
- [ ] Test mode (in-memory SQLite) working
- [ ] Both backends pass same test suite
- [ ] Error handling comprehensive
- [ ] Performance benchmarks meet targets
- [ ] Code documented with comments
- [ ] Integration tests passing
- [ ] Compatibility with EventoDB verified

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
- ❌ Custom stored procedures (only EventoDB functions)
- ❌ Schema versioning/migrations beyond initial
- ❌ Backup/restore functionality
- ❌ Cross-namespace queries
- ❌ SQL condition parameter (security risk, skip this EventoDB feature)
- ❌ Debug mode / NOTICE logging (can add later if needed)
- ❌ Reporting views (can add as separate API endpoints later)
