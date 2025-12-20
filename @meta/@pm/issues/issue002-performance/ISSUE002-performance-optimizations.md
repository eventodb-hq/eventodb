# ISSUE002: Zero-Allocation Performance Optimizations

**Status**: Open  
**Priority**: High  
**Created**: 2024-12-18  
**Scope**: High-priority hot-path optimizations only

---

## Overview

This issue tracks zero-allocation optimizations for the Go codebase, focusing on hot paths that will have measurable impact on throughput and latency. We prioritize data-driven decisions using repeatable profiling scripts.

---

## Profiling Infrastructure

### 1. Baseline Profiling Scripts

#### Script: `scripts/profile-baseline.sh`
```bash
#!/bin/bash
# Baseline performance profiling for EventoDB
# Generates CPU, memory, and allocation profiles

set -e

PROFILE_DIR="./profiles/$(date +%Y%m%d_%H%M%S)"
mkdir -p "$PROFILE_DIR"

echo "=== EventoDB Performance Profiling ==="
echo "Profile directory: $PROFILE_DIR"

# Start server with profiling enabled
export LOG_LEVEL=warn
go build -o ./dist/eventodb-profile ./golang/cmd/eventodb

# Run with pprof enabled
./dist/eventodb-profile \
  --test-mode \
  --port 8080 \
  --log-level warn &

SERVER_PID=$!
sleep 2

# Wait for server to be ready
until curl -s http://localhost:8080/health > /dev/null 2>&1; do
  sleep 0.5
done

echo "Server started (PID: $SERVER_PID)"

# Capture baseline profiles BEFORE load
echo "Capturing pre-load baseline..."
curl -s http://localhost:8080/debug/pprof/heap > "$PROFILE_DIR/heap-before.prof"
curl -s http://localhost:8080/debug/pprof/allocs > "$PROFILE_DIR/allocs-before.prof"

# Run load test
echo "Running load test..."
go run ./scripts/load-test.go \
  --duration 30s \
  --workers 10 \
  --profile-dir "$PROFILE_DIR"

# Capture profiles DURING load (in background)
sleep 15 # Mid-point of load test
curl -s http://localhost:8080/debug/pprof/profile?seconds=10 > "$PROFILE_DIR/cpu.prof" &
PROFILE_PID=$!

# Wait for load test to complete
wait

# Capture post-load profiles
echo "Capturing post-load profiles..."
curl -s http://localhost:8080/debug/pprof/heap > "$PROFILE_DIR/heap-after.prof"
curl -s http://localhost:8080/debug/pprof/allocs > "$PROFILE_DIR/allocs-after.prof"
curl -s http://localhost:8080/debug/pprof/goroutine > "$PROFILE_DIR/goroutine.prof"

# Stop server
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null || true

echo "=== Profile Analysis ==="
echo ""
echo "Top 10 allocation sources:"
go tool pprof -top -alloc_space "$PROFILE_DIR/allocs-after.prof" | head -20

echo ""
echo "=== Profiles saved to: $PROFILE_DIR ==="
echo ""
echo "Analysis commands:"
echo "  CPU:         go tool pprof $PROFILE_DIR/cpu.prof"
echo "  Allocations: go tool pprof -alloc_space $PROFILE_DIR/allocs-after.prof"
echo "  Heap:        go tool pprof -inuse_space $PROFILE_DIR/heap-after.prof"
echo ""
echo "Web UI:        go tool pprof -http=:9090 $PROFILE_DIR/allocs-after.prof"
```

#### Script: `scripts/load-test.go`
```go
// Load testing tool for EventoDB profiling
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	duration   = flag.Duration("duration", 30*time.Second, "Test duration")
	workers    = flag.Int("workers", 10, "Number of concurrent workers")
	baseURL    = flag.String("url", "http://localhost:8080", "Server URL")
	profileDir = flag.String("profile-dir", "", "Profile output directory")
)

type Stats struct {
	writes      atomic.Int64
	reads       atomic.Int64
	errors      atomic.Int64
	totalLatency atomic.Int64 // microseconds
}

func main() {
	flag.Parse()

	stats := &Stats{}
	ctx := make(chan struct{})
	var wg sync.WaitGroup

	// Get default namespace token
	token := getDefaultToken()

	// Start workers
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go worker(ctx, stats, token, &wg)
	}

	// Run for duration
	time.Sleep(*duration)
	close(ctx)
	wg.Wait()

	// Print results
	printStats(stats, *duration)

	// Save results if profile dir specified
	if *profileDir != "" {
		saveStats(stats, *duration, *profileDir)
	}
}

func worker(ctx chan struct{}, stats *Stats, token string, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{Timeout: 5 * time.Second}
	streamID := 0

	for {
		select {
		case <-ctx:
			return
		default:
			// Write message
			start := time.Now()
			if err := writeMessage(client, token, fmt.Sprintf("test-stream-%d", streamID%100)); err != nil {
				stats.errors.Add(1)
			} else {
				stats.writes.Add(1)
				stats.totalLatency.Add(time.Since(start).Microseconds())
			}

			// Read messages (every 10 writes)
			if streamID%10 == 0 {
				start := time.Now()
				if err := readMessages(client, token, fmt.Sprintf("test-stream-%d", streamID%100)); err != nil {
					stats.errors.Add(1)
				} else {
					stats.reads.Add(1)
					stats.totalLatency.Add(time.Since(start).Microseconds())
				}
			}

			streamID++
		}
	}
}

func writeMessage(client *http.Client, token, stream string) error {
	payload := []interface{}{
		"stream.write",
		stream,
		map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{
				"counter": time.Now().Unix(),
				"payload": "test data for profiling",
			},
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", *baseURL+"/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func readMessages(client *http.Client, token, stream string) error {
	payload := []interface{}{
		"stream.get",
		stream,
		map[string]interface{}{
			"position":  0,
			"batchSize": 10,
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", *baseURL+"/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func getDefaultToken() string {
	// For test mode, generate deterministic token
	return "ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"
}

func printStats(stats *Stats, duration time.Duration) {
	writes := stats.writes.Load()
	reads := stats.reads.Load()
	errors := stats.errors.Load()
	totalOps := writes + reads
	totalLatency := stats.totalLatency.Load()

	fmt.Printf("\n=== Load Test Results ===\n")
	fmt.Printf("Duration:      %v\n", duration)
	fmt.Printf("Total Ops:     %d\n", totalOps)
	fmt.Printf("  Writes:      %d\n", writes)
	fmt.Printf("  Reads:       %d\n", reads)
	fmt.Printf("  Errors:      %d\n", errors)
	fmt.Printf("Throughput:    %.0f ops/sec\n", float64(totalOps)/duration.Seconds())
	if totalOps > 0 {
		fmt.Printf("Avg Latency:   %.2f ms\n", float64(totalLatency)/float64(totalOps)/1000.0)
	}
	fmt.Printf("\n")
}

func saveStats(stats *Stats, duration time.Duration, dir string) {
	writes := stats.writes.Load()
	reads := stats.reads.Load()
	errors := stats.errors.Load()
	totalOps := writes + reads
	totalLatency := stats.totalLatency.Load()

	results := map[string]interface{}{
		"timestamp":   time.Now().Format(time.RFC3339),
		"duration_s":  duration.Seconds(),
		"total_ops":   totalOps,
		"writes":      writes,
		"reads":       reads,
		"errors":      errors,
		"ops_per_sec": float64(totalOps) / duration.Seconds(),
	}

	if totalOps > 0 {
		results["avg_latency_ms"] = float64(totalLatency) / float64(totalOps) / 1000.0
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(dir+"/load-test-results.json", data, 0644)
}
```

#### Script: `scripts/compare-profiles.sh`
```bash
#!/bin/bash
# Compare two profile runs to measure optimization impact

if [ $# -ne 2 ]; then
  echo "Usage: $0 <baseline-profile-dir> <optimized-profile-dir>"
  exit 1
fi

BASELINE=$1
OPTIMIZED=$2

echo "=== Profile Comparison ==="
echo "Baseline:  $BASELINE"
echo "Optimized: $OPTIMIZED"
echo ""

# Compare load test results
echo "=== Throughput Comparison ==="
if [ -f "$BASELINE/load-test-results.json" ] && [ -f "$OPTIMIZED/load-test-results.json" ]; then
  BASELINE_OPS=$(jq -r '.ops_per_sec' "$BASELINE/load-test-results.json")
  OPTIMIZED_OPS=$(jq -r '.ops_per_sec' "$OPTIMIZED/load-test-results.json")
  IMPROVEMENT=$(echo "scale=2; ($OPTIMIZED_OPS - $BASELINE_OPS) / $BASELINE_OPS * 100" | bc)
  
  echo "Baseline:  ${BASELINE_OPS} ops/sec"
  echo "Optimized: ${OPTIMIZED_OPS} ops/sec"
  echo "Change:    ${IMPROVEMENT}%"
  echo ""
fi

# Compare allocation profiles
echo "=== Allocation Comparison ==="
echo ""
echo "Baseline top allocations:"
go tool pprof -top -alloc_space "$BASELINE/allocs-after.prof" | head -15

echo ""
echo "Optimized top allocations:"
go tool pprof -top -alloc_space "$OPTIMIZED/allocs-after.prof" | head -15

echo ""
echo "=== Detailed Comparison ==="
echo "Run: go tool pprof -base=$BASELINE/allocs-after.prof $OPTIMIZED/allocs-after.prof"
go tool pprof -top -alloc_space -base="$BASELINE/allocs-after.prof" "$OPTIMIZED/allocs-after.prof" | head -20
```

#### Script: `Makefile` additions
```makefile
# Add to existing Makefile

.PHONY: profile-baseline profile-compare benchmark-all

profile-baseline:
	@echo "Running baseline performance profile..."
	@chmod +x scripts/profile-baseline.sh
	@./scripts/profile-baseline.sh

profile-compare:
	@echo "Comparing profiles..."
	@chmod +x scripts/compare-profiles.sh
	@./scripts/compare-profiles.sh $(BASELINE) $(OPTIMIZED)

benchmark-all:
	@echo "Running all Go benchmarks..."
	cd golang && go test -bench=. -benchmem -benchtime=5s ./...
```

---

## High-Priority Optimization Targets

### 1. JSON Marshal/Unmarshal in Database Layer

**Impact**: 🔴 CRITICAL - Hot path on every read/write operation

**Files**:
- `golang/internal/store/timescale/write.go` (lines 30-38, 43-50)
- `golang/internal/store/timescale/read.go` (lines 123-135)
- `golang/internal/store/postgres/write.go` (lines 30-38, 43-50)
- `golang/internal/store/postgres/read.go` (lines 123-135)
- `golang/internal/store/sqlite/write.go` (lines 50-58)
- `golang/internal/store/sqlite/read.go` (lines 110-118)

**Current Code**:
```go
// Write path - allocates on every marshal
dataJSON, err := json.Marshal(msg.Data)
metadataJSON, err := json.Marshal(msg.Metadata)

// Read path - allocates on every unmarshal
var data map[string]interface{}
json.Unmarshal(dataJSON, &data)
```

**Optimization Strategy**:
1. Use `easyjson` for codegen (eliminates reflection)
2. Use `sync.Pool` for reusable byte buffers
3. Consider `jsoniter` as drop-in replacement

**Expected Impact**: 30-50% reduction in allocations, 20-30% throughput improvement

**Benchmark**:
```go
// Add to golang/internal/store/benchmark_json_test.go
func BenchmarkJSONMarshal(b *testing.B) {
	data := map[string]interface{}{
		"field1": "value1",
		"field2": 123,
		"field3": true,
	}
	
	b.Run("stdlib", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = json.Marshal(data)
		}
	})
	
	b.Run("jsoniter", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = jsoniter.Marshal(data)
		}
	})
}
```

---

### 2. Message Slice Pre-allocation in Read Operations

**Impact**: 🟡 HIGH - Reduces GC pressure on batch reads

**Files**:
- `golang/internal/store/timescale/read.go:106`
- `golang/internal/store/postgres/read.go:106`
- `golang/internal/store/sqlite/read.go:86`

**Current Code**:
```go
messages := []*store.Message{}  // No capacity hint
for rows.Next() {
    // ...
    messages = append(messages, msg)  // May trigger reallocation
}
```

**Optimization Strategy**:
```go
// Pre-allocate with capacity hint from BatchSize
capacity := int(batchSize)
if capacity <= 0 || capacity > 10000 {
    capacity = 1000 // reasonable default
}
messages := make([]*store.Message, 0, capacity)
```

**Expected Impact**: 10-15% reduction in allocations for batch operations

---

### 3. String Splitting in Utility Functions ✅ COMPLETED

**Impact**: 🟡 HIGH - Called frequently in category/consumer group operations

**Files**:
- `golang/internal/store/utils.go` (Category, ID, CardinalID functions)

**Optimization Applied**:
```go
// BEFORE: Allocates slice
func Category(streamName string) string {
    parts := strings.SplitN(streamName, "-", 2)
    return parts[0]
}

// AFTER: Zero allocations
func Category(streamName string) string {
    if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
        return streamName[:idx]  // Substring shares backing array
    }
    return streamName
}
```

**Actual Results**: 
- **7.3x faster** (25ns → 3ns)
- **100% allocation reduction** (1 alloc → 0 allocs)
- **CardinalID**: 5.6x faster, 2 allocs → 0 allocs
- System-wide impact: ~1-2% reduction

**Key Insight**: `strings.IndexByte()` + substring slicing = zero allocations!

---

### 4. Poke Object Pooling in SSE ✅ COMPLETED

**Impact**: 🟡 HIGH - Frequent allocations in SSE streaming

**Files**:
- `golang/internal/api/sse.go` (4 locations updated)

**Optimization Applied**:
```go
// Global pool for Poke objects
var pokePool = sync.Pool{
    New: func() interface{} {
        return &Poke{}
    },
}

// BEFORE: Direct allocation
poke := Poke{
    Stream:         streamName,
    Position:       msg.Position,
    GlobalPosition: msg.GlobalPosition,
}
if err := h.sendPoke(w, poke); err != nil {
    return
}

// AFTER: Pool reuse
poke := pokePool.Get().(*Poke)
poke.Stream = streamName
poke.Position = msg.Position
poke.GlobalPosition = msg.GlobalPosition

err := h.sendPoke(w, poke)
pokePool.Put(poke)  // Return immediately

if err != nil {
    return
}
```

**Actual Results**:
- **10% faster** (153ns → 141ns)
- **50% allocation reduction** (2 allocs → 1 alloc)
- **33% memory reduction** (96 bytes → 64 bytes)
- System-wide impact: ~2.5% baseline, 5-10% under high SSE load

**Key Insight**: sync.Pool perfect for short-lived, frequently-created objects!

---

### 5. Response Map Allocations in RPC Handlers

**Impact**: 🟡 HIGH - Every RPC response allocates a map

**Files**:
- `golang/internal/api/handlers.go` (multiple locations)

**Current Code**:
```go
return map[string]interface{}{
    "position":       result.Position,
    "globalPosition": result.GlobalPosition,
}, nil
```

**Optimization Strategy**:
```go
// Define typed response structs
type WriteResponse struct {
    Position       int64 `json:"position"`
    GlobalPosition int64 `json:"globalPosition"`
}

var writeRespPool = sync.Pool{
    New: func() interface{} {
        return &WriteResponse{}
    },
}

// In handler:
resp := writeRespPool.Get().(*WriteResponse)
defer writeRespPool.Put(resp)
resp.Position = result.Position
resp.GlobalPosition = result.GlobalPosition
return resp, nil
```

**Expected Impact**: 10-20% reduction in RPC handler allocations

---

## Success Metrics

Each optimization must demonstrate:

1. **Allocation Reduction**: Measured via `go test -benchmem`
   - Target: 20%+ reduction in allocs/op for targeted code

2. **Throughput Improvement**: Measured via load test
   - Target: 10%+ improvement in ops/sec

3. **Latency Improvement**: Measured via load test
   - Target: 5%+ reduction in avg latency

4. **No Regression**: CPU usage and correctness maintained
   - All tests pass
   - CPU profile shows no new hotspots

---

## Implementation Checklist

- [x] Set up profiling scripts (scripts directory)
- [x] Run baseline profile and save results
- [x] Create benchmarks for each optimization
- [x] **Implement optimization #1: JSON Marshal/Unmarshal** ✅ COMPLETED 2024-12-18
  - [x] Benchmark before/after - 7.36% throughput improvement
  - [x] Profile before/after - 30% JSON allocation reduction
  - [x] Verify correctness (all tests pass) - ✅ 100% pass rate
  - [x] **Results**: See `optimization-001-jsoniter-results.md`
- [x] **Implement optimization #2: Message slice pre-allocation** ✅ COMPLETED 2024-12-18
  - [x] Benchmark before/after - 1.15% throughput improvement (avg of 3 runs)
  - [x] Profile before/after - 12.1% reduction in scanMessages allocations, 4.2% system-wide
  - [x] Verify correctness (all tests pass) - ✅ 100% pass rate
  - [x] **Results**: See `optimization-002-slice-prealloc-results.md`
  - [x] **Key Lesson**: ALWAYS run multiple iterations! Single runs misleading due to variance
- [x] **Implement optimization #3: String splitting** ✅ COMPLETED 2024-12-18
  - [x] Benchmark before/after - 7.3x local speedup, 100% allocation reduction
  - [x] Profile impact - 1-2% system-wide reduction (estimated)
  - [x] Verify correctness (all tests pass) - ✅ 100% pass rate
  - [x] **Results**: See `optimization-003-string-splitting-results.md`
  - [x] **Key Lesson**: Best effort-to-reward ratio - simple change, massive local improvement
- [x] **Implement optimization #4: Poke object pooling** ✅ COMPLETED 2024-12-18
  - [x] Benchmark before/after - 10% faster, 50% allocation reduction
  - [x] Profile impact - 2.5% baseline (5-10% under high SSE load)
  - [x] Verify correctness (all tests pass) - ✅ 100% pass rate
  - [x] **Results**: See `optimization-004-poke-pooling-results.md`
  - [x] **Key Lesson**: sync.Pool perfect for transient objects - GC benefits compound over time
- [ ] Implement optimization #5: Response map pooling
- [ ] Run final comparison profile
- [ ] Document results and learnings

---

## Notes

- Focus on **measurable** improvements only
- Each optimization is independent and can be tested separately
- Use profiling to validate assumptions before and after
- Document any trade-offs (e.g., code complexity vs performance)

---

## References

- Go profiling guide: https://go.dev/blog/pprof
- sync.Pool best practices: https://pkg.go.dev/sync#Pool
- easyjson: https://github.com/mailru/easyjson
- jsoniter: https://github.com/json-iterator/go
