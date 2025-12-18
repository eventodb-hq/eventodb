#!/bin/bash
# Baseline performance profiling for Message DB
# Generates CPU, memory, and allocation profiles

set -e

PROFILE_DIR="./profiles/$(date +%Y%m%d_%H%M%S)"
mkdir -p "$PROFILE_DIR"

echo "=== Message DB Performance Profiling ==="
echo "Profile directory: $PROFILE_DIR"

# Build server with profiling enabled
echo "Building server..."
cd golang
go build -o ../dist/messagedb-profile ./cmd/messagedb
cd ..

# Start server with pprof enabled
echo "Starting server..."
./dist/messagedb-profile \
  --test-mode \
  --port 8080 \
  --log-level warn &

SERVER_PID=$!
echo "Server PID: $SERVER_PID"

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
    kill $SERVER_PID 2>/dev/null || true
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

# Stop server gracefully
echo "Stopping server..."
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null || true

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
