# Test Server Quick Reference

## Common Commands

```bash
# Start test server (SQLite, test mode)
EVENTODB_TEST_MODE=1 bin/manage_test_server.sh start sqlite 8080

# Start production-like server (SQLite with auth)
bin/manage_test_server.sh start sqlite 8080

# Start PostgreSQL server
bin/manage_test_server.sh start postgres 6789

# Check if running
bin/manage_test_server.sh status 8080

# Stop server
bin/manage_test_server.sh stop 8080

# Restart with different backend
bin/manage_test_server.sh restart pebble 8080
```

## Standard Test Pattern

```bash
#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT=8080
BACKEND="sqlite"
TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Auto-cleanup on exit
cleanup() {
    "$SCRIPT_DIR/manage_test_server.sh" stop "$PORT" || true
}
trap cleanup EXIT

# Start server
"$SCRIPT_DIR/manage_test_server.sh" start "$BACKEND" "$PORT"

# Export for tests
export EVENTODB_URL="http://localhost:$PORT"
export EVENTODB_ADMIN_TOKEN="$TOKEN"

# Run your tests here
cd test_dir && ./run_tests.sh
```

## Backend Shortcuts

| Backend | Command | Use Case |
|---------|---------|----------|
| SQLite (test) | `EVENTODB_TEST_MODE=1 ... start sqlite` | Fast, no auth |
| SQLite (auth) | `... start sqlite` | Production-like |
| PostgreSQL | `... start postgres` | Multi-tenant tests |
| Pebble | `... start pebble` | KV store tests |
| TimescaleDB | `... start timescale` | Time-series tests |

## Environment Setup

```bash
# PostgreSQL
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_USER=postgres
export POSTGRES_PASSWORD=postgres
export POSTGRES_DB=eventodb_store

# TimescaleDB  
export TIMESCALE_HOST=localhost
export TIMESCALE_PORT=6666
export TIMESCALE_DB=eventodb_timescale_test

# Test mode (no auth, in-memory)
export EVENTODB_TEST_MODE=1

# Custom log location
export EVENTODB_LOG_FILE=/tmp/my_test.log
```

## Troubleshooting

```bash
# Port stuck?
lsof -ti:8080 | xargs kill -9
bin/manage_test_server.sh stop 8080

# Check server logs
tail -f /tmp/eventodb_test_8080.log

# Verify server is responding
curl http://localhost:8080/health

# Clean up test data (PostgreSQL)
bin/manage_test_server.sh cleanup postgres
```

## Standard Token

All test servers use this admin token:
```
ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000
```

## Integration Examples

### Elixir Tests
```bash
bin/run_elixir_kit_specs.sh
# Uses: sqlite on port 8080, test mode
```

### Node.js Tests
```bash
bin/run_sdk_tests.sh node
# Uses: sqlite on port 6789, auth enabled
```

### Blackbox Tests
```bash
bin/run_blackbox.sh postgres
# Uses: postgres on port 6789, test mode
```

### Go SDK Tests
```bash
bin/run_golang_sdk_specs.sh sqlite
# Starts own servers, uses manage_test_server.sh internally
```

## Default Ports

| Script | Port | Reason |
|--------|------|--------|
| run_elixir_kit_specs.sh | 8080 | Avoid conflicts with SDK tests |
| run_sdk_tests.sh | 6789 | Historical default |
| run_blackbox.sh | 6789 | Historical default |
| Manual testing | 8080 | User-facing default |

## Log Files

| Server Port | Log Location |
|-------------|--------------|
| 8080 | `/tmp/eventodb_test_8080.log` |
| 6789 | `/tmp/eventodb_test_6789.log` |
| Custom | `$EVENTODB_LOG_FILE` |

## Data Directories

SQLite creates temp directories:
```
/tmp/eventodb-test-<port>-<pid>/
```

Auto-cleaned on server stop.

## See Also

- `bin/TEST_SERVER.md` - Full documentation
- `bin/MIGRATION_SUMMARY.md` - Migration guide
- `bin/manage_test_server.sh --help` - Command help
