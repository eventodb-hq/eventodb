# EventoDB Go - Design Summary

**Status:** Specification Complete - Ready for Implementation  
**Date:** 2024-12-17

---

## Project Goal

Create a **Go-based HTTP API server** that wraps EventoDB (PostgreSQL event sourcing database) to provide:

1. Simple RPC-style HTTP API (no direct Postgres access needed)
2. Multi-tenant namespaces with physical data isolation
3. SQLite backend for testing (in-memory)
4. Server-Sent Events for real-time subscriptions
5. Automatic schema migrations

---

## Architecture at a Glance

```
┌─────────────────────────────────────────────────────────────────┐
│                        Client Applications                       │
│                  (Any HTTP client - curl, JS, Go, etc.)         │
└────────────────────────────────┬────────────────────────────────┘
                                 │ HTTP/JSON RPC
                                 │ Authorization: Bearer token
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                    EventoDB Go Server                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ RPC Handler  │  │     Auth     │  │     SSE      │          │
│  │ POST /rpc    │  │   Middleware │  │   Streaming  │          │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘          │
│         │                 │                                      │
│         ▼                 ▼                                      │
│  ┌─────────────────────────────────────────────────┐           │
│  │            Store Interface                       │           │
│  │  - WriteMessage()                                │           │
│  │  - GetStreamMessages()                           │           │
│  │  - GetCategoryMessages()                         │           │
│  │  - Subscribe()                                   │           │
│  └─────────────────┬───────────────────────────────┘           │
│                    │                                             │
│         ┌──────────┴──────────┐                                │
│         ▼                     ▼                                 │
│  ┌──────────────┐      ┌──────────────┐                       │
│  │   Postgres   │      │    SQLite    │                       │
│  │   Backend    │      │   Backend    │                       │
│  │              │      │  (test mode) │                       │
│  └──────┬───────┘      └──────┬───────┘                       │
└─────────┼──────────────────────┼──────────────────────────────┘
          │                      │
          ▼                      ▼
┌─────────────────┐    ┌─────────────────┐
│   PostgreSQL    │    │ SQLite Files/   │
│                 │    │   In-Memory     │
│ message_store   │    │ ┌─────────────┐ │
│ ├─ namespaces   │    │ │ metadata.db │ │
│                 │    │ │ default.db  │ │
│ eventodb_*     │    │ │ tenant-a.db │ │
│ ├─ messages     │    │ └─────────────┘ │
│ ├─ functions    │    │                 │
└─────────────────┘    └─────────────────┘
```

---

## Core Concepts

### 1. RPC API

**Single endpoint:** `POST /rpc`

**Request format:**
```json
["method", arg1, arg2, arg3]
```

**Methods:**
- `stream.write` - Write event to stream
- `stream.get` - Read events from stream
- `stream.last` - Get last event
- `stream.version` - Get stream version
- `category.get` - Read events from category (multiple streams)
- `ns.create` - Create namespace
- `ns.delete` - Delete namespace
- `ns.list` - List namespaces (admin)
- `sys.version` - Server version
- `sys.health` - Health check

**Subscriptions:** `GET /subscribe?stream=X` (Server-Sent Events)

### 2. Namespaces

**Physical isolation** - not just logical partitioning:

**Postgres:**
- `message_store` schema: Namespace registry (tokens)
- `eventodb_default` schema: Default namespace data
- `eventodb_tenant_a` schema: Tenant A data
- Each schema has full EventoDB structure (tables + functions)

**SQLite:**
- `metadata.db`: Namespace registry
- `default.db`: Default namespace data
- `tenant-a.db`: Tenant A data
- Each DB file has full EventoDB structure

**Benefits:**
- Complete data separation
- Fast deletion (`DROP SCHEMA CASCADE` or delete file)
- No prefix filtering needed
- Better performance

### 3. Authentication

**Token format:**
```
ns_<base64(namespace_id)>_<random_32_bytes_hex>
```

**Example:**
```
ns_dGVuYW50LWE_a7f3c8d9e2b1f4a6c8e9d2b3f5a7c9e1
```

**Usage:**
```bash
curl -H "Authorization: Bearer ns_dGVu..." \
  http://localhost:8080/rpc \
  -d '["stream.write", "account-123", {...}]'
```

**Token storage:**
- Only SHA-256 hash stored in database
- Token returned once on namespace creation
- Must be saved by client

### 4. Test Mode

**Flag:** `--test-mode`

**Behavior:**
- Uses in-memory SQLite for all namespaces
- Auto-creates namespaces on first request
- Returns token in `X-EventoDB-Token` response header
- All data lost on shutdown
- Perfect for fast, isolated tests

**Example:**
```bash
eventodb serve --test-mode

# Tests can create namespaces automatically:
curl http://localhost:8080/rpc \
  -d '["stream.write", "account-1", {...}]'
# Server auto-creates namespace, returns token in header
```

### 5. Migrations

**Two-level system:**

**Level 1: Metadata migrations**
- Run once on server startup
- Creates namespace registry

**Level 2: Namespace migrations**
- Run when creating each namespace
- Creates EventoDB structure (tables, indexes, functions)

**Automatic:**
- No manual migration commands
- Server migrates on boot
- Template-based (schema name replaced at runtime)

---

## Data Flow Examples

### Writing an Event

```
Client                    Server                    Postgres
  │                         │                          │
  │  POST /rpc             │                          │
  │  ["stream.write",      │                          │
  │   "account-123",       │                          │
  │   {type: "Deposited",  │                          │
  │    data: {amt: 100}}]  │                          │
  │────────────────────────>│                          │
  │                         │                          │
  │                         │  1. Validate token      │
  │                         │  2. Get namespace       │
  │                         │     from token          │
  │                         │                          │
  │                         │  SELECT * FROM          │
  │                         │  message_store.namespaces│
  │                         │  WHERE token_hash=?     │
  │                         │─────────────────────────>│
  │                         │<─────────────────────────│
  │                         │  (schema: eventodb_xyz) │
  │                         │                          │
  │                         │  3. Call write function │
  │                         │  SELECT eventodb_xyz   │
  │                         │   .write_message(...)   │
  │                         │─────────────────────────>│
  │                         │<─────────────────────────│
  │                         │  (position: 42)          │
  │<────────────────────────│                          │
  │  {position: 42,        │                          │
  │   globalPosition: 1234}│                          │
```

### Creating a Namespace

```
Client                    Server                    Postgres
  │                         │                          │
  │  POST /rpc             │                          │
  │  ["ns.create",         │                          │
  │   "tenant-a", {}]      │                          │
  │────────────────────────>│                          │
  │                         │                          │
  │                         │  1. Verify admin token  │
  │                         │  2. Generate new token  │
  │                         │  3. Hash token          │
  │                         │                          │
  │                         │  CREATE SCHEMA          │
  │                         │   "eventodb_tenant_a"  │
  │                         │─────────────────────────>│
  │                         │                          │
  │                         │  4. Load namespace      │
  │                         │     migration template  │
  │                         │  5. Replace {{SCHEMA}}  │
  │                         │     with actual name    │
  │                         │  6. Execute migration   │
  │                         │                          │
  │                         │  CREATE TABLE           │
  │                         │   eventodb_tenant_a    │
  │                         │   .messages (...)       │
  │                         │─────────────────────────>│
  │                         │                          │
  │                         │  CREATE FUNCTION        │
  │                         │   eventodb_tenant_a    │
  │                         │   .write_message(...)   │
  │                         │─────────────────────────>│
  │                         │                          │
  │                         │  7. Insert namespace    │
  │                         │  INSERT INTO            │
  │                         │   message_store         │
  │                         │   .namespaces (...)     │
  │                         │─────────────────────────>│
  │<────────────────────────│                          │
  │  {namespace: "tenant-a",                          │
  │   token: "ns_...",     │                          │
  │   createdAt: "..."}    │                          │
```

### Real-time Subscription

```
Client                    Server                    Postgres
  │                         │                          │
  │  GET /subscribe?       │                          │
  │    stream=account-1    │                          │
  │    &token=ns_...       │                          │
  │────────────────────────>│                          │
  │                         │                          │
  │                         │  1. Validate token      │
  │                         │  2. Start SSE stream    │
  │<────────────────────────│                          │
  │  (Connection open)     │                          │
  │                         │                          │
  │                         │  3. Listen for NOTIFY   │
  │                         │     or poll for new     │
  │                         │     messages            │
  │                         │                          │
  │                    [New message written]          │
  │                         │                          │
  │                         │  SELECT max(position)   │
  │                         │  FROM eventodb_xyz     │
  │                         │   .messages             │
  │                         │  WHERE stream_name=?    │
  │                         │─────────────────────────>│
  │                         │<─────────────────────────│
  │                         │  (position: 43)          │
  │<────────────────────────│                          │
  │  event: poke           │                          │
  │  data: {stream:        │                          │
  │   "account-1",         │                          │
  │   position: 43,        │                          │
  │   globalPosition: 1235}│                          │
  │                         │                          │
  │  POST /rpc             │                          │
  │  ["stream.get",        │                          │
  │   "account-1",         │                          │
  │   {position: 43}]      │                          │
  │────────────────────────>│                          │
  │                         │  SELECT * FROM          │
  │                         │   eventodb_xyz.        │
  │                         │   get_stream_messages() │
  │                         │─────────────────────────>│
  │<────────────────────────│<─────────────────────────│
  │  [message data]        │                          │
```

---

## File Structure

```
eventodb-go/
├── cmd/
│   └── eventodb/
│       └── main.go              # CLI entry point
│
├── internal/
│   ├── api/
│   │   ├── handler.go           # RPC handler
│   │   ├── sse.go               # Server-Sent Events
│   │   └── middleware.go        # Auth middleware
│   │
│   ├── auth/
│   │   ├── token.go             # Token generation/validation
│   │   └── namespace.go         # Namespace auth logic
│   │
│   ├── migrate/
│   │   ├── migrate.go           # Migration engine
│   │   └── template.go          # Template replacement
│   │
│   └── store/
│       ├── store.go             # Store interface
│       ├── postgres/
│       │   ├── store.go         # Postgres implementation
│       │   ├── write.go
│       │   ├── read.go
│       │   └── namespace.go     # Schema management
│       └── sqlite/
│           ├── store.go         # SQLite implementation
│           ├── write.go         # Write logic (no stored procs)
│           ├── read.go
│           └── namespace.go     # DB file management
│
├── migrations/
│   ├── embed.go                 # Go embed declarations
│   ├── metadata/
│   │   ├── postgres/
│   │   │   └── 001_namespace_registry.sql
│   │   └── sqlite/
│   │       └── 001_namespace_registry.sql
│   └── namespace/
│       ├── postgres/
│       │   └── 001_eventodb.sql      # Template with {{SCHEMA_NAME}}
│       └── sqlite/
│           └── 001_eventodb.sql
│
├── pkg/
│   └── client/                  # Optional Go client library
│       └── client.go
│
├── test/                        # External Bun.js test suite
│   ├── package.json
│   ├── tests/
│   │   ├── stream.test.ts
│   │   ├── category.test.ts
│   │   ├── namespace.test.ts
│   │   └── subscription.test.ts
│   └── lib/
│       ├── client.ts            # TypeScript client
│       └── helpers.ts
│
└── @meta/
    └── @adr/
        ├── README.md
        ├── ADR001-api-format.md
        ├── ADR002-schema-migrations.md
        ├── ADR003-external-test-suite.md
        └── ADR004-namespaces-and-auth.md
```

---

## Key Implementation Notes

### Postgres Store

1. **Namespace management:**
   - Create: Execute template migration with schema name substitution
   - Delete: `DROP SCHEMA "eventodb_xyz" CASCADE`
   - List: Query `message_store.namespaces`

2. **Write/Read:**
   - Call stored procedures in namespace schema
   - Example: `SELECT "eventodb_xyz".write_message(...)`

3. **Connection pooling:**
   - Single connection pool to database
   - Switch schemas per request (via token → schema mapping)

### SQLite Store

1. **Namespace management:**
   - Metadata DB: Always open (namespace registry)
   - Per-namespace DBs: Lazy-loaded on first access
   - In-memory mode: Named connections (`file:xyz?mode=memory&cache=shared`)

2. **Write/Read:**
   - Implement EventoDB logic in Go (no stored procedures)
   - Same semantics as Postgres functions
   - Optimistic locking via `expected_version`

3. **Connection management:**
   - Map of namespace → *sql.DB
   - Close connections on namespace deletion

### Authentication

1. **Token generation:**
   - Crypto-random 32 bytes
   - Base64-encode namespace for parsing
   - Store only SHA-256 hash

2. **Validation:**
   - Extract token from `Authorization: Bearer` header
   - Hash token, lookup in namespaces table
   - Cache valid tokens (with expiration)

3. **SSE authentication:**
   - Token in query param (EventSource doesn't support headers)
   - `GET /subscribe?stream=X&token=ns_...`

---

## Testing Strategy

### Unit Tests (Go)

- Store interface implementations
- Token generation/validation
- Migration engine
- RPC handler logic

### Integration Tests (Bun.js)

- Full API contract validation
- Real HTTP requests
- Namespace isolation
- Concurrent operations
- SSE subscriptions

### Performance Tests

- Write throughput
- Read latency
- Concurrent namespace operations
- Subscription overhead

---

## Next Steps

1. **Scaffold project structure**
2. **Implement store interface**
3. **Build Postgres backend**
4. **Build SQLite backend**
5. **Implement RPC handler**
6. **Add authentication**
7. **Implement SSE subscriptions**
8. **Write external tests**
9. **Documentation & examples**

---

## Success Criteria

- [ ] Single binary, no dependencies
- [ ] Auto-migrate on boot
- [ ] All EventoDB functions accessible via RPC
- [ ] Namespace creation/deletion works
- [ ] Token authentication works
- [ ] SQLite test mode works
- [ ] External tests pass
- [ ] Can import existing EventoDB test cases
- [ ] Performance comparable to direct Postgres access (±20%)

---

## References

- [EventoDB Repo](https://github.com/eventodb/eventodb)
- [EventoDB Docs](http://docs.eventide-project.org/user-guide/eventodb/)
- ADRs in `@meta/@adr/`
