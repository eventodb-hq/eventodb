#!/bin/bash
# Pebble performance profiling for Message DB
# Generates CPU, memory, and allocation profiles

set -e

# Pebble data directory
PEBBLE_DATA_DIR="${PEBBLE_DATA_DIR:-/tmp/messagedb-pebble-profile}"
PROFILE_DIR="./profiles/$(date +%Y%m%d_%H%M%S)-pebble"
mkdir -p "$PROFILE_DIR"

echo "=== Message DB Performance Profiling (Pebble) ==="
echo "Database Type: Pebble"
echo "Data Directory: $PEBBLE_DATA_DIR"
echo "Profile directory: $PROFILE_DIR"

# Clean up old data
if [ -d "$PEBBLE_DATA_DIR" ]; then
  echo "Cleaning up old Pebble data..."
  rm -rf "$PEBBLE_DATA_DIR"
fi

# Build server with profiling enabled
echo "Building server..."
cd golang
timeout 60s env CGO_ENABLED=0 go build -o ../dist/messagedb-profile ./cmd/messagedb
cd ..

# Use a deterministic token for profiling
PROFILE_TOKEN="ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"

# Start server with pprof enabled
echo "Starting Pebble server..."
./dist/messagedb-profile \
  --port 8080 \
  --db-url "pebble://$PEBBLE_DATA_DIR" \
  --token "$PROFILE_TOKEN" \
  --log-level warn &
SERVER_PID=$!

echo "Server PID: $SERVER_PID"

# Cleanup function
cleanup() {
  echo "Stopping server..."
  kill $SERVER_PID 2>/dev/null || true
  wait $SERVER_PID 2>/dev/null || true
  
  echo "Cleaning up Pebble data..."
  rm -rf "$PEBBLE_DATA_DIR"
}
trap cleanup EXIT

# Wait for server to be ready
echo "Waiting for server to be ready..."
sleep 2
for i in {1..30}; do
  if timeout 5s curl -s http://localhost:8080/health > /dev/null 2>&1; then
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
timeout 10s curl -s http://localhost:8080/debug/pprof/heap > "$PROFILE_DIR/heap-before.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/allocs > "$PROFILE_DIR/allocs-before.prof"

# Verify profiles were captured
if [ ! -s "$PROFILE_DIR/heap-before.prof" ]; then
  echo "Warning: Failed to capture heap-before.prof"
fi
if [ ! -s "$PROFILE_DIR/allocs-before.prof" ]; then
  echo "Warning: Failed to capture allocs-before.prof"
fi

# Run load test
echo "Running load test (30 seconds)..."
export DEFAULT_TOKEN="ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"
timeout 60s go run ./scripts/load-test.go \
  --duration 30s \
  --workers 10 \
  --profile-dir "$PROFILE_DIR" &

LOAD_PID=$!

# Capture CPU profile during load (at midpoint)
sleep 15
echo "Capturing CPU profile..."
timeout 20s curl -s "http://localhost:8080/debug/pprof/profile?seconds=10" > "$PROFILE_DIR/cpu.prof" &

# Wait for load test to complete
wait $LOAD_PID

# Capture post-load profiles
echo "Capturing post-load profiles..."
timeout 10s curl -s http://localhost:8080/debug/pprof/heap > "$PROFILE_DIR/heap-after.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/allocs > "$PROFILE_DIR/allocs-after.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/goroutine > "$PROFILE_DIR/goroutine.prof"

# Capture Pebble-specific metrics
echo "Capturing Pebble metrics..."
if [ -d "$PEBBLE_DATA_DIR/_metadata" ]; then
  du -sh "$PEBBLE_DATA_DIR"/* > "$PROFILE_DIR/pebble-disk-usage.txt" || true
fi

# Count number of open namespace DBs
ls -1d "$PEBBLE_DATA_DIR"/*/ 2>/dev/null | wc -l > "$PROFILE_DIR/pebble-namespace-count.txt" || echo "0" > "$PROFILE_DIR/pebble-namespace-count.txt"

echo ""
echo "=== Profile Analysis ==="
echo ""

# Verify profile file exists and is valid
if [ ! -s "$PROFILE_DIR/allocs-after.prof" ]; then
  echo "Error: allocs-after.prof is empty or missing"
else
  echo "Top 10 allocation sources:"
  if ! timeout 30s go tool pprof -top -alloc_space "$PROFILE_DIR/allocs-after.prof" 2>&1 | head -20; then
    echo "Warning: Could not parse allocation profile"
    echo "File info:"
    ls -lh "$PROFILE_DIR/allocs-after.prof"
    echo "First 100 bytes:"
    head -c 100 "$PROFILE_DIR/allocs-after.prof" | od -c
  fi
fi

echo ""
echo "=== Pebble Metrics ==="
echo ""
echo "Disk usage:"
cat "$PROFILE_DIR/pebble-disk-usage.txt" 2>/dev/null || echo "No disk usage data"
echo ""
echo "Namespace count:"
cat "$PROFILE_DIR/pebble-namespace-count.txt" 2>/dev/null || echo "0"
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
# Profile Results (Pebble)

**Date**: $(date)
**Database**: Pebble
**Data Directory**: $PEBBLE_DATA_DIR
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
- \`pebble-disk-usage.txt\` - Pebble disk usage by namespace
- \`pebble-namespace-count.txt\` - Number of open namespaces

## Quick Analysis

\`\`\`bash
# View top allocations
go tool pprof -top -alloc_space allocs-after.prof

# Interactive analysis
go tool pprof allocs-after.prof

# Web UI
go tool pprof -http=:9090 allocs-after.prof
\`\`\`

## Pebble Metrics

**Disk Usage:**
\`\`\`
$(cat "$PROFILE_DIR/pebble-disk-usage.txt" 2>/dev/null || echo "No data")
\`\`\`

**Namespace Count:** $(cat "$PROFILE_DIR/pebble-namespace-count.txt" 2>/dev/null || echo "0")

## Load Test Results

$(cat "$PROFILE_DIR/load-test-results.json" 2>/dev/null || echo "No results file")
EOF

echo "Profile complete! See $PROFILE_DIR/README.md for details"
