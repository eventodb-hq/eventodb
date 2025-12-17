# Message DB - Golang Backend

This directory contains the Golang implementation of the Message DB service, which provides a modern HTTP/SSE API on top of the Message DB PostgreSQL database.

## Structure

- `cmd/messagedb/` - Main application entry point
- `internal/` - Internal packages
  - `api/` - HTTP handlers, SSE, RPC endpoints, and middleware
  - `auth/` - Authentication and token handling
  - `migrate/` - Database migration logic
  - `store/` - Data access layer for Message DB operations
- `migrations/` - Embedded SQL migration files
- `test_integration/` - Integration tests
- `test_unit/` - Unit tests

## Building

From this directory:

```bash
go build -o messagedb ./cmd/messagedb
```

Or from the project root:

```bash
cd golang && go build -o messagedb ./cmd/messagedb
```

## Running

```bash
./messagedb
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
./bin/qa_check_messagedb.sh
```

Or from this directory:

```bash
./qa_check_go.sh
```

## Development

This Golang backend is designed to work alongside the original PostgreSQL-based Message DB implementation found in the `../database/` directory. It provides a REST API and SSE-based subscriptions for event streaming.
