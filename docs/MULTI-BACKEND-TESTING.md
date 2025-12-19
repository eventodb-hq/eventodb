# Multi-Backend Testing Guide

## Overview

The MessageDB test suite supports running the same tests against multiple storage backends:
- **SQLite** - Embedded SQL database (default)
- **PostgreSQL** - Full-featured relational database
- **Pebble** - High-performance key-value store

This ensures consistent behavior across all backends and makes it easy to validate changes.

## Quick Start

### Test Against a Specific Backend

```bash
# SQLite (default)
cd golang
CGO_ENABLED=0 go test ./test_integration/

# Or explicitly
CGO_ENABLED=0 TEST_BACKEND=sqlite go test ./test_integration/

# PostgreSQL (requires running Postgres)
CGO_ENABLED=0 TEST_BACKEND=postgres go test ./test_integration/

# Pebble
CGO_ENABLED=0 TEST_BACKEND=pebble go test ./test_integration/
```

### Test Against All Backends

```bash
cd golang
./test_all_backends.sh

# Or specific tests
./test_all_backends.sh "-run TestSSE"
./test_all_backends.sh "-run TestWRITE"
```

## Environment Variables

### `TEST_BACKEND`

Controls which backend to use for tests:
- `sqlite` (default) - In-memory SQLite database
- `postgres` - PostgreSQL database
- `pebble` - Pebble key-value store

### PostgreSQL Configuration

When using `TEST_BACKEND=postgres`, you can configure the connection:

```bash
export POSTGRES_HOST=localhost      # Default: localhost
export POSTGRES_PORT=5432           # Default: 5432
export POSTGRES_USER=postgres       # Default: postgres
export POSTGRES_PASSWORD=postgres   # Default: postgres
export POSTGRES_DB=postgres         # Default: postgres

CGO_ENABLED=0 TEST_BACKEND=postgres go test ./test_integration/
```

### CGO_ENABLED=0

**Important**: Always run tests with `CGO_ENABLED=0` to ensure compatibility:
- SQLite uses pure Go implementation (modernc.org/sqlite)
- Pebble is pure Go
- PostgreSQL driver (pgx) is pure Go

## Test Infrastructure

### Backend Switching

The test infrastructure automatically adapts to the selected backend:

```go
// In test_helpers.go
func GetTestBackend() BackendType {
    backend := os.Getenv("TEST_BACKEND")
    switch backend {
    case "postgres", "postgresql":
        return BackendPostgres
    case "pebble":
        return BackendPebble
    default:
        return BackendSQLite
    }
}

func SetupTestEnv(t *testing.T) *TestEnv {
    backend := GetTestBackend()
    return SetupTestEnvWithBackend(t, backend)
}
```

### Backend-Specific Setup

Each backend has its own setup function:

- `setupSQLiteEnv(t)` - Creates in-memory SQLite database
- `setupPostgresEnv(t)` - Connects to PostgreSQL and creates test schema
- `setupPebbleEnv(t)` - Creates temporary directory for Pebble data

All setup functions:
1. Create a unique namespace for the test
2. Generate authentication token
3. Initialize the store
4. Return cleanup function

### Test Isolation

Each test gets:
- **Unique namespace** - Prevents test interference
- **Automatic cleanup** - Resources released after test
- **Fresh database** - SQLite uses new in-memory DB, Pebble uses temp dir, Postgres uses unique schema

## Writing Backend-Agnostic Tests

### Good Practices

✅ **Use test helpers**
```go
func TestMyFeature(t *testing.T) {
    ts := SetupTestServer(t)  // Automatically uses TEST_BACKEND
    defer ts.Cleanup()
    
    // Test logic here
}
```

✅ **Test behavior, not implementation**
```go
// Good - tests the result
result, err := makeRPCCall(t, ts.Port, ts.Token, "stream.write", stream, msg)
require.NoError(t, err)
assert.GreaterOrEqual(t, result["position"].(float64), 0.0)

// Avoid - assumes specific backend behavior
// Don't check internal database state
```

✅ **Use standard test data**
```go
// Consistent test data works across all backends
msg := map[string]interface{}{
    "type": "TestEvent",
    "data": map[string]interface{}{"foo": "bar"},
}
```

### Backend-Specific Tests

If you need to test backend-specific behavior:

```go
func TestBackendSpecificFeature(t *testing.T) {
    backend := GetTestBackend()
    
    if backend != BackendPostgres {
        t.Skip("This test only applies to PostgreSQL")
    }
    
    // Postgres-specific test
}
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test All Backends

jobs:
  test-sqlite:
    name: SQLite
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run tests
        run: |
          cd golang
          CGO_ENABLED=0 TEST_BACKEND=sqlite go test -v ./test_integration/

  test-postgres:
    name: PostgreSQL
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run tests
        env:
          TEST_BACKEND: postgres
          POSTGRES_HOST: localhost
          POSTGRES_PORT: 5432
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: postgres
        run: |
          cd golang
          CGO_ENABLED=0 go test -v ./test_integration/

  test-pebble:
    name: Pebble
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run tests
        run: |
          cd golang
          CGO_ENABLED=0 TEST_BACKEND=pebble go test -v ./test_integration/
```

## Backend Comparison

| Feature | SQLite | PostgreSQL | Pebble |
|---------|--------|------------|--------|
| **Setup** | Instant (in-memory) | Requires server | Instant (temp dir) |
| **Speed** | Fast | Medium | Very Fast |
| **Production Use** | Single-node | Multi-node capable | High-throughput |
| **Concurrency** | Limited | Excellent | Excellent |
| **Durability** | File-based | Transactional | LSM-tree |
| **Test Isolation** | Per-test DB | Per-test schema | Per-test dir |

## Troubleshooting

### PostgreSQL Not Available

If you see:
```
⚠️  Postgres not available, skipping
```

Make sure PostgreSQL is running:
```bash
# Check if Postgres is running
pg_isready -h localhost -p 5432

# Start Postgres (varies by OS)
# macOS with Homebrew:
brew services start postgresql

# Linux with systemd:
sudo systemctl start postgresql

# Docker:
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15
```

### Connection Refused

If tests fail with connection errors:
```
Failed to connect to PostgreSQL: connection refused
```

Check your connection settings:
```bash
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=postgres
export POSTGRES_PASSWORD=postgres
export POSTGRES_DB=postgres
```

### CGO Errors

If you see CGO-related errors:
```
cgo: exec gcc: not found
```

Always use `CGO_ENABLED=0`:
```bash
CGO_ENABLED=0 go test ./test_integration/
```

## Performance Tips

### Parallel Testing

Run tests in parallel for faster execution:
```bash
CGO_ENABLED=0 go test -parallel 4 ./test_integration/
```

### Specific Test Selection

Run only specific tests:
```bash
# Single test
CGO_ENABLED=0 go test -run TestSSE001 ./test_integration/

# Test category
CGO_ENABLED=0 go test -run TestSSE ./test_integration/
CGO_ENABLED=0 go test -run TestWRITE ./test_integration/
```

### Benchmark Comparison

Compare backend performance:
```bash
# SQLite
CGO_ENABLED=0 TEST_BACKEND=sqlite go test -bench=. ./test_integration/

# Pebble
CGO_ENABLED=0 TEST_BACKEND=pebble go test -bench=. ./test_integration/

# Postgres
CGO_ENABLED=0 TEST_BACKEND=postgres go test -bench=. ./test_integration/
```

## Next Steps

- See `docs/SDK-TEST-SPEC.md` for complete test specification
- See `docs/SSE-TEST-FIXES.md` for SSE implementation details
- See `@meta/@pm/issues/ISSUE005-golang-based-sdk-spec.md` for original proposal
