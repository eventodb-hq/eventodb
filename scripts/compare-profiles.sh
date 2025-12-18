#!/bin/bash
# Compare two profile directories

if [ $# -ne 2 ]; then
  echo "Usage: $0 <profile_dir_1> <profile_dir_2>"
  echo "Example: $0 ./profiles/20251218_225728-postgres ./profiles/20251218_225959-postgres"
  exit 1
fi

DIR1="$1"
DIR2="$2"

echo "=== Profile Comparison ==="
echo ""
echo "Profile 1: $DIR1"
echo "Profile 2: $DIR2"
echo ""

# Compare load test results
if [ -f "$DIR1/load-test-results.json" ] && [ -f "$DIR2/load-test-results.json" ]; then
  echo "=== Load Test Results ==="
  echo ""
  echo "Profile 1:"
  cat "$DIR1/load-test-results.json"
  echo ""
  echo "Profile 2:"
  cat "$DIR2/load-test-results.json"
  echo ""
fi

# Compare top allocations
echo "=== Top Allocations Comparison ==="
echo ""
echo "Profile 1 - Top 15 allocations:"
go tool pprof -top -alloc_space "$DIR1/allocs-after.prof" 2>/dev/null | head -20
echo ""
echo "Profile 2 - Top 15 allocations:"
go tool pprof -top -alloc_space "$DIR2/allocs-after.prof" 2>/dev/null | head -20
echo ""

# Get total allocations
echo "=== Total Allocations ==="
TOTAL1=$(go tool pprof -top -alloc_space "$DIR1/allocs-after.prof" 2>/dev/null | grep "^Showing" | awk '{print $7}')
TOTAL2=$(go tool pprof -top -alloc_space "$DIR2/allocs-after.prof" 2>/dev/null | grep "^Showing" | awk '{print $7}')
echo "Profile 1 Total: $TOTAL1"
echo "Profile 2 Total: $TOTAL2"
echo ""

# Compare specific functions
echo "=== Driver-specific allocations ==="
echo ""
echo "Profile 1 (lib/pq):"
go tool pprof -top -alloc_space "$DIR1/allocs-after.prof" 2>/dev/null | grep "lib/pq" || echo "  (none found)"
echo ""
echo "Profile 2 (pgx):"
go tool pprof -top -alloc_space "$DIR2/allocs-after.prof" 2>/dev/null | grep "pgx" || echo "  (none found)"
echo ""
