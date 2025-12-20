# EventoDB Test Runners

## Quick Reference

```bash
# Blackbox tests (external API via HTTP)
bin/run_blackbox.sh sqlite       # Fast, in-memory
bin/run_blackbox.sh postgres     # PostgreSQL backend
bin/run_blackbox.sh pebble       # Embedded KV store
bin/run_blackbox.sh timescale    # TimescaleDB (optional)

# Go SDK integration tests
bin/run_golang_sdk_specs.sh sqlite
bin/run_golang_sdk_specs.sh postgres
bin/run_golang_sdk_specs.sh pebble
bin/run_golang_sdk_specs.sh all  # Run all backends
```

---

## run_blackbox.sh

Run external (blackbox) tests against EventoDB HTTP API with different storage backends.

### Usage

```bash
bin/run_blackbox.sh <backend>
```

### Backends

- **sqlite** - In-memory SQLite (default, no setup required)
- **postgres** - PostgreSQL (requires running PostgreSQL)
- **pebble** - Pebble embedded KV store
- **timescale** - TimescaleDB (requires TimescaleDB extension)

### Examples

```bash
# Run with SQLite (fastest, no setup)
bin/run_blackbox.sh sqlite

# Run with PostgreSQL
bin/run_blackbox.sh postgres

# Run with Pebble
bin/run_blackbox.sh pebble

# Run with TimescaleDB (requires setup)
TIMESCALE_PORT=6666 bin/run_blackbox.sh timescale
```

### Environment Variables

**PostgreSQL:**
- `POSTGRES_HOST` - Host (default: `localhost`)
- `POSTGRES_PORT` - Port (default: `5432`)
- `POSTGRES_USER` - User (default: `postgres`)
- `POSTGRES_PASSWORD` - Password (default: `postgres`)
- `POSTGRES_DB` - Database (default: `eventodb_store`)

**TimescaleDB:**
- `TIMESCALE_HOST` - Host (default: `localhost`)
- `TIMESCALE_PORT` - Port (default: `6666`)
- `TIMESCALE_USER` - User (default: `postgres`)
- `TIMESCALE_PASSWORD` - Password (default: `postgres`)
- `TIMESCALE_DB` - Database (default: `eventodb_timescale_test`)
- `KEEP_TEST_DB` - Set to `1` to keep test database after run

### Features

- ✅ Single, simple script for all backends
- ✅ Automatic server lifecycle management
- ✅ Clean setup and teardown
- ✅ Colorized output with backend-specific icons
- ✅ Automatic database cleanup
- ✅ Server health checks
- ✅ Clear error messages with logs

### Output

```
╔══════════════════════════════════════════════════════════════╗
║  📦 EventoDB Blackbox Tests - SQLITE
╚══════════════════════════════════════════════════════════════╝

Backend: SQLite (in-memory)
Starting server...
Waiting for server to be ready...
Server ready!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Running tests...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

 95 pass
 0 fail

╔══════════════════════════════════════════════════════════════╗
║  📦 All tests PASSED with SQLITE backend!
╚══════════════════════════════════════════════════════════════╝
```

### Requirements

- **All:** `bun` installed, `dist/eventodb` binary built
- **PostgreSQL:** PostgreSQL server running, `psql` client installed
- **TimescaleDB:** TimescaleDB-enabled PostgreSQL, `psql` client installed
- **Pebble:** No external dependencies

### Exit Codes

- `0` - All tests passed
- `1` - Tests failed or setup error

---

## run_golang_sdk_specs.sh

Run Go SDK specification tests against one or all storage backends.

### Usage

```bash
bin/run_golang_sdk_specs.sh [backend] [test_pattern]
```

### Arguments

- **backend** - Which backend to test: `sqlite`, `postgres`, `pebble`, or `all` (default: `all`)
- **test_pattern** - Test pattern to match (optional, e.g., `TestSSE`, `WRITE`)

### Examples

```bash
# Run all tests on all backends
bin/run_golang_sdk_specs.sh

# Run all tests on SQLite only
bin/run_golang_sdk_specs.sh sqlite

# Run SSE tests on all backends
bin/run_golang_sdk_specs.sh all TestSSE

# Run Write tests on Pebble only
bin/run_golang_sdk_specs.sh pebble WRITE

# Run specific test on specific backend
bin/run_golang_sdk_specs.sh sqlite TestSSE001

# Show help
bin/run_golang_sdk_specs.sh --help
```

### Output

The script provides colorized output with:
- 📦 SQLite backend indicator
- 🐘 PostgreSQL backend indicator  
- 🪨 Pebble backend indicator
- ✅ Pass status in green
- ❌ Fail status in red
- ⚠️  Skip status in yellow

Example output:
```
=========================================
Go SDK Spec Tests
=========================================
Backend: all
Pattern: TestSSE
=========================================

=========================================
📦 Testing SQLITE backend
=========================================
ok  	github.com/eventodb/eventodb/test_integration	0.983s
✅ sqlite PASSED

=========================================
🐘 Testing POSTGRES backend
=========================================
ok  	github.com/eventodb/eventodb/test_integration	1.042s
✅ postgres PASSED

=========================================
🪨 Testing PEBBLE backend
=========================================
ok  	github.com/eventodb/eventodb/test_integration	1.641s
✅ pebble PASSED

=========================================
Summary
=========================================
📦 sqlite  : ✅ PASS
🐘 postgres: ✅ PASS
🪨 pebble  : ✅ PASS
=========================================
✅ All tests passed!
```

### Environment Variables

For PostgreSQL configuration:
- `POSTGRES_HOST` - PostgreSQL host (default: `localhost`)
- `POSTGRES_PORT` - PostgreSQL port (default: `5432`)
- `POSTGRES_USER` - PostgreSQL user (default: `postgres`)
- `POSTGRES_PASSWORD` - PostgreSQL password (default: `postgres`)
- `POSTGRES_DB` - PostgreSQL database (default: `postgres`)

Example:
```bash
export POSTGRES_HOST=myhost
export POSTGRES_PORT=5433
bin/run_golang_sdk_specs.sh postgres
```

### Requirements

- Go 1.21+ installed
- For PostgreSQL tests: PostgreSQL server running and accessible
- CGO disabled (script sets `CGO_ENABLED=0` automatically)

### Exit Codes

- `0` - All tests passed
- `1` - One or more tests failed
- `2` - Invalid arguments or missing requirements

### Notes

- Tests always run with `CGO_ENABLED=0` for portability
- Each test gets isolated environment (unique namespace, temp directories)
- PostgreSQL tests are automatically skipped if server is not available
- Tests can be run in parallel with Go's `-parallel` flag (not default)

## Common Test Patterns

### By Feature Category

```bash
bin/run_golang_sdk_specs.sh all TestWRITE    # Write operations
bin/run_golang_sdk_specs.sh all TestREAD     # Read operations
bin/run_golang_sdk_specs.sh all TestSSE      # Server-Sent Events
bin/run_golang_sdk_specs.sh all TestCATEGORY # Category operations
bin/run_golang_sdk_specs.sh all TestNS       # Namespace operations
```

### By Backend Performance

```bash
# Fastest - SQLite in-memory
bin/run_golang_sdk_specs.sh sqlite

# Medium - Pebble on disk
bin/run_golang_sdk_specs.sh pebble

# Requires external service - PostgreSQL
bin/run_golang_sdk_specs.sh postgres
```

### CI/CD Usage

```bash
# In CI, test all backends
bin/run_golang_sdk_specs.sh all

# Or test specific critical paths
bin/run_golang_sdk_specs.sh all TestSSE
bin/run_golang_sdk_specs.sh all TestWRITE
```

## Troubleshooting

### PostgreSQL Not Available

If you see:
```
⚠️  PostgreSQL not available, skipping
```

Start PostgreSQL:
```bash
# macOS with Homebrew
brew services start postgresql

# Docker
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15

# Check if running
pg_isready -h localhost -p 5432
```

### All Tests Fail

Check you're in the project root:
```bash
# Should see bin/, golang/, docs/
ls

# Run from root
bin/run_golang_sdk_specs.sh
```

### Specific Test Fails

Run verbose to see details:
```bash
cd golang
CGO_ENABLED=0 TEST_BACKEND=sqlite go test -v -run TestFailingTest ./test_integration/
```

## See Also

- `docs/SDK-TEST-SPEC.md` - Complete test specification
- `docs/MULTI-BACKEND-TESTING.md` - Multi-backend testing guide
- `docs/SSE-TEST-FIXES.md` - SSE implementation details
- `BACKEND-TESTING-SUMMARY.md` - Summary of test fixes
