# EventoDB - Golang Backend

This directory contains the Golang implementation of the EventoDB service, which provides a modern HTTP/SSE API on top of the EventoDB PostgreSQL database.

## Structure

- `cmd/eventodb/` - Main application entry point
- `internal/` - Internal packages
  - `api/` - HTTP handlers, SSE, RPC endpoints, and middleware
  - `auth/` - Authentication and token handling
  - `migrate/` - Database migration logic
  - `store/` - Data access layer for EventoDB operations
- `migrations/` - Embedded SQL migration files
- `test_integration/` - Integration tests
- `test_unit/` - Unit tests

## Building

From this directory:

```bash
go build -o eventodb ./cmd/eventodb
```

Or from the project root:

```bash
cd golang && go build -o eventodb ./cmd/eventodb
```

## Running

```bash
./eventodb
```

See the main project README for configuration options and environment variables.

## Testing

Run all tests:

```bash
go test ./...
```

Run with race detector:

```bash
go test ./... -race
```

Run QA checks (from project root):

```bash
./bin/qa_check_eventodb.sh
```

Or from this directory:

```bash
./qa_check_go.sh
```

## Development

This Golang backend is designed to work alongside the original PostgreSQL-based EventoDB implementation found in the `../database/` directory. It provides a REST API and SSE-based subscriptions for event streaming.
