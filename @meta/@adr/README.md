# Architecture Decision Records (ADRs)

This directory contains the architectural decisions for the MessageDB Go server.

---

## Overview

**MessageDB Go** is a REST/RPC API server that wraps PostgreSQL-based Message DB (event sourcing database) and provides:

- Simple RPC-style HTTP API
- Multi-tenant namespaces with token authentication
- Dual backend support (Postgres + SQLite for tests)
- Automatic schema migrations
- Server-Sent Events for real-time subscriptions

---

## ADR Index

### [ADR-001: API Format](./ADR001-api-format.md)
**Status:** Accepted  
**Summary:** RPC-style API using compact JSON format

**Key Decisions:**
- Single endpoint: `POST /rpc`
- Method naming: `noun.verb` (e.g., `stream.write`, `category.get`)
- Max 4 arguments per method
- Compact JSON arrays (not objects) for efficiency
- SSE streaming for subscriptions (pokes only, not full messages)
- Namespace prefix in stream names: `tenant-a:account-123`

**Example:**
```json
["stream.write", "account-123", {"type": "Withdrawn", "data": {...}}, {"expectedVersion": 5}]
```

---

### [ADR-002: Schema Migrations](./ADR002-schema-migrations.md)
**Status:** Accepted  
**Summary:** Automatic migrations on boot, dual-backend support

**Key Decisions:**
- Auto-migrate on server startup (transparent to users)
- Separate migrations for Postgres and SQLite
- Two-level migration system:
  - **Metadata migrations:** Namespace registry
  - **Namespace migrations:** Message DB structure per namespace
- SQLite: No stored procedures (logic implemented in Go)
- Idempotent migrations with `IF NOT EXISTS`

**Migration Structure:**
```
migrations/
├── metadata/
│   ├── postgres/001_namespace_registry.sql
│   └── sqlite/001_namespace_registry.sql
└── namespace/
    ├── postgres/001_message_db.sql
    └── sqlite/001_message_db.sql
```

---

### [ADR-003: External Test Suite](./ADR003-external-test-suite.md)
**Status:** Accepted  
**Summary:** Black-box testing with Bun.js, namespace isolation

**Key Decisions:**
- External test suite using Bun.js (not Go tests)
- Tests run against real HTTP API (black-box)
- Test mode uses in-memory SQLite
- Each test creates/deletes its own namespace
- Parallel test execution (no conflicts)
- Tests validate API contract, not implementation

**Example Test:**
```typescript
test('write and read message', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  await client.writeMessage('account-123', {...});
  const messages = await client.getStream('account-123');
  
  expect(messages).toHaveLength(1);
  
  await client.deleteNamespace();
  server.close();
});
```

---

### [ADR-004: Namespaces and Authentication](./ADR004-namespaces-and-auth.md)
**Status:** Accepted  
**Summary:** Physical data isolation per namespace, token-based auth

**Key Decisions:**
- **Postgres:** Each namespace = separate schema
  - `message_store` schema: namespace registry (tokens only)
  - `messagedb_default` schema: default namespace data
  - `messagedb_tenant_a` schema: tenant-a data
- **SQLite:** Each namespace = separate database file/in-memory DB
  - Metadata DB: namespace registry
  - Per-namespace DBs: message data
- **Token format:** `ns_<base64(namespace)>_<random_hex>`
- **Authentication:** Bearer token in `Authorization` header
- **Test mode:** Auto-creates namespaces, returns token in response header
- **Deletion:** `DROP SCHEMA CASCADE` (Postgres) or delete DB file (SQLite)

**Architecture:**
```
Postgres:
  message_store schema (metadata)
    ├── namespaces table
  messagedb_default schema (data)
    ├── messages table
    ├── write_message() function
    └── get_stream_messages() function
  messagedb_tenant_a schema (data)
    ├── messages table
    ├── write_message() function
    └── get_stream_messages() function

SQLite (test mode):
  metadata.db (:memory:)
    ├── namespaces table
  default.db (:memory:)
    ├── messages table
  tenant-a.db (:memory:)
    ├── messages table
```

---

## Design Principles

### 1. Simplicity
- Single binary, no bash scripts
- Auto-migrate on boot
- No manual setup

### 2. Multi-Backend
- Postgres for production
- SQLite for tests/dev
- Same API, different storage

### 3. Multi-Tenancy
- Physical isolation (schemas/databases)
- Token-based auth
- Clean namespace deletion

### 4. Developer Experience
- External tests (language-agnostic validation)
- Test mode for fast iteration
- Clear error messages

### 5. Performance
- Lightweight SSE (pokes only)
- No prefix filtering (separate schemas)
- Connection pooling per namespace

---

## Implementation Status

- [ ] ADR-001: API Format
- [ ] ADR-002: Schema Migrations
- [ ] ADR-003: External Test Suite
- [ ] ADR-004: Namespaces and Authentication

---

## Future ADRs

Potential topics for future decisions:

- **ADR-005:** Observability (metrics, logging, tracing)
- **ADR-006:** Rate limiting and quotas
- **ADR-007:** Backup and restore strategy
- **ADR-008:** High availability and clustering
- **ADR-009:** Client libraries (auto-generation)
- **ADR-010:** Performance benchmarking suite

---

## References

- [Message DB Documentation](http://docs.eventide-project.org/user-guide/message-db/)
- [HN Discussion: Postgres-based messaging](https://news.ycombinator.com/item?id=21810272)
- [Compact JSON Spec](https://jsonjoy.com/specs/compact-json/)
- [ADR Process](https://github.com/joelparkerhenderson/architecture-decision-record)
