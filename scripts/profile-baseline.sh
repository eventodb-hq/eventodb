#!/bin/bash
# Baseline performance profiling for Message DB
# Generates CPU, memory, and allocation profiles

set -e

# Parse arguments
DB_TYPE="${1:-sqlite}"  # default to sqlite for backward compatibility
PROFILE_SUFFIX="${DB_TYPE}"

# PostgreSQL connection settings (only used if DB_TYPE=postgres or DB_TYPE=timescale)
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-message_store}"

PROFILE_DIR="./profiles/$(date +%Y%m%d_%H%M%S)-${PROFILE_SUFFIX}"
mkdir -p "$PROFILE_DIR"

echo "=== Message DB Performance Profiling ==="
echo "Database Type: $DB_TYPE"
echo "Profile directory: $PROFILE_DIR"

# Build server with profiling enabled
echo "Building server..."
cd golang
go build -o ../dist/messagedb-profile ./cmd/messagedb
cd ..

# Start server with pprof enabled
echo "Starting server..."

if [ "$DB_TYPE" = "sqlite" ]; then
  # SQLite test mode
  ./dist/messagedb-profile \
    --test-mode \
    --port 8080 \
    --log-level warn &
  SERVER_PID=$!
elif [ "$DB_TYPE" = "postgres" ] || [ "$DB_TYPE" = "timescale" ]; then
  # PostgreSQL/TimescaleDB mode
  DB_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable"
  
  # Clean up any old test namespaces
  if command -v psql &> /dev/null; then
    echo "Cleaning up old test namespaces..."
    PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "
      DO \$\$
      DECLARE
        ns RECORD;
      BEGIN
        FOR ns IN SELECT schema_name FROM message_store.namespaces WHERE id = 'default' OR id LIKE 'test_%'
        LOOP
          EXECUTE 'DROP SCHEMA IF EXISTS \"' || ns.schema_name || '\" CASCADE';
        END LOOP;
        DELETE FROM message_store.namespaces WHERE id = 'default' OR id LIKE 'test_%';
      EXCEPTION
        WHEN undefined_table THEN NULL;
        WHEN invalid_schema_name THEN NULL;
      END \$\$;
    " 2>/dev/null || true
  fi
  
  # Use a deterministic token for profiling to avoid auth issues
  PROFILE_TOKEN="ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"
  
  if [ "$DB_TYPE" = "timescale" ]; then
    ./dist/messagedb-profile \
      --port 8080 \
      --db-url "$DB_URL" \
      --db-type timescale \
      --token "$PROFILE_TOKEN" \
      --log-level warn &
  else
    ./dist/messagedb-profile \
      --port 8080 \
      --db-url "$DB_URL" \
      --token "$PROFILE_TOKEN" \
      --log-level warn &
  fi
  SERVER_PID=$!
else
  echo "Error: Unknown database type '$DB_TYPE'. Use: sqlite, postgres, or timescale"
  exit 1
fi

echo "Server PID: $SERVER_PID"

# Cleanup function
cleanup() {
  echo "Stopping server..."
  kill $SERVER_PID 2>/dev/null || true
  wait $SERVER_PID 2>/dev/null || true
  
  # Clean up PostgreSQL test namespaces
  if [ "$DB_TYPE" = "postgres" ] || [ "$DB_TYPE" = "timescale" ]; then
    if command -v psql &> /dev/null; then
      echo "Cleaning up test namespaces..."
      PGPASSWORD="${POSTGRES_PASSWORD}" psql -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -c "
        DO \$\$
        DECLARE
          ns RECORD;
        BEGIN
          FOR ns IN SELECT schema_name FROM message_store.namespaces WHERE id = 'default' OR id LIKE 'test_%'
          LOOP
            EXECUTE 'DROP SCHEMA IF EXISTS \"' || ns.schema_name || '\" CASCADE';
          END LOOP;
          DELETE FROM message_store.namespaces WHERE id = 'default' OR id LIKE 'test_%';
        EXCEPTION
          WHEN undefined_table THEN NULL;
          WHEN invalid_schema_name THEN NULL;
        END \$\$;
      " 2>/dev/null || true
    fi
  fi
}
trap cleanup EXIT

# Wait for server to be ready
echo "Waiting for server to be ready..."
sleep 2
for i in {1..30}; do
  if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "Server is ready!"
    break
  fi
  sleep 1
  if [ $i -eq 30 ]; then
    echo "Server failed to start"
    exit 1
  fi
done

# Capture baseline profiles BEFORE load
echo "Capturing pre-load baseline..."
curl -s http://localhost:8080/debug/pprof/heap > "$PROFILE_DIR/heap-before.prof"
curl -s http://localhost:8080/debug/pprof/allocs > "$PROFILE_DIR/allocs-before.prof"

# Verify profiles were captured
if [ ! -s "$PROFILE_DIR/heap-before.prof" ]; then
  echo "Warning: Failed to capture heap-before.prof"
fi
if [ ! -s "$PROFILE_DIR/allocs-before.prof" ]; then
  echo "Warning: Failed to capture allocs-before.prof"
fi

# Run load test
echo "Running load test (30 seconds)..."
# For PostgreSQL/TimescaleDB, use the deterministic token
if [ "$DB_TYPE" = "postgres" ] || [ "$DB_TYPE" = "timescale" ]; then
  export DEFAULT_TOKEN="ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"
fi
go run ./scripts/load-test.go \
  --duration 30s \
  --workers 10 \
  --profile-dir "$PROFILE_DIR" &

LOAD_PID=$!

# Capture CPU profile during load (at midpoint)
sleep 15
echo "Capturing CPU profile..."
curl -s "http://localhost:8080/debug/pprof/profile?seconds=10" > "$PROFILE_DIR/cpu.prof" &

# Wait for load test to complete
wait $LOAD_PID

# Capture post-load profiles
echo "Capturing post-load profiles..."
curl -s http://localhost:8080/debug/pprof/heap > "$PROFILE_DIR/heap-after.prof"
curl -s http://localhost:8080/debug/pprof/allocs > "$PROFILE_DIR/allocs-after.prof"
curl -s http://localhost:8080/debug/pprof/goroutine > "$PROFILE_DIR/goroutine.prof"

echo ""
echo "=== Profile Analysis ==="
echo ""

# Verify profile file exists and is valid
if [ ! -s "$PROFILE_DIR/allocs-after.prof" ]; then
  echo "Error: allocs-after.prof is empty or missing"
else
  echo "Top 10 allocation sources:"
  if ! go tool pprof -top -alloc_space "$PROFILE_DIR/allocs-after.prof" 2>&1 | head -20; then
    echo "Warning: Could not parse allocation profile"
    echo "File info:"
    ls -lh "$PROFILE_DIR/allocs-after.prof"
    echo "First 100 bytes:"
    head -c 100 "$PROFILE_DIR/allocs-after.prof" | od -c
  fi
fi

echo ""
echo "=== Profiles saved to: $PROFILE_DIR ==="
echo ""
echo "Analysis commands:"
echo "  CPU:         go tool pprof $PROFILE_DIR/cpu.prof"
echo "  Allocations: go tool pprof -alloc_space $PROFILE_DIR/allocs-after.prof"
echo "  Heap:        go tool pprof -inuse_space $PROFILE_DIR/heap-after.prof"
echo ""
echo "Web UI:        go tool pprof -http=:9090 $PROFILE_DIR/allocs-after.prof"
echo ""

# Save summary
cat > "$PROFILE_DIR/README.md" << EOF
# Profile Results

**Date**: $(date)
**Database**: $DB_TYPE
**Duration**: 30 seconds
**Workers**: 10

## Files

- \`cpu.prof\` - CPU profile (10 seconds during load)
- \`heap-before.prof\` - Heap snapshot before load
- \`heap-after.prof\` - Heap snapshot after load
- \`allocs-before.prof\` - Allocations before load
- \`allocs-after.prof\` - Allocations after load (primary analysis target)
- \`goroutine.prof\` - Goroutine snapshot
- \`load-test-results.json\` - Load test metrics

## Quick Analysis

\`\`\`bash
# View top allocations
go tool pprof -top -alloc_space allocs-after.prof

# Interactive analysis
go tool pprof allocs-after.prof

# Web UI
go tool pprof -http=:9090 allocs-after.prof
\`\`\`

## Load Test Results

$(cat "$PROFILE_DIR/load-test-results.json" 2>/dev/null || echo "No results file")
EOF

echo "Profile complete! See $PROFILE_DIR/README.md for details"
