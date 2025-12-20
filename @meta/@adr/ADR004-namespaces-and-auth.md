# ADR-004: Namespaces and Authentication

**Date:** 2024-12-17  
**Status:** Accepted  
**Context:** Multi-tenancy via namespaces requires proper lifecycle management and access control

---

## Decision

Implement **namespace-based multi-tenancy** with **token-based authentication**.

---

## Core Concepts

### 1. Namespace Lifecycle

Namespaces are **not ephemeral** - they must be explicitly created and cleaned up.

```json
// Create namespace
["ns.create", "tenant-a", {"description": "Tenant A production"}]

Response:
{
  "namespace": "tenant-a",
  "token": "ns_abc123xyz789...",
  "createdAt": "2024-12-17T01:00:00Z"
}

// Delete namespace (requires token)
["ns.delete", "tenant-a"]

Response:
{
  "namespace": "tenant-a",
  "deletedAt": "2024-12-17T02:00:00Z"
}

// List namespaces (admin only)
["ns.list"]

Response:
[
  {"namespace": "default", "createdAt": "2024-12-17T00:00:00Z"},
  {"namespace": "tenant-a", "createdAt": "2024-12-17T01:00:00Z"}
]
```

---

### 2. Authentication Model

**Every namespace has a token.** Token must be provided in requests.

#### Token Format
```
ns_<base64(namespace_id)>_<random_32_bytes_hex>

Example: ns_dGVuYW50LWE_a7f3c8d9e2b1f4a6c8e9d2b3f5a7c9e1
```

#### Token Storage
```sql
-- migrations/postgres/001_initial_schema.sql
CREATE TABLE IF NOT EXISTS namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,  -- SHA-256 of token
  description TEXT,
  created_at BIGINT NOT NULL,
  metadata JSONB
);

-- Default namespace always exists
INSERT INTO namespaces (id, token_hash, description, created_at)
VALUES (
  'default',
  '<hash of default token>',
  'Default namespace',
  EXTRACT(EPOCH FROM NOW())
) ON CONFLICT (id) DO NOTHING;
```

---

### 3. Request Authentication

#### Header-based
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Authorization: Bearer ns_abc123..." \
  -H "Content-Type: application/json" \
  -d '["stream.write", "account-123", {...}]'
```

**Server logic:**
1. Extract token from `Authorization: Bearer <token>`
2. Parse namespace from token prefix
3. Validate token hash matches database
4. Scope all operations to that namespace

#### Stream Name Prefix (Alternative)
```json
// Client includes namespace in stream name
["stream.write", "tenant-a:account-123", {...}]

// Server validates token matches namespace prefix
```

**Decision: Use header-based auth** (cleaner, follows standards)

---

### 4. Default Namespace

The `default` namespace always exists with a well-known token.

```bash
# Server startup
eventodb serve

# Output:
# [MIGRATE] Schema up to date
# [NAMESPACE] Default namespace token: ns_ZGVmYXVsdA_1234567890abcdef...
# [SERVER] Listening on :8080
```

**For development:**
```bash
# Token printed on stdout
export MESSAGEDB_TOKEN="ns_ZGVmYXVsdA_1234567890abcdef..."

curl -H "Authorization: Bearer $MESSAGEDB_TOKEN" \
  http://localhost:8080/rpc \
  -d '["stream.write", "account-1", {...}]'
```

---

### 5. Test Mode

When started with `--test-mode`, server uses in-memory SQLite and relaxed namespace management.

```bash
eventodb serve --test-mode

# Behavior:
# - Backend: SQLite :memory:
# - Auto-creates namespaces on first use
# - Returns tokens in response headers (for tests to grab)
# - Cleans up on shutdown
```

**Test workflow:**
```typescript
// Bun test
const server = await startServer({ testMode: true });

// Create namespace (returns token immediately)
const ns = await server.createNamespace('test-123');
// { namespace: 'test-123', token: 'ns_...' }

const client = new EventoDBClient({
  baseURL: server.url,
  token: ns.token
});

await client.writeMessage('account-1', {...});

// Cleanup
await server.deleteNamespace('test-123');
server.close();
```

---

## API Methods

### ns.create

```json
["ns.create", "tenant-a", {opts}]

// opts (optional):
{
  "description": "Production tenant A",
  "metadata": {"plan": "enterprise"}
}

Response:
{
  "namespace": "tenant-a",
  "token": "ns_dGVuYW50LWE_a7f3c8d9...",
  "createdAt": "2024-12-17T01:00:00Z"
}
```

**Server-side operations:**

**Postgres:**
1. Generate token and hash
2. Create schema: `CREATE SCHEMA "eventodb_tenant_a"`
3. Run namespace migrations (create tables, indexes, functions)
4. Insert record into `message_store.namespaces`

**SQLite:**
1. Generate token and hash
2. Create database file: `/tmp/eventodb-tenant-a.db` (or in-memory)
3. Run namespace migrations on new database
4. Insert record into metadata database `namespaces` table

**Authentication required:** 
- Default namespace token (acts as admin)
- OR another namespace token with `admin: true` metadata

---

### ns.delete

```json
["ns.delete", "tenant-a"]

Response:
{
  "namespace": "tenant-a",
  "deletedAt": "2024-12-17T02:00:00Z",
  "messagesDeleted": 1543
}
```

**Server-side operations:**

**Postgres:**
1. Verify token matches namespace
2. Get schema name from `message_store.namespaces`
3. `DROP SCHEMA "eventodb_tenant_a" CASCADE` (deletes everything)
4. Delete from `message_store.namespaces`

**SQLite:**
1. Verify token matches namespace
2. Get db_path from metadata `namespaces` table
3. Close database connection
4. Delete database file (if not in-memory)
5. Delete from metadata `namespaces` table

**Benefits:** 
- Clean deletion (no orphaned data)
- Fast (single DROP SCHEMA or file delete)
- No cascading cleanup needed

**Authentication required:** Token for that namespace OR admin token

---

### ns.list

```json
["ns.list", {opts}]

// opts (optional):
{
  "limit": 100,
  "offset": 0
}

Response:
[
  {
    "namespace": "default",
    "description": "Default namespace",
    "createdAt": "2024-12-17T00:00:00Z",
    "messageCount": 1234
  },
  {
    "namespace": "tenant-a",
    "description": "Tenant A",
    "createdAt": "2024-12-17T01:00:00Z",
    "messageCount": 567
  }
]
```

**Authentication required:** Admin token (default namespace or flagged namespace)

**Note:** Tokens are NOT returned in list (security)

---

### ns.info

```json
["ns.info", "tenant-a"]

Response:
{
  "namespace": "tenant-a",
  "description": "Tenant A",
  "createdAt": "2024-12-17T01:00:00Z",
  "messageCount": 567,
  "streamCount": 12,
  "lastActivity": "2024-12-17T01:30:00Z"
}
```

**Authentication required:** Token for that namespace

---

### ns.rotateToken

```json
["ns.rotateToken", "tenant-a"]

Response:
{
  "namespace": "tenant-a",
  "token": "ns_dGVuYW50LWE_NEW_TOKEN_HERE...",
  "rotatedAt": "2024-12-17T03:00:00Z"
}
```

**Behavior:**
1. Generate new token
2. Update token_hash in database
3. Old token immediately invalid

**Authentication required:** Current token for that namespace

---

## Architecture Overview

### Postgres Structure

```
┌─────────────────────────────────────────────────────────────┐
│ PostgreSQL Database                                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ message_store (metadata schema)      │                  │
│  ├──────────────────────────────────────┤                  │
│  │ namespaces table:                    │                  │
│  │  - id: 'default'                     │                  │
│  │    token_hash: <hash>                │                  │
│  │    schema_name: 'eventodb_default'  │                  │
│  │  - id: 'tenant-a'                    │                  │
│  │    token_hash: <hash>                │                  │
│  │    schema_name: 'eventodb_tenant_a' │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ eventodb_default (data schema)      │                  │
│  ├──────────────────────────────────────┤                  │
│  │ messages table                       │                  │
│  │ + all EventoDB functions           │                  │
│  │ + indexes                            │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ eventodb_tenant_a (data schema)     │                  │
│  ├──────────────────────────────────────┤                  │
│  │ messages table                       │                  │
│  │ + all EventoDB functions           │                  │
│  │ + indexes                            │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### SQLite Structure (Test Mode)

```
┌─────────────────────────────────────────────────────────────┐
│ In-Memory SQLite Databases                                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ metadata.db (:memory:)               │                  │
│  ├──────────────────────────────────────┤                  │
│  │ namespaces table:                    │                  │
│  │  - id: 'default'                     │                  │
│  │    token_hash: <hash>                │                  │
│  │    db_path: 'memory:default'         │                  │
│  │  - id: 'tenant-a'                    │                  │
│  │    token_hash: <hash>                │                  │
│  │    db_path: 'memory:tenant-a'        │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ memory:default (in-memory DB)        │                  │
│  ├──────────────────────────────────────┤                  │
│  │ messages table                       │                  │
│  │ + indexes                            │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
│  ┌──────────────────────────────────────┐                  │
│  │ memory:tenant-a (in-memory DB)       │                  │
│  ├──────────────────────────────────────┤                  │
│  │ messages table                       │                  │
│  │ + indexes                            │                  │
│  └──────────────────────────────────────┘                  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## Data Isolation

### Stream Names

Streams are NOT prefixed - each namespace has completely isolated storage.

**Client sends:**
```json
["stream.write", "account-123", {...}]
```

**Server stores (Postgres):**
```sql
-- In namespace-specific schema
INSERT INTO "tenant-a".messages (stream_name, ...)
VALUES ('account-123', ...);  -- NO prefix!
```

**Server stores (SQLite in test mode):**
```sql
-- In namespace-specific database file
-- /tmp/eventodb-tenant-a.db
INSERT INTO messages (stream_name, ...)
VALUES ('account-123', ...);
```

**Benefit:** 
- Complete physical isolation
- No prefix pollution
- Easy to drop entire namespace
- Better performance (no prefix filtering)

---

## Database Structure (Postgres)

### Default Schema (message_store)

The `message_store` schema contains **only namespace metadata**:

```sql
CREATE SCHEMA IF NOT EXISTS message_store;

-- Namespace registry (tokens + metadata)
CREATE TABLE message_store.namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  schema_name TEXT NOT NULL UNIQUE,  -- Points to data schema
  description TEXT,
  created_at BIGINT NOT NULL,
  metadata JSONB
);

-- Default namespace entry
INSERT INTO message_store.namespaces (id, token_hash, schema_name, description, created_at)
VALUES ('default', '<hash>', 'eventodb_default', 'Default namespace', <timestamp>);
```

**Key point:** No message data in `message_store` schema!

---

### Per-Namespace Schema

Each namespace gets its own Postgres schema with the full EventoDB structure:

```sql
-- When creating namespace "tenant-a"
CREATE SCHEMA "eventodb_tenant_a";

-- Full EventoDB structure in this schema
CREATE TABLE "eventodb_tenant_a".messages (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  stream_name VARCHAR NOT NULL,  -- Just "account-123", no prefix!
  type VARCHAR NOT NULL,
  position BIGINT NOT NULL,
  global_position BIGSERIAL NOT NULL,
  data JSONB,
  metadata JSONB,
  time TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT (now() AT TIME ZONE 'utc'),
  PRIMARY KEY (global_position)
);

-- Indexes
CREATE UNIQUE INDEX messages_id ON "eventodb_tenant_a".messages (id);
CREATE UNIQUE INDEX messages_stream ON "eventodb_tenant_a".messages (stream_name, position);
CREATE INDEX messages_category ON "eventodb_tenant_a".messages (
  (SPLIT_PART(stream_name, '-', 1)),
  global_position,
  (metadata->>'correlationStreamName')
);

-- Stored procedures (in namespace schema)
CREATE OR REPLACE FUNCTION "eventodb_tenant_a".write_message(...)
RETURNS BIGINT AS $$
  -- Same implementation as EventoDB
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION "eventodb_tenant_a".get_stream_messages(...)
RETURNS TABLE(...) AS $$
  -- Same implementation as EventoDB
$$ LANGUAGE plpgsql;

-- ... all other EventoDB functions
```

**Namespace deletion:**
```sql
-- Drop entire schema (cascades to all tables, functions, data)
DROP SCHEMA "eventodb_tenant_a" CASCADE;

-- Remove from registry
DELETE FROM message_store.namespaces WHERE id = 'tenant-a';
```

---

## SQLite Structure (Test Mode)

### Metadata Database

A single SQLite database holds namespace metadata:

```bash
# Default metadata database
/tmp/eventodb-metadata.db
```

```sql
-- Namespace registry
CREATE TABLE namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  db_path TEXT NOT NULL UNIQUE,  -- Points to namespace database
  description TEXT,
  created_at INTEGER NOT NULL,
  metadata TEXT  -- JSON as TEXT
);

-- Default namespace entry
INSERT INTO namespaces (id, token_hash, db_path, description, created_at)
VALUES ('default', '<hash>', '/tmp/eventodb-default.db', 'Default namespace', <timestamp>);
```

---

### Per-Namespace Database

Each namespace gets its own SQLite database file:

```bash
/tmp/eventodb-default.db      # Default namespace
/tmp/eventodb-tenant-a.db     # Tenant A namespace
/tmp/eventodb-tenant-b.db     # Tenant B namespace
```

**Each database contains full EventoDB structure:**

```sql
-- /tmp/eventodb-tenant-a.db

CREATE TABLE messages (
  id TEXT NOT NULL,
  stream_name TEXT NOT NULL,  -- Just "account-123", no prefix!
  type TEXT NOT NULL,
  position INTEGER NOT NULL,
  global_position INTEGER PRIMARY KEY AUTOINCREMENT,
  data TEXT,  -- JSON
  metadata TEXT,  -- JSON
  time INTEGER NOT NULL
);

CREATE UNIQUE INDEX messages_id ON messages (id);
CREATE UNIQUE INDEX messages_stream ON messages (stream_name, position);
CREATE INDEX messages_category ON messages (
  substr(stream_name, 1, instr(stream_name || '-', '-') - 1),
  global_position
);
```

**Namespace deletion:**
```bash
# Delete database file
rm /tmp/eventodb-tenant-a.db

# Remove from registry
DELETE FROM namespaces WHERE id = 'tenant-a';
```

---

## In-Memory Mode (Test Mode)

When `--test-mode` is enabled:

```bash
eventodb serve --test-mode
```

**Behavior:**
1. **Metadata DB:** In-memory (`:memory:`)
2. **Namespace DBs:** Also in-memory (named connections)

```go
// internal/store/sqlite/store.go

type SQLiteStore struct {
    metadataDB *sql.DB                    // :memory: - namespace registry
    namespaceDBs map[string]*sql.DB       // namespace_id -> in-memory DB
    testMode bool
}

func (s *SQLiteStore) getOrCreateNamespaceDB(namespace string) (*sql.DB, error) {
    if db, exists := s.namespaceDBs[namespace]; exists {
        return db, nil
    }
    
    var connString string
    if s.testMode {
        // In-memory with unique name
        connString = fmt.Sprintf("file:%s?mode=memory&cache=shared", namespace)
    } else {
        // File-based
        connString = fmt.Sprintf("/tmp/eventodb-%s.db", namespace)
    }
    
    db, err := sql.Open("sqlite3", connString)
    if err != nil {
        return nil, err
    }
    
    // Run migrations on new namespace DB
    if err := s.migrateNamespaceDB(db); err != nil {
        return nil, err
    }
    
    s.namespaceDBs[namespace] = db
    return db, nil
}
```

**Benefits:**
- All data in memory (fast tests)
- Still maintains separation between namespaces
- No file I/O
- Automatic cleanup on process exit

---

## Token Generation

```go
// internal/auth/token.go
package auth

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/hex"
    "fmt"
)

func GenerateToken(namespace string) (string, error) {
    // Generate 32 random bytes
    randomBytes := make([]byte, 32)
    if _, err := rand.Read(randomBytes); err != nil {
        return "", err
    }
    
    // Format: ns_<base64(namespace)>_<hex(random)>
    nsEncoded := base64.RawURLEncoding.EncodeToString([]byte(namespace))
    randomHex := hex.EncodeToString(randomBytes)
    
    token := fmt.Sprintf("ns_%s_%s", nsEncoded, randomHex)
    return token, nil
}

func ParseToken(token string) (namespace string, err error) {
    // Extract namespace from token prefix
    parts := strings.Split(token, "_")
    if len(parts) != 3 || parts[0] != "ns" {
        return "", errors.New("invalid token format")
    }
    
    nsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
    if err != nil {
        return "", err
    }
    
    return string(nsBytes), nil
}

func HashToken(token string) string {
    h := sha256.Sum256([]byte(token))
    return hex.EncodeToString(h[:])
}
```

---

## Error Codes

```json
// Missing token
{
  "error": {
    "code": "AUTH_REQUIRED",
    "message": "Authorization header required"
  }
}

// Invalid token format
{
  "error": {
    "code": "AUTH_INVALID_TOKEN",
    "message": "Token format invalid"
  }
}

// Token doesn't match namespace
{
  "error": {
    "code": "AUTH_UNAUTHORIZED",
    "message": "Token not authorized for namespace 'tenant-a'"
  }
}

// Namespace doesn't exist
{
  "error": {
    "code": "NAMESPACE_NOT_FOUND",
    "message": "Namespace 'tenant-a' does not exist"
  }
}

// Namespace already exists
{
  "error": {
    "code": "NAMESPACE_EXISTS",
    "message": "Namespace 'tenant-a' already exists"
  }
}
```

---

## Configuration

### Environment Variables

```bash
# Default namespace token (generated on first run if not set)
MESSAGEDB_DEFAULT_TOKEN="ns_ZGVmYXVsdA_1234567890abcdef..."

# Test mode (in-memory SQLite, auto-create namespaces)
MESSAGEDB_TEST_MODE=true

# Require auth (if false, allows unauthenticated access to default namespace)
MESSAGEDB_REQUIRE_AUTH=true
```

### Config File

```yaml
# eventodb.yaml
auth:
  requireAuth: true
  defaultToken: "ns_ZGVmYXVsdA_1234567890abcdef..."

namespaces:
  allowAutoCreate: false  # In test mode, set to true
```

---

## Test Mode Behavior

```bash
eventodb serve --test-mode --port=8080
```

**Changes in test mode:**
1. **Backend:** SQLite in-memory (`:memory:`)
2. **Auto-create namespaces:** First write creates namespace
3. **Token exposure:** Returns token in response header for new namespaces
4. **Cleanup:** All data lost on shutdown

**Example test flow:**

```typescript
const server = await startTestServer();

// First request to new namespace auto-creates it
const response = await fetch(`${server.url}/rpc`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    // No auth header - test mode allows this
  },
  body: JSON.stringify([
    'stream.write',
    'account-1',
    { type: 'Opened', data: {} }
  ])
});

// Server returns token in response header
const token = response.headers.get('X-EventoDB-Token');
// "ns_dGVzdC0xMjM_a7f3c8d9..."

// Subsequent requests use token
await fetch(`${server.url}/rpc`, {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify(['stream.get', 'account-1', {}])
});
```

---

## Migration Structure

### Two-Level Migration System

#### Level 1: Metadata Schema (message_store)

```
migrations/
├── metadata/
│   ├── postgres/
│   │   └── 001_namespace_registry.sql
│   └── sqlite/
│       └── 001_namespace_registry.sql
```

**migrations/metadata/postgres/001_namespace_registry.sql:**
```sql
-- Create namespace registry schema
CREATE SCHEMA IF NOT EXISTS message_store;

-- Namespace registry
CREATE TABLE IF NOT EXISTS message_store.namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  schema_name TEXT NOT NULL UNIQUE,  -- For Postgres
  description TEXT,
  created_at BIGINT NOT NULL,
  metadata JSONB
);
```

**migrations/metadata/sqlite/001_namespace_registry.sql:**
```sql
-- Namespace registry (metadata database)
CREATE TABLE IF NOT EXISTS namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  db_path TEXT NOT NULL UNIQUE,  -- For SQLite
  description TEXT,
  created_at INTEGER NOT NULL,
  metadata TEXT
);
```

---

#### Level 2: Namespace Data Schema

```
migrations/
├── namespace/
│   ├── postgres/
│   │   └── 001_eventodb.sql
│   └── sqlite/
│       └── 001_eventodb.sql
```

**migrations/namespace/postgres/001_eventodb.sql:**
```sql
-- Template for namespace schema
-- {{SCHEMA_NAME}} will be replaced with actual schema name

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

-- Indexes
CREATE UNIQUE INDEX messages_id ON "{{SCHEMA_NAME}}".messages (id);
CREATE UNIQUE INDEX messages_stream ON "{{SCHEMA_NAME}}".messages (stream_name, position);
CREATE INDEX messages_category ON "{{SCHEMA_NAME}}".messages (
  (SPLIT_PART(stream_name, '-', 1)),
  global_position,
  (metadata->>'correlationStreamName')
);

-- Stored procedures
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".write_message(
  _id VARCHAR,
  _stream_name VARCHAR,
  _type VARCHAR,
  _data JSONB,
  _metadata JSONB DEFAULT NULL,
  _expected_version BIGINT DEFAULT NULL
) RETURNS BIGINT AS $$
  -- Implementation from EventoDB
$$ LANGUAGE plpgsql;

-- ... all other EventoDB functions
```

**migrations/namespace/sqlite/001_eventodb.sql:**
```sql
-- Template for namespace database
-- Applied to each namespace database file

CREATE TABLE IF NOT EXISTS messages (
  id TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  type TEXT NOT NULL,
  position INTEGER NOT NULL,
  global_position INTEGER PRIMARY KEY AUTOINCREMENT,
  data TEXT,
  metadata TEXT,
  time INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS messages_id ON messages (id);
CREATE UNIQUE INDEX IF NOT EXISTS messages_stream ON messages (stream_name, position);
CREATE INDEX IF NOT EXISTS messages_category ON messages (
  substr(stream_name, 1, instr(stream_name || '-', '-') - 1),
  global_position
);
```

---

## Server Startup Flow

```go
// cmd/eventodb/main.go
func main() {
    cfg := loadConfig()
    
    // 1. Connect to metadata database
    metadataDB := connectMetadataDB(cfg)
    
    // 2. Migrate metadata schema
    if err := migrateMetadataSchema(metadataDB, cfg.Backend); err != nil {
        log.Fatal(err)
    }
    
    // 3. Ensure default namespace exists
    defaultToken := ensureDefaultNamespace(metadataDB, cfg)
    log.Printf("[NAMESPACE] Default token: %s", defaultToken)
    
    // 4. Initialize store (handles namespace DBs)
    var store store.Store
    if cfg.Backend == "postgres" {
        store = postgres.New(metadataDB)
    } else {
        store = sqlite.New(metadataDB, cfg.TestMode)
    }
    
    // 5. Start server
    server := api.New(store)
    server.Listen(":8080")
}

func ensureDefaultNamespace(db *sql.DB, cfg Config) string {
    // Check if default namespace exists
    var exists bool
    db.QueryRow("SELECT EXISTS(SELECT 1 FROM namespaces WHERE id = 'default')").Scan(&exists)
    
    if !exists {
        // Generate token for default namespace
        token, _ := auth.GenerateToken("default")
        hash := auth.HashToken(token)
        
        if cfg.Backend == "postgres" {
            schemaName := "eventodb_default"
            
            // Create namespace record
            db.Exec(`
                INSERT INTO message_store.namespaces (id, token_hash, schema_name, description, created_at)
                VALUES ('default', $1, $2, 'Default namespace', $3)
            `, hash, schemaName, time.Now().Unix())
            
            // Create namespace schema with EventoDB structure
            createPostgresNamespaceSchema(db, schemaName)
            
        } else {
            dbPath := "/tmp/eventodb-default.db"
            if cfg.TestMode {
                dbPath = ":memory:"
            }
            
            // Create namespace record
            db.Exec(`
                INSERT INTO namespaces (id, token_hash, db_path, description, created_at)
                VALUES ('default', $1, $2, 'Default namespace', $3)
            `, hash, dbPath, time.Now().Unix())
        }
        
        return token
    }
    
    // Namespace exists, load token from env
    token := os.Getenv("MESSAGEDB_DEFAULT_TOKEN")
    if token == "" {
        log.Fatal("MESSAGEDB_DEFAULT_TOKEN required")
    }
    
    return token
}

func createPostgresNamespaceSchema(db *sql.DB, schemaName string) error {
    // Load namespace migration template
    template := loadMigrationTemplate("namespace/postgres/001_eventodb.sql")
    
    // Replace {{SCHEMA_NAME}} with actual schema name
    sql := strings.ReplaceAll(template, "{{SCHEMA_NAME}}", schemaName)
    
    // Execute
    _, err := db.Exec(sql)
    return err
}
```

---

## Security Considerations

1. **Token storage:** Only hash stored in DB, never plaintext
2. **Token transmission:** Always over HTTPS in production
3. **Token rotation:** Supported via `ns.rotateToken`
4. **Namespace isolation:** Enforced at query level (prefixing)
5. **Admin operations:** Only default namespace or flagged namespaces

---

## Benefits

1. **Multi-tenancy:** Full data isolation per namespace
2. **Simple auth:** Token-based, no user management complexity
3. **Testable:** Test mode allows ephemeral namespaces
4. **Secure:** Hash-based verification, namespace scoping
5. **Flexible:** Metadata allows custom attributes per namespace

---

## Future Enhancements

1. **Rate limiting:** Per-namespace quotas
2. **Token expiration:** Time-limited tokens
3. **Audit log:** Track namespace operations
4. **Billing integration:** Track usage per namespace
5. **Token scopes:** Read-only vs read-write tokens

---

## References

- [Multi-tenancy patterns](https://docs.microsoft.com/en-us/azure/architecture/guide/multitenant/approaches/overview)
- [Bearer token authentication](https://tools.ietf.org/html/rfc6750)
