# Changelog

All notable changes to this project will be documented in this file.

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
