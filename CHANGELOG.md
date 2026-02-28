# Changelog

All notable changes to this project will be documented in this file.

## [0.7.0] - 2026-03-01

### Features

- **Namespace stream and category listing** (ADR-011): New RPC methods for namespace introspection
  - `ns.streams` — list streams in the current namespace with optional prefix filtering and cursor-based pagination
  - `ns.categories` — list distinct categories with stream and message counts
  - Enables browser/introspection UIs (e.g. EventodbWeb) to discover what streams and categories exist
  - Both methods are namespace-scoped; reads in one namespace are invisible to another

### API

- `ns.streams` with optional `prefix`, `limit`, and `cursor` fields; results sorted lexicographically
- `ns.categories` with no options; returns category name, `streamCount`, and `messageCount`

### Store

- `ListStreams(ctx, namespace, opts)` added to `store.Store` interface
- `ListCategories(ctx, namespace)` added to `store.Store` interface
- New types: `ListStreamsOpts`, `StreamInfo`, `CategoryInfo`

### SDK

- `EventodbEx` (`eventodb_ex`): `namespace_streams/2`, `namespace_categories/1`
- `EventodbKit` (`eventodb_kit`): `namespace_streams/2`, `namespace_categories/1`
- TypeScript client: `listStreams(opts?)`, `listCategories()`, `StreamInfo`, `CategoryInfo`, `ListStreamsOptions`

---

## [0.6.0] - 2026-01-25

### Features

- **Export/Import CLI commands**: New `eventodb export` and `eventodb import` commands for backup and restore
  - Export to NDJSON format with optional gzip compression
  - Filter by categories and time range (`--since`, `--until`)
  - Stream all events without category filter
  - Import with `--force` flag to clear existing data (with y/N confirmation)
  - Streaming import with progress feedback
  - Preserves original global positions for sparse data transfer

- **Namespace schema versioning**: Automatic schema migrations for existing namespaces (ADR-010)
  - Per-namespace `_schema_version` table tracks applied migrations
  - Auto-migrate on server startup
  - Enables incremental schema updates without manual intervention

- **Empty category query**: `category.get` RPC now accepts empty string to return all messages ordered by global position

### API

- `POST /import` endpoint for streaming NDJSON import
  - `?force=true` query parameter clears existing data before import
  - SSE progress events during import
- `category.get` with empty category name returns all messages

### Store

- `ImportBatch` method for bulk import with explicit positions
- `ClearNamespaceMessages` method for clearing namespace data
- `MigrateNamespaces` method for applying pending schema migrations

### Documentation

- ADR-009: Sparse Export/Import design
- ADR-010: Namespace Schema Versioning design

## [0.5.2] - 2026-01-24

### Features

- **Namespace-wide SSE subscription**: New `?all=true` parameter for `/subscribe` endpoint to receive poke notifications for all events in a namespace

### Fixes

- **Graceful shutdown**: Fixed server hanging on shutdown when SSE connections were active. PubSub now closes all subscriber channels on SIGTERM/SIGINT, allowing immediate cleanup

## [0.5.1] - 2025-01-24

### Features

- **Global SSE subscription**: New `?all=true` parameter for `/subscribe` endpoint to receive pokes for all events in a namespace via single connection
- **Simplified dev token**: Easier token handling in development mode
- **Config module**: Added `log_sql` option for SQL query logging

### Fixes

- Fixed consumer events fetching with correct position (EventodbKit)
- Fixed health check to use 127.0.0.1 instead of localhost

### SDKs

- Elixir SDKs (`eventodb_ex`, `eventodb_kit`) prepared for hex.pm release
- Plain-text namespace encoding in tokens (ADR-006)

### Operations

- Added Docker deployment support with environment variable configuration
- Added server management script (`bin/server.sh`)

## [0.5.0] - 2025-12-25

Initial public release of EventoDB.

### Features

- **Multi-backend support**: SQLite, PostgreSQL, TimescaleDB, Pebble KV
- **HTTP/RPC API**: Simple JSON-RPC over HTTP POST
- **SSE subscriptions**: Real-time pub/sub via Server-Sent Events
- **Multi-tenancy**: Namespace isolation with token-based authentication
- **Consumer groups**: Coordinated message consumption across multiple consumers
- **Optimistic locking**: Safe concurrent writes with expected version checks
- **UUID7 message IDs**: Time-ordered unique identifiers

### SDKs

- Go SDK (`clients/eventodb-go`)
- Node.js SDK (`clients/eventodb-js`)
- Elixir client examples

### Operations

- Test mode with in-memory SQLite/Pebble for development
- Comprehensive test suite: unit, integration, black-box tests
- Performance profiling and benchmarking tools
