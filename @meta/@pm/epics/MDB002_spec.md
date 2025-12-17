# EPIC MDB002: RPC API & Authentication

## Overview

**Epic ID:** MDB002
**Name:** RPC API & Authentication
**Duration:** 2-3 weeks
**Status:** pending
**Priority:** high
**Depends On:** MDB001 (Core Storage & Migrations)

**Goal:** Build HTTP server with RPC endpoints and token-based namespace authentication for multi-tenant event sourcing access.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ HTTP Server (cmd/messagedb)                                  │
│ - POST /rpc (RPC handler)                                    │
│ - GET /subscribe (SSE subscriptions)                         │
│ - Authentication middleware                                  │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ API Layer (internal/api)                                     │
│ - RPC method routing                                         │
│ - SSE connection management                                  │
│ - Token validation                                           │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Auth Layer (internal/auth)                                   │
│ - Token generation (ns_<base64>_<random>)                   │
│ - Token validation                                           │
│ - Namespace extraction                                       │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Store Interface (internal/store)                             │
│ - Postgres backend (stored procedures)                       │
│ - SQLite backend (Go-based logic)                           │
└─────────────────────────────────────────────────────────────┘
```

## Technical Requirements

### API Format (ADR-001)

**RPC Endpoint:** `POST /rpc`

**Request Format:**
```json
["method", arg1, arg2, arg3]
```

**Response Format:**
```json
{result}  // or {"error": {...}}
```

**Method Naming:** `noun.verb` (e.g., `stream.write`, `category.get`)

### Authentication (ADR-004)

**Token Format:**
```
ns_<base64(namespace_id)>_<random_32_bytes_hex>
Example: ns_dGVuYW50LWE_a7f3c8d9e2b1f4a6c8e9d2b3f5a7c9e1
```

**Token Storage:**
```sql
-- Postgres: message_store.namespaces
CREATE TABLE message_store.namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,  -- SHA-256 of token
  schema_name TEXT NOT NULL UNIQUE,
  description TEXT,
  created_at BIGINT NOT NULL,
  metadata JSONB
);

-- SQLite: metadata.db
CREATE TABLE namespaces (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  db_path TEXT NOT NULL UNIQUE,
  description TEXT,
  created_at INTEGER NOT NULL,
  metadata TEXT
);
```

**Authentication Method:** Header-based
```
Authorization: Bearer ns_abc123...
```

## Functional Requirements

### FR-1: Stream Operations

#### stream.write
```json
["stream.write", "streamName", {msg}, {opts}]

// msg object:
{
  "type": "Withdrawn",
  "data": {"amount": 50},
  "metadata": {"userId": "u1"}  // optional
}

// opts object (optional):
{
  "id": "a5eb2a97-...",      // optional, server generates if not provided
  "expectedVersion": 5        // optional, for optimistic locking
}

// Response:
{"position": 6, "globalPosition": 1234}
```

#### stream.get
```json
["stream.get", "streamName", {opts}]

// opts object (optional):
{
  "position": 0,              // stream position
  "globalPosition": 1235,     // OR global position (mutually exclusive)
  "batchSize": 100,
  "condition": null           // SQL condition if needed
}

// Response:
[
  ["id1", "Opened", 0, 1000, {...data}, {...meta}, "2024-12-17T01:00:00Z"],
  ["id2", "Deposited", 1, 1001, {...data}, {...meta}, "2024-12-17T01:01:00Z"]
]
// Format: [messageId, type, position, globalPosition, data, metadata, time]
```

#### stream.last
```json
["stream.last", "streamName", {opts}]

// opts (optional):
{
  "type": "Withdrawn"  // filter by message type
}

// Response:
["id", "Withdrawn", 5, 1234, {...data}, {...meta}, "2024-12-17T01:00:00Z"]
// Returns null if not found
```

#### stream.version
```json
["stream.version", "streamName"]

// Response:
5  // or null if stream doesn't exist
```

### FR-2: Category Operations

#### category.get
```json
["category.get", "categoryName", {opts}]

// opts object:
{
  "position": 0,                            // global position for category
  "globalPosition": 1235,                   // alternative
  "batchSize": 100,
  "correlation": null,
  "consumerGroup": {"member": 0, "size": 2}, // optional
  "condition": null
}

// Response:
[
  ["id1", "account-123", "Opened", 0, 1000, {...}, {...}, "2024-12-17T01:00:00Z"],
  ["id2", "account-456", "Deposited", 1, 1001, {...}, {...}, "2024-12-17T01:01:00Z"]
]
// Format: [messageId, streamName, type, position, globalPosition, data, metadata, time]
// Note: Stream name included since messages come from multiple streams
```

### FR-3: Namespace Management

#### ns.create
```json
["ns.create", "tenant-a", {opts}]

// opts (optional):
{
  "description": "Production tenant A",
  "metadata": {"plan": "enterprise"}
}

// Response:
{
  "namespace": "tenant-a",
  "token": "ns_dGVuYW50LWE_a7f3c8d9...",
  "createdAt": "2024-12-17T01:00:00Z"
}
```

**Server Operations (Postgres):**
1. Generate token and hash
2. Create schema: `CREATE SCHEMA "messagedb_tenant_a"`
3. Run namespace migrations (tables, indexes, functions)
4. Insert record into `message_store.namespaces`

**Server Operations (SQLite):**
1. Generate token and hash
2. Create database file or in-memory DB
3. Run namespace migrations
4. Insert record into metadata database

#### ns.delete
```json
["ns.delete", "tenant-a"]

// Response:
{
  "namespace": "tenant-a",
  "deletedAt": "2024-12-17T02:00:00Z",
  "messagesDeleted": 1543
}
```

**Server Operations (Postgres):**
1. Verify token matches namespace
2. `DROP SCHEMA "messagedb_tenant_a" CASCADE`
3. Delete from `message_store.namespaces`

**Server Operations (SQLite):**
1. Verify token matches namespace
2. Close database connection
3. Delete database file
4. Delete from metadata namespaces table

#### ns.list
```json
["ns.list", {opts}]

// opts (optional):
{
  "limit": 100,
  "offset": 0
}

// Response:
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

**Authentication:** Admin token required

#### ns.info
```json
["ns.info", "tenant-a"]

// Response:
{
  "namespace": "tenant-a",
  "description": "Tenant A",
  "createdAt": "2024-12-17T01:00:00Z",
  "messageCount": 567,
  "streamCount": 12,
  "lastActivity": "2024-12-17T01:30:00Z"
}
```

**Authentication:** Token for that namespace required

### FR-4: System Operations

#### sys.version
```json
["sys.version"]

// Response:
"1.3.0"
```

#### sys.health
```json
["sys.health"]

// Response:
{
  "status": "ok",
  "backend": "postgres",
  "connections": 5
}
```

### FR-5: SSE Subscriptions (ADR-001)

**Philosophy:** Stream only lightweight "pokes" (notifications), not full message data. Clients fetch actual data separately.

#### Subscribe to Stream
```
GET /subscribe?stream=account-123&position=5
Authorization: Bearer ns_...
```

**SSE Event Format:**
```
event: poke
data: {"stream": "account-123", "position": 6, "globalPosition": 1235}

event: poke
data: {"stream": "account-123", "position": 7, "globalPosition": 1236}
```

**Client Flow:**
1. Receive poke with position info
2. Fetch actual data: `["stream.get", "account-123", {"position": 6, "batchSize": 10}]`

#### Subscribe to Category
```
GET /subscribe?category=account&position=1000&consumer=0&size=2
Authorization: Bearer ns_...
```

**SSE Event Format:**
```
event: poke
data: {"stream": "account-123", "position": 6, "globalPosition": 1235}

event: poke
data: {"stream": "account-456", "position": 2, "globalPosition": 1236}
```

### FR-6: Test Mode Support

When started with `--test-mode`:

**Behavior:**
1. Backend: SQLite in-memory (`:memory:`)
2. Auto-creates namespaces on first use
3. Returns tokens in response headers
4. All data lost on shutdown

**Example Response Header:**
```
X-MessageDB-Token: ns_dGVzdC0xMjM_a7f3c8d9...
```

### FR-7: Error Response Format

```json
{
  "error": {
    "code": "STREAM_VERSION_CONFLICT",
    "message": "Expected version 5, stream is at version 6",
    "details": {"expected": 5, "actual": 6}
  }
}
```

**Error Codes:**
- `STREAM_VERSION_CONFLICT` - Optimistic concurrency violation
- `INVALID_REQUEST` - Malformed request
- `STREAM_NOT_FOUND` - Stream doesn't exist
- `NAMESPACE_NOT_FOUND` - Invalid namespace
- `NAMESPACE_EXISTS` - Namespace already exists
- `BACKEND_ERROR` - Database/storage error
- `AUTH_REQUIRED` - Missing authorization header
- `AUTH_INVALID_TOKEN` - Invalid token format
- `AUTH_UNAUTHORIZED` - Token not authorized for namespace

## Implementation Strategy

### Phase 1: RPC Handler Foundation (2-3 days)
- HTTP server setup
- RPC request parsing (method routing)
- Response formatting
- Basic error handling
- Health check endpoint

### Phase 2: Authentication Middleware (2-3 days)
- Token generation and hashing
- Token validation
- Namespace extraction from token
- Auth middleware integration
- Default namespace setup

### Phase 3: Stream Operations (3-4 days)
- stream.write implementation
- stream.get implementation
- stream.last implementation
- stream.version implementation
- Optimistic locking support

### Phase 4: Category Operations (2-3 days)
- category.get implementation
- Consumer group support
- Correlation filtering
- Category stream extraction

### Phase 5: Namespace Management (2-3 days)
- ns.create implementation
- ns.delete implementation
- ns.list implementation
- ns.info implementation
- Schema creation/deletion

### Phase 6: SSE Subscriptions (3-4 days)
- SSE handler setup
- Stream subscription logic
- Category subscription logic
- Connection management
- Poke notification system

### Phase 7: Test Mode & Integration (2-3 days)
- Test mode implementation
- Auto-namespace creation
- Token exposure in headers
- Integration testing
- Documentation

## Acceptance Criteria

### AC-1: Stream Write with Optimistic Locking
- **GIVEN** Stream with version 5
- **WHEN** Writing with expectedVersion=5
- **THEN** Write succeeds, returns position 6
- **AND** Writing again with expectedVersion=5 fails with STREAM_VERSION_CONFLICT

### AC-2: Category Read with Consumer Groups
- **GIVEN** Category "account" with 100 messages across 10 streams
- **WHEN** Reading with consumerGroup {member: 0, size: 2}
- **THEN** Returns only messages from streams assigned to member 0
- **AND** No overlap with member 1

### AC-3: Namespace Isolation
- **GIVEN** Two namespaces: "tenant-a" and "tenant-b"
- **WHEN** Writing to "account-123" in both namespaces
- **THEN** Each namespace has separate data
- **AND** Token for tenant-a cannot access tenant-b data

### AC-4: SSE Stream Subscription
- **GIVEN** Subscription to stream "account-123" at position 5
- **WHEN** New message written at position 6
- **THEN** SSE poke received with position 6 and globalPosition
- **AND** Client fetches data via stream.get

### AC-5: Test Mode Auto-Creation
- **GIVEN** Server started with --test-mode
- **WHEN** First write to new namespace
- **THEN** Namespace auto-created
- **AND** Token returned in X-MessageDB-Token header

### AC-6: Namespace Deletion Safety
- **GIVEN** Namespace with existing messages
- **WHEN** Calling ns.delete
- **THEN** Namespace and all data deleted
- **AND** Token immediately invalid
- **AND** Subsequent requests fail with NAMESPACE_NOT_FOUND

### AC-7: Default Namespace
- **GIVEN** Server startup
- **WHEN** Server starts
- **THEN** Default namespace created
- **AND** Token printed to stdout
- **AND** Can write/read using default token

## Definition of Done

- [ ] HTTP server with RPC endpoint working
- [ ] All stream operations implemented (write, get, last, version)
- [ ] All category operations implemented (get with consumer groups)
- [ ] All namespace operations implemented (create, delete, list, info)
- [ ] All system operations implemented (version, health)
- [ ] Token generation and validation working
- [ ] Authentication middleware enforcing namespace isolation
- [ ] SSE subscriptions working (stream and category)
- [ ] Test mode with auto-namespace creation
- [ ] Default namespace setup on startup
- [ ] Error handling for all edge cases
- [ ] Integration tests for all API methods
- [ ] Performance targets met (API response < 50ms p95)
- [ ] Documentation complete (API examples)
- [ ] Code passes linting and formatting

## Error Codes Reference

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `STREAM_VERSION_CONFLICT` | 409 | Optimistic locking failed |
| `INVALID_REQUEST` | 400 | Malformed JSON or arguments |
| `STREAM_NOT_FOUND` | 404 | Stream doesn't exist |
| `NAMESPACE_NOT_FOUND` | 404 | Namespace doesn't exist |
| `NAMESPACE_EXISTS` | 409 | Namespace already exists |
| `BACKEND_ERROR` | 500 | Database/storage error |
| `AUTH_REQUIRED` | 401 | Missing Authorization header |
| `AUTH_INVALID_TOKEN` | 401 | Invalid token format |
| `AUTH_UNAUTHORIZED` | 403 | Token not authorized |

## Performance Expectations

| Operation | Expected Performance |
|-----------|---------------------|
| RPC method routing | <1ms |
| Token validation | <1ms |
| stream.write | <10ms (p95) |
| stream.get (100 msgs) | <20ms (p95) |
| category.get (100 msgs) | <30ms (p95) |
| SSE poke delivery | <5ms |
| Namespace creation | <100ms |
| Namespace deletion | <200ms |

## Validation Rules

1. **Stream names:** Non-empty strings, no validation on format
2. **Message types:** Non-empty strings
3. **Message data:** Valid JSON objects
4. **Expected version:** Non-negative integer or null
5. **Batch size:** 1-1000 messages
6. **Consumer group:** member < size, both non-negative

## Non-Goals

- ❌ WebSocket support (SSE sufficient per ADR-001)
- ❌ GraphQL interface
- ❌ Rate limiting (future version)
- ❌ Built-in metrics (use external tools)
- ❌ Message replay features
- ❌ Backup/restore API
- ❌ User management (only namespace tokens)
- ❌ Token expiration (use rotation instead)

## Dependencies

- **MDB001:** Core storage and migrations must be complete
- **Go 1.21+:** For HTTP server and concurrency
- **Chi router:** For HTTP routing (or standard library)
- **SSE library:** For Server-Sent Events

## References

- ADR-001: RPC-Style API Format
- ADR-004: Namespaces and Authentication
- Message DB Documentation: http://docs.eventide-project.org/
