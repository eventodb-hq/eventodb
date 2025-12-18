#!/bin/bash
# Compare two profile runs to measure optimization impact

set -e

if [ $# -ne 2 ]; then
  echo "Usage: $0 <baseline-profile-dir> <optimized-profile-dir>"
  echo ""
  echo "Example:"
  echo "  $0 profiles/20241218_120000 profiles/20241218_130000"
  exit 1
fi

BASELINE=$1
OPTIMIZED=$2

if [ ! -d "$BASELINE" ]; then
  echo "Error: Baseline directory not found: $BASELINE"
  exit 1
fi

if [ ! -d "$OPTIMIZED" ]; then
  echo "Error: Optimized directory not found: $OPTIMIZED"
  exit 1
fi

echo "╔════════════════════════════════════════════════════════════╗"
echo "║         Profile Comparison Report                          ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""
echo "Baseline:  $BASELINE"
echo "Optimized: $OPTIMIZED"
echo ""

# Compare load test results
echo "═══════════════════════════════════════════════════════════════"
echo "  THROUGHPUT COMPARISON"
echo "═══════════════════════════════════════════════════════════════"
if [ -f "$BASELINE/load-test-results.json" ] && [ -f "$OPTIMIZED/load-test-results.json" ]; then
  BASELINE_OPS=$(jq -r '.ops_per_sec' "$BASELINE/load-test-results.json")
  OPTIMIZED_OPS=$(jq -r '.ops_per_sec' "$OPTIMIZED/load-test-results.json")
  BASELINE_LAT=$(jq -r '.avg_latency_ms' "$BASELINE/load-test-results.json")
  OPTIMIZED_LAT=$(jq -r '.avg_latency_ms' "$OPTIMIZED/load-test-results.json")
  
  # Calculate improvements
  OPS_IMPROVEMENT=$(echo "scale=2; ($OPTIMIZED_OPS - $BASELINE_OPS) / $BASELINE_OPS * 100" | bc)
  LAT_IMPROVEMENT=$(echo "scale=2; ($BASELINE_LAT - $OPTIMIZED_LAT) / $BASELINE_LAT * 100" | bc)
  
  echo ""
  echo "Throughput:"
  echo "  Baseline:    ${BASELINE_OPS} ops/sec"
  echo "  Optimized:   ${OPTIMIZED_OPS} ops/sec"
  echo "  Change:      ${OPS_IMPROVEMENT}%"
  echo ""
  echo "Latency:"
  echo "  Baseline:    ${BASELINE_LAT} ms"
  echo "  Optimized:   ${OPTIMIZED_LAT} ms"
  echo "  Change:      ${LAT_IMPROVEMENT}%"
  echo ""
else
  echo "⚠️  Load test results not found in one or both directories"
  echo ""
fi

# Compare allocation profiles
echo "═══════════════════════════════════════════════════════════════"
echo "  ALLOCATION ANALYSIS"
echo "═══════════════════════════════════════════════════════════════"
echo ""

if [ -f "$BASELINE/allocs-after.prof" ] && [ -f "$OPTIMIZED/allocs-after.prof" ]; then
  echo "Top allocation changes (negative = improvement):"
  echo ""
  go tool pprof -top -alloc_space -base="$BASELINE/allocs-after.prof" "$OPTIMIZED/allocs-after.prof" 2>/dev/null | head -20
  echo ""
  
  echo "─────────────────────────────────────────────────────────────"
  echo "Baseline top allocations:"
  echo ""
  go tool pprof -top -alloc_space "$BASELINE/allocs-after.prof" 2>/dev/null | head -15
  
  echo ""
  echo "─────────────────────────────────────────────────────────────"
  echo "Optimized top allocations:"
  echo ""
  go tool pprof -top -alloc_space "$OPTIMIZED/allocs-after.prof" 2>/dev/null | head -15
else
  echo "⚠️  Allocation profiles not found in one or both directories"
fi

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  DETAILED ANALYSIS COMMANDS"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "Interactive comparison:"
echo "  go tool pprof -base=$BASELINE/allocs-after.prof $OPTIMIZED/allocs-after.prof"
echo ""
echo "Web UI comparison:"
echo "  go tool pprof -http=:9090 -base=$BASELINE/allocs-after.prof $OPTIMIZED/allocs-after.prof"
echo ""
echo "CPU profile comparison:"
echo "  go tool pprof -base=$BASELINE/cpu.prof $OPTIMIZED/cpu.prof"
echo ""

# Generate summary report
REPORT_FILE="profile-comparison-$(date +%Y%m%d_%H%M%S).txt"
{
  echo "Profile Comparison Report"
  echo "Generated: $(date)"
  echo ""
  echo "Baseline:  $BASELINE"
  echo "Optimized: $OPTIMIZED"
  echo ""
  if [ -f "$BASELINE/load-test-results.json" ] && [ -f "$OPTIMIZED/load-test-results.json" ]; then
    echo "Performance Metrics:"
    echo "  Throughput: ${BASELINE_OPS} -> ${OPTIMIZED_OPS} ops/sec (${OPS_IMPROVEMENT}%)"
    echo "  Latency:    ${BASELINE_LAT} -> ${OPTIMIZED_LAT} ms (${LAT_IMPROVEMENT}%)"
  fi
} > "$REPORT_FILE"

echo "Summary saved to: $REPORT_FILE"
echo ""
