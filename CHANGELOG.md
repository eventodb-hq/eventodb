# Changelog

All notable changes to this project will be documented in this file.

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
