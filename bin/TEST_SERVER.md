# EventoDB Test Server Management

## Overview

The `manage_test_server.sh` script provides unified test server management across all test scripts. It eliminates duplicated logic for starting, stopping, and configuring test servers.

## Usage

### Basic Commands

```bash
# Start a test server
bin/manage_test_server.sh start [backend] [port]

# Stop a test server
bin/manage_test_server.sh stop [port]

# Restart a test server
bin/manage_test_server.sh restart [backend] [port]

# Check server status
bin/manage_test_server.sh status [port]

# Wait for server to be ready
bin/manage_test_server.sh wait [port] [timeout]

# Cleanup backend test data
bin/manage_test_server.sh cleanup [backend]
```

### Examples

```bash
# Start SQLite test server on default port (6789)
bin/manage_test_server.sh start sqlite

# Start PostgreSQL test server on port 8080
bin/manage_test_server.sh start postgres 8080

# Start Pebble with test mode flag
EVENTODB_TEST_MODE=1 bin/manage_test_server.sh start pebble

# Check if server is running
bin/manage_test_server.sh status 8080

# Stop server
bin/manage_test_server.sh stop 8080
```

## Supported Backends

- **sqlite** - In-memory SQLite (default, no setup required)
- **postgres** - PostgreSQL (requires running PostgreSQL server)
- **pebble** - Pebble embedded KV store (in-memory)
- **timescale** - TimescaleDB (requires TimescaleDB extension)

## Configuration

### Environment Variables

#### General
- `EVENTODB_TEST_MODE=1` - Use `--test-mode` flag (disables auth, uses in-memory)
- `EVENTODB_LOG_FILE=/path/to/log` - Custom log file path (default: `/tmp/eventodb_test_<port>.log`)

#### PostgreSQL
- `POSTGRES_HOST` - PostgreSQL host (default: localhost)
- `POSTGRES_PORT` - PostgreSQL port (default: 5432)
- `POSTGRES_USER` - PostgreSQL user (default: postgres)
- `POSTGRES_PASSWORD` - PostgreSQL password (default: postgres)
- `POSTGRES_DB` - PostgreSQL database (default: eventodb_store)

#### TimescaleDB
- `TIMESCALE_HOST` - TimescaleDB host (default: localhost)
- `TIMESCALE_PORT` - TimescaleDB port (default: 6666)
- `TIMESCALE_USER` - TimescaleDB user (default: postgres)
- `TIMESCALE_PASSWORD` - TimescaleDB password (default: postgres)
- `TIMESCALE_DB` - TimescaleDB database (default: eventodb_timescale_test)

### Default Token

All test servers use the same default admin token:
```
ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000
```

## Integration in Test Scripts

### Pattern

```bash
#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT=8080
BACKEND="sqlite"
ADMIN_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Cleanup on exit
cleanup() {
    "$SCRIPT_DIR/manage_test_server.sh" stop "$PORT" || true
}
trap cleanup EXIT

# Start server
if "$SCRIPT_DIR/manage_test_server.sh" start "$BACKEND" "$PORT"; then
    echo "✓ Server ready"
else
    echo "✗ Failed to start server"
    exit 1
fi

# Run tests
export EVENTODB_URL="http://localhost:$PORT"
export EVENTODB_ADMIN_TOKEN="$ADMIN_TOKEN"
# ... your test commands here ...
```

### Example: Elixir Kit Tests

```bash
# bin/run_elixir_kit_specs.sh (simplified)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT=8080

cleanup() {
    "$SCRIPT_DIR/manage_test_server.sh" stop "$PORT" || true
}
trap cleanup EXIT

if "$SCRIPT_DIR/manage_test_server.sh" start sqlite "$PORT"; then
    export EVENTODB_URL="http://localhost:$PORT"
    export EVENTODB_ADMIN_TOKEN="ns_ZGVmYXVsdA_..."
    cd clients/eventodb_kit && mix test
fi
```

## Features

### Automatic Cleanup
- Kills existing processes on target port
- Removes temporary data directories
- Cleans up test data in PostgreSQL/TimescaleDB
- Tracks PIDs and data directories for proper cleanup

### Health Checks
- Waits for server to be ready before returning
- Configurable timeout
- Shows server logs on failure

### Backend Setup
- PostgreSQL: Cleans up old test namespaces before starting
- TimescaleDB: Creates fresh test database with extension
- SQLite/Pebble: Uses in-memory mode (no setup needed)

### Build Management
- Automatically builds server if binary doesn't exist
- Ensures latest code is used

## Migration Guide

### Old Pattern (run_sdk_tests.sh)
```bash
# Kill existing processes
if lsof -ti:$PORT > /dev/null 2>&1; then
    kill $(lsof -ti:$PORT) 2>/dev/null || true
fi

# Build server
cd golang && CGO_ENABLED=0 go build -o ../dist/eventodb ./cmd/eventodb

# Start server
TEST_DATA_DIR="/tmp/eventodb-sdk-test-$$"
mkdir -p "$TEST_DATA_DIR"
./dist/eventodb -db-url "sqlite://:memory:" -data-dir "$TEST_DATA_DIR" -port $PORT &
SERVER_PID=$!

# Wait for ready
for i in {1..30}; do
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        break
    fi
    sleep 0.1
done

# Cleanup
cleanup() {
    kill $SERVER_PID 2>/dev/null || true
    rm -rf "$TEST_DATA_DIR"
}
trap cleanup EXIT
```

### New Pattern
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cleanup() {
    "$SCRIPT_DIR/manage_test_server.sh" stop "$PORT" || true
}
trap cleanup EXIT

"$SCRIPT_DIR/manage_test_server.sh" start sqlite "$PORT"
```

## Benefits

1. **DRY**: Single source of truth for server management logic
2. **Consistency**: All tests use same startup/shutdown patterns
3. **Reliability**: Proper cleanup even on script failure
4. **Debugging**: Centralized logging and error handling
5. **Flexibility**: Easy to add new backends or configuration options
6. **Maintainability**: Fix bugs in one place, all scripts benefit

## Testing the Manager

```bash
# Start a test server
bin/manage_test_server.sh start sqlite 9999

# In another terminal, check status
bin/manage_test_server.sh status 9999

# Test health endpoint
curl http://localhost:9999/health

# Stop server
bin/manage_test_server.sh stop 9999
```

## Troubleshooting

### Server won't start
```bash
# Check logs
tail -f /tmp/eventodb_test_<port>.log

# Try with verbose logging
EVENTODB_LOG_FILE=/tmp/debug.log bin/manage_test_server.sh start sqlite 9999
tail -f /tmp/debug.log
```

### Port already in use
```bash
# Force stop any process on port
bin/manage_test_server.sh stop 9999

# Or manually
lsof -ti:9999 | xargs kill -9
```

### PostgreSQL connection issues
```bash
# Verify PostgreSQL is running
pg_isready -h localhost -p 5432

# Test connection
PGPASSWORD=postgres psql -h localhost -U postgres -c "SELECT 1"

# Check environment variables
echo $POSTGRES_HOST $POSTGRES_PORT $POSTGRES_USER
```

## Future Enhancements

- [ ] Support for multiple concurrent test servers
- [ ] Docker-based PostgreSQL/TimescaleDB setup
- [ ] Server health monitoring with automatic restart
- [ ] Performance profiling integration
- [ ] Custom server flags passthrough
- [ ] Test isolation (namespace per test run)
