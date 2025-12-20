#!/bin/bash
# Compare performance between SQLite and Pebble
# Generates side-by-side profiles and metrics


# Disable CGO for consistent builds across platforms
export CGO_ENABLED=0

set -e

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
COMPARE_DIR="./profiles/${TIMESTAMP}-comparison"
mkdir -p "$COMPARE_DIR"

echo "=== Message DB Performance Comparison ==="
echo "Comparing: SQLite vs Pebble"
echo "Results directory: $COMPARE_DIR"
echo ""

# Run SQLite profiling
echo "========================================"
echo "Phase 1/2: Profiling SQLite..."
echo "========================================"
SQLITE_PROFILE_DIR="${COMPARE_DIR}/sqlite"
mkdir -p "$SQLITE_PROFILE_DIR"

# SQLite profiling (inline to avoid separate script execution)
PEBBLE_DATA_DIR="/tmp/eventodb-sqlite-profile"
rm -rf "$PEBBLE_DATA_DIR"

cd golang
timeout 60s go build -o ../dist/eventodb-profile ./cmd/eventodb
cd ..

PROFILE_TOKEN="ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"

./dist/eventodb-profile \
  --test-mode \
  --port 8080 \
  --log-level warn &
SQLITE_PID=$!

echo "SQLite server PID: $SQLITE_PID"

# Wait for server
sleep 2
for i in {1..30}; do
  if timeout 5s curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "SQLite server ready!"
    break
  fi
  sleep 1
done

# Capture baseline
timeout 10s curl -s http://localhost:8080/debug/pprof/heap > "$SQLITE_PROFILE_DIR/heap-before.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/allocs > "$SQLITE_PROFILE_DIR/allocs-before.prof"

# Run load test
echo "Running SQLite load test..."
export DEFAULT_TOKEN="ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"
timeout 60s go run ./scripts/load-test.go \
  --duration 30s \
  --workers 10 \
  --profile-dir "$SQLITE_PROFILE_DIR" &
LOAD_PID=$!

sleep 15
timeout 20s curl -s "http://localhost:8080/debug/pprof/profile?seconds=10" > "$SQLITE_PROFILE_DIR/cpu.prof" &
wait $LOAD_PID

# Capture post-load
timeout 10s curl -s http://localhost:8080/debug/pprof/heap > "$SQLITE_PROFILE_DIR/heap-after.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/allocs > "$SQLITE_PROFILE_DIR/allocs-after.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/goroutine > "$SQLITE_PROFILE_DIR/goroutine.prof"

# Stop SQLite server
kill $SQLITE_PID 2>/dev/null || true
wait $SQLITE_PID 2>/dev/null || true

echo ""
echo "SQLite profiling complete!"
echo ""

# Small delay between tests
sleep 3

# Run Pebble profiling
echo "========================================"
echo "Phase 2/2: Profiling Pebble..."
echo "========================================"
PEBBLE_PROFILE_DIR="${COMPARE_DIR}/pebble"
mkdir -p "$PEBBLE_PROFILE_DIR"

PEBBLE_DATA_DIR="/tmp/eventodb-pebble-profile"
rm -rf "$PEBBLE_DATA_DIR"

./dist/eventodb-profile \
  --port 8080 \
  --db-url "pebble://$PEBBLE_DATA_DIR" \
  --token "$PROFILE_TOKEN" \
  --log-level warn &
PEBBLE_PID=$!

echo "Pebble server PID: $PEBBLE_PID"

# Wait for server
sleep 2
for i in {1..30}; do
  if timeout 5s curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "Pebble server ready!"
    break
  fi
  sleep 1
done

# Capture baseline
timeout 10s curl -s http://localhost:8080/debug/pprof/heap > "$PEBBLE_PROFILE_DIR/heap-before.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/allocs > "$PEBBLE_PROFILE_DIR/allocs-before.prof"

# Run load test
echo "Running Pebble load test..."
timeout 60s go run ./scripts/load-test.go \
  --duration 30s \
  --workers 10 \
  --profile-dir "$PEBBLE_PROFILE_DIR" &
LOAD_PID=$!

sleep 15
timeout 20s curl -s "http://localhost:8080/debug/pprof/profile?seconds=10" > "$PEBBLE_PROFILE_DIR/cpu.prof" &
wait $LOAD_PID

# Capture post-load
timeout 10s curl -s http://localhost:8080/debug/pprof/heap > "$PEBBLE_PROFILE_DIR/heap-after.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/allocs > "$PEBBLE_PROFILE_DIR/allocs-after.prof"
timeout 10s curl -s http://localhost:8080/debug/pprof/goroutine > "$PEBBLE_PROFILE_DIR/goroutine.prof"

# Capture Pebble metrics
if [ -d "$PEBBLE_DATA_DIR/_metadata" ]; then
  du -sh "$PEBBLE_DATA_DIR"/* > "$PEBBLE_PROFILE_DIR/pebble-disk-usage.txt" || true
fi
ls -1d "$PEBBLE_DATA_DIR"/*/ 2>/dev/null | wc -l > "$PEBBLE_PROFILE_DIR/pebble-namespace-count.txt" || echo "0" > "$PEBBLE_PROFILE_DIR/pebble-namespace-count.txt"

# Stop Pebble server
kill $PEBBLE_PID 2>/dev/null || true
wait $PEBBLE_PID 2>/dev/null || true
rm -rf "$PEBBLE_DATA_DIR"

echo ""
echo "Pebble profiling complete!"
echo ""

# Generate comparison report
echo "========================================"
echo "Generating comparison report..."
echo "========================================"

# Extract key metrics
SQLITE_RESULTS="$SQLITE_PROFILE_DIR/load-test-results.json"
PEBBLE_RESULTS="$PEBBLE_PROFILE_DIR/load-test-results.json"

# Create comparison report
cat > "$COMPARE_DIR/COMPARISON.md" << 'EOF'
# SQLite vs Pebble Performance Comparison

**Date**: $(date)
**Duration**: 30 seconds per test
**Workers**: 10

## Load Test Results

### SQLite
```json
$(cat "$SQLITE_RESULTS" 2>/dev/null || echo "No results")
```

### Pebble
```json
$(cat "$PEBBLE_RESULTS" 2>/dev/null || echo "No results")
```

## Profile Analysis

### Top Allocations - SQLite
```
$(timeout 30s go tool pprof -top -alloc_space "$SQLITE_PROFILE_DIR/allocs-after.prof" 2>&1 | head -15 || echo "Could not parse profile")
```

### Top Allocations - Pebble
```
$(timeout 30s go tool pprof -top -alloc_space "$PEBBLE_PROFILE_DIR/allocs-after.prof" 2>&1 | head -15 || echo "Could not parse profile")
```

## Pebble-Specific Metrics

**Disk Usage:**
```
$(cat "$PEBBLE_PROFILE_DIR/pebble-disk-usage.txt" 2>/dev/null || echo "No data")
```

**Namespace Count:** $(cat "$PEBBLE_PROFILE_DIR/pebble-namespace-count.txt" 2>/dev/null || echo "0")

## Analysis Commands

### SQLite
```bash
# CPU
go tool pprof sqlite/cpu.prof

# Allocations
go tool pprof -alloc_space sqlite/allocs-after.prof

# Heap
go tool pprof -inuse_space sqlite/heap-after.prof

# Web UI
go tool pprof -http=:9090 sqlite/allocs-after.prof
```

### Pebble
```bash
# CPU
go tool pprof pebble/cpu.prof

# Allocations
go tool pprof -alloc_space pebble/allocs-after.prof

# Heap
go tool pprof -inuse_space pebble/heap-after.prof

# Web UI
go tool pprof -http=:9091 pebble/allocs-after.prof
```

## Comparison

| Metric | SQLite | Pebble | Difference |
|--------|--------|--------|------------|
| Throughput | TBD | TBD | TBD |
| Avg Latency | TBD | TBD | TBD |
| P99 Latency | TBD | TBD | TBD |
| Memory Usage | TBD | TBD | TBD |

_Note: Extract metrics from JSON files above and calculate differences._
EOF

# Use eval to expand variables in the heredoc
eval "cat > \"$COMPARE_DIR/COMPARISON.md\" << 'EOF'
# SQLite vs Pebble Performance Comparison

**Date**: $(date)
**Duration**: 30 seconds per test
**Workers**: 10

## Load Test Results

### SQLite
\`\`\`json
$(cat "$SQLITE_RESULTS" 2>/dev/null || echo "No results")
\`\`\`

### Pebble
\`\`\`json
$(cat "$PEBBLE_RESULTS" 2>/dev/null || echo "No results")
\`\`\`

## Profile Analysis

### Top Allocations - SQLite
\`\`\`
$(timeout 30s go tool pprof -top -alloc_space "$SQLITE_PROFILE_DIR/allocs-after.prof" 2>&1 | head -15 || echo "Could not parse profile")
\`\`\`

### Top Allocations - Pebble
\`\`\`
$(timeout 30s go tool pprof -top -alloc_space "$PEBBLE_PROFILE_DIR/allocs-after.prof" 2>&1 | head -15 || echo "Could not parse profile")
\`\`\`

## Pebble-Specific Metrics

**Disk Usage:**
\`\`\`
$(cat "$PEBBLE_PROFILE_DIR/pebble-disk-usage.txt" 2>/dev/null || echo "No data")
\`\`\`

**Namespace Count:** $(cat "$PEBBLE_PROFILE_DIR/pebble-namespace-count.txt" 2>/dev/null || echo "0")

## Analysis Commands

### SQLite
\`\`\`bash
# CPU
go tool pprof $COMPARE_DIR/sqlite/cpu.prof

# Allocations
go tool pprof -alloc_space $COMPARE_DIR/sqlite/allocs-after.prof

# Heap
go tool pprof -inuse_space $COMPARE_DIR/sqlite/heap-after.prof

# Web UI
go tool pprof -http=:9090 $COMPARE_DIR/sqlite/allocs-after.prof
\`\`\`

### Pebble
\`\`\`bash
# CPU
go tool pprof $COMPARE_DIR/pebble/cpu.prof

# Allocations
go tool pprof -alloc_space $COMPARE_DIR/pebble/allocs-after.prof

# Heap
go tool pprof -inuse_space $COMPARE_DIR/pebble/heap-after.prof

# Web UI
go tool pprof -http=:9091 $COMPARE_DIR/pebble/allocs-after.prof
\`\`\`

## Files Structure

\`\`\`
$COMPARE_DIR/
├── COMPARISON.md (this file)
├── sqlite/
│   ├── cpu.prof
│   ├── heap-before.prof
│   ├── heap-after.prof
│   ├── allocs-before.prof
│   ├── allocs-after.prof
│   ├── goroutine.prof
│   └── load-test-results.json
└── pebble/
    ├── cpu.prof
    ├── heap-before.prof
    ├── heap-after.prof
    ├── allocs-before.prof
    ├── allocs-after.prof
    ├── goroutine.prof
    ├── pebble-disk-usage.txt
    ├── pebble-namespace-count.txt
    └── load-test-results.json
\`\`\`
EOF
"

echo ""
echo "========================================"
echo "Comparison Complete!"
echo "========================================"
echo ""
echo "Results saved to: $COMPARE_DIR"
echo ""
echo "View comparison report:"
echo "  cat $COMPARE_DIR/COMPARISON.md"
echo ""
echo "Or open in your editor:"
echo "  \$EDITOR $COMPARE_DIR/COMPARISON.md"
echo ""
