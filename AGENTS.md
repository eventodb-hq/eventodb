# EventoDB - AI Agent Guidelines

## Communication Style

**CRITICAL**: Keep responses **compact and concise**. 

**NO markdown files** unless explicitly requested. Focus on:
- Direct answers
- Code changes only
- Minimal explanations
- No over-engineering

## Project Overview

EventoDB is a high-performance event store/message store in Go. It supports multiple storage backends (SQLite, PostgreSQL, TimescaleDB, Pebble KV) with a unified HTTP/RPC API and SSE subscriptions.

**Key concepts**: Event Sourcing, Pub/Sub, Multi-tenancy (namespaces), Consumer Groups, Optimistic Locking

## Code Structure

```
golang/
├── cmd/eventodb/          # Server entry point
├── internal/
│   ├── api/               # HTTP handlers, RPC, SSE, middleware
│   ├── auth/              # Token-based authentication
│   ├── store/             # Storage layer (interface + backends)
│   │   ├── sqlite/        # SQLite backend
│   │   ├── postgres/      # PostgreSQL backend
│   │   ├── timescale/     # TimescaleDB backend
│   │   └── pebble/        # Pebble KV backend
│   ├── logger/            # Logging utilities
│   └── migrate/           # Database migrations
├── test_integration/      # Integration tests (multi-backend)
└── test_unit/             # Unit tests

clients/eventodb-go/       # Go SDK client
test_external/             # Black-box HTTP API tests (Bun/TypeScript)
bin/                       # Test runner scripts
docs/                      # Documentation
```

## Development Workflow

### Building
```bash
make build              # Builds to dist/eventodb
cd golang && go build -o eventodb ./cmd/eventodb
```

### Running
```bash
# Test mode (in-memory SQLite)
./dist/eventodb --test-mode --port=8080

# PostgreSQL
./dist/eventodb --db-url="postgres://user:pass@host/db" --port=8080

# Pebble KV (in-memory)
./dist/eventodb --db-url="pebble://memory" --test-mode --port=8080

# Pebble KV (persistent)
./dist/eventodb --db-url="pebble:///path/to/data" --port=8080

# SQLite (persistent)
./dist/eventodb --db-url="sqlite://eventodb.db" --data-dir=./data --port=8080
```

### Testing

**Use scripts in `bin/` for consistent test execution:**

```bash
# General tests + QA checks
bin/qa_check.sh

# Race detection
bin/race_check.sh

# Black-box tests (HTTP API) - different backends
bin/run_blackbox.sh sqlite
bin/run_blackbox.sh postgres
bin/run_blackbox.sh pebble

# Go SDK integration tests - different backends
bin/run_golang_sdk_specs.sh all
bin/run_golang_sdk_specs.sh sqlite TestSSE
bin/run_golang_sdk_specs.sh postgres TestWRITE

# Manual testing
cd golang && CGO_ENABLED=0 go test ./...
cd golang && go test ./... -race
```

## Key Design Principles

1. **Backend Abstraction**: All storage backends implement `store.Store` interface
2. **Namespace Isolation**: Multi-tenant via namespace tokens (`ns_<base64>_<signature>`)
3. **Immutable Events**: Events are append-only, never modified
4. **SSE for Real-time**: Server-Sent Events for pub/sub subscriptions
5. **RPC-style API**: Simple JSON-RPC over HTTP POST to `/rpc`
6. **Test Coverage**: Unit tests, integration tests (multi-backend), black-box tests

## Common Tasks

### Adding a New RPC Method
1. Define handler in `golang/internal/api/rpc_*.go`
2. Register in `golang/internal/api/server.go` RPC handler map
3. Add tests in `golang/test_integration/` for all backends
4. Add black-box tests in `test_external/`
5. Update `docs/API.md`

### Adding a New Storage Backend
1. Create package in `golang/internal/store/<backend>/`
2. Implement `store.Store` interface
3. Add initialization in `golang/cmd/eventodb/main.go`
4. Add tests in `golang/test_integration/` (test all backends)
5. Update `bin/run_golang_sdk_specs.sh` and `bin/run_blackbox.sh`

### Fixing Race Conditions
1. Run `bin/race_check.sh` to detect races
2. Focus on SSE subscriptions, concurrent map access, token management
3. Test with `go test -race` locally
4. Verify with `bin/race_check.sh` before committing

### Performance Testing
```bash
make benchmark-all
make profile-baseline
# Make changes...
make profile-baseline  # Creates new profile
# Compare: make profile-compare BASELINE=profiles/run1 OPTIMIZED=profiles/run2
```

## Common Patterns

### Error Handling
```go
// Return structured errors
return nil, fmt.Errorf("failed to write message: %w", err)

// API errors use RPC error format
return api.RPCError(code, message)
```

### Database Queries
```go
// Always use parameterized queries
rows, err := s.db.Query(ctx, "SELECT * FROM messages WHERE stream_name = $1", streamName)

// Scan into structs
var msg store.Message
err := row.Scan(&msg.ID, &msg.Type, &msg.Data, ...)
```

### Testing Multi-Backend
```go
// Test runs against all backends via TEST_BACKEND env var
backend := os.Getenv("TEST_BACKEND")
if backend == "" {
    backend = "sqlite"
}
```

## Documentation

- **API Reference**: `docs/API.md`
- **Deployment**: `docs/DEPLOYMENT.md`
- **Performance**: `docs/PERFORMANCE.md`
- **Migration**: `docs/MIGRATION.md`
- **Test Specs**: `docs/SDK-TEST-SPEC.md`
- **Test Runners**: `bin/README.md`

## Common Issues

### PostgreSQL not available
Scripts gracefully skip PostgreSQL tests if unavailable. Set connection via env vars:
```bash
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=postgres
export POSTGRES_PASSWORD=postgres
```

### Race detector on macOS Xcode 16+
`bin/race_check.sh` handles compatibility issues automatically. Uses Docker fallback if needed.

### Tests hanging
Check for goroutine leaks in SSE subscriptions. Always clean up:
```go
defer subscription.Close()
```

## Quick Reference

| Task | Command |
|------|---------|
| Build | `make build` |
| Run tests | `bin/qa_check.sh` |
| Race detection | `bin/race_check.sh` |
| Black-box tests | `bin/run_blackbox.sh sqlite` |
| SDK tests | `bin/run_golang_sdk_specs.sh all` |
| Benchmark | `make benchmark-all` |
| Format | `cd golang && gofmt -l -w .` |
| Vet | `cd golang && go vet ./...` |
