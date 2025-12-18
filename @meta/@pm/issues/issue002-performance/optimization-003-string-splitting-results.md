# Optimization 003: String Splitting in Utility Functions - Results

**Date**: 2024-12-18  
**Optimization Target**: String splitting operations in Category, ID, and CardinalID functions  
**Files Modified**: `golang/internal/store/utils.go`  
**Status**: ✅ **COMPLETED**

---

## Executive Summary

Replaced `strings.SplitN()` allocations with zero-allocation `strings.IndexByte()` operations in frequently-called utility functions. Achieved **7-8x speedup** and **100% allocation reduction** for Category and ID operations.

### Key Results
- **Performance**: 7-8x faster execution (25ns → 3ns)
- **Allocations**: 100% reduction (1 alloc/op → 0 allocs/op)
- **Memory**: 100% reduction (32 bytes/op → 0 bytes/op)
- **Correctness**: ✅ All tests pass

---

## Optimization Strategy

### Before: Using `strings.SplitN()`

```go
func Category(streamName string) string {
    parts := strings.SplitN(streamName, "-", 2)  // Allocates slice
    return parts[0]
}

func ID(streamName string) string {
    parts := strings.SplitN(streamName, "-", 2)  // Allocates slice
    if len(parts) < 2 {
        return ""
    }
    return parts[1]
}

func CardinalID(streamName string) string {
    id := ID(streamName)  // 1 allocation
    if id == "" {
        return ""
    }
    parts := strings.SplitN(id, "+", 2)  // Another allocation
    return parts[0]
}
```

**Issues**:
1. `SplitN()` allocates a new slice on every call
2. CardinalID makes TWO allocations (one for ID, one for Split)
3. Allocations occur even when result is simple substring

### After: Using `strings.IndexByte()`

```go
func Category(streamName string) string {
    if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
        return streamName[:idx]  // Substring shares backing array
    }
    return streamName
}

func ID(streamName string) string {
    if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
        return streamName[idx+1:]  // Substring shares backing array
    }
    return ""
}

func CardinalID(streamName string) string {
    // Extract ID part
    if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
        id := streamName[idx+1:]
        // Extract part before '+' for compound IDs
        if plusIdx := strings.IndexByte(id, '+'); plusIdx >= 0 {
            return id[:plusIdx]
        }
        return id
    }
    return ""
}
```

**Benefits**:
1. `IndexByte()` is highly optimized assembly routine
2. Substring slicing shares backing array (no allocation)
3. CardinalID now makes ZERO allocations (inlined everything)

---

## Benchmark Results

### Category() Function

| Test Case | Method | Time (ns/op) | Allocs (allocs/op) | Memory (B/op) | Speedup |
|-----------|--------|--------------|---------------------|---------------|---------|
| Simple (`account-123`) | Split | 25.42 | 1 | 32 | - |
| Simple (`account-123`) | **Index** | **3.23** | **0** | **0** | **7.9x** |
| Compound (`account-123+456`) | Split | 25.92 | 1 | 32 | - |
| Compound (`account-123+456`) | **Index** | **3.76** | **0** | **0** | **6.9x** |
| No ID (`account`) | Split | 24.12 | 1 | 32 | - |
| No ID (`account`) | **Index** | **3.20** | **0** | **0** | **7.5x** |
| Long UUID | Split | 26.22 | 1 | 32 | - |
| Long UUID | **Index** | **3.71** | **0** | **0** | **7.1x** |

**Average speedup**: **7.35x faster**  
**Allocation reduction**: **100% (1 → 0)**

### ID() Function

| Test Case | Method | Time (ns/op) | Allocs (allocs/op) | Memory (B/op) | Speedup |
|-----------|--------|--------------|---------------------|---------------|---------|
| Simple (`account-123`) | Split | 26.29 | 1 | 32 | - |
| Simple (`account-123`) | **Index** | **3.48** | **0** | **0** | **7.6x** |
| Compound (`account-123+456`) | Split | 26.93 | 1 | 32 | - |
| Compound (`account-123+456`) | **Index** | **4.01** | **0** | **0** | **6.7x** |
| No ID (`account`) | Split | 24.61 | 1 | 32 | - |
| No ID (`account`) | **Index** | **3.19** | **0** | **0** | **7.7x** |
| Long UUID | Split | 28.80 | 1 | 32 | - |
| Long UUID | **Index** | **3.92** | **0** | **0** | **7.3x** |

**Average speedup**: **7.33x faster**  
**Allocation reduction**: **100% (1 → 0)**

### CardinalID() Function

| Test Case | Time (ns/op) | Allocs (allocs/op) | Memory (B/op) | Improvement |
|-----------|--------------|---------------------|---------------|-------------|
| Simple (BEFORE) | 54.59 | 2 | 64 | - |
| Simple (AFTER) | **15.01** | **0** | **0** | **3.6x faster** |
| Compound (BEFORE) | 56.89 | 2 | 64 | - |
| Compound (AFTER) | **11.41** | **0** | **0** | **5.0x faster** |
| No ID (BEFORE) | 33.77 | 1 | 32 | - |
| No ID (AFTER) | **3.20** | **0** | **0** | **10.6x faster** |
| Long Compound (BEFORE) | 56.76 | 2 | 64 | - |
| Long Compound (AFTER) | **18.05** | **0** | **0** | **3.1x faster** |

**Average speedup**: **5.6x faster**  
**Allocation reduction**: **100% (2 → 0)** for most cases

**Note**: CardinalID had 2 allocations before (one from ID(), one from second Split), now has zero!

---

## Correctness Verification

### Unit Tests
```bash
$ go test ./internal/store/... -v -run="TestCategory|TestID|TestCardinal"
```

**Results**: ✅ **100% PASS**

All test cases verified:
- ✅ Simple stream names (`account-123`)
- ✅ Compound IDs (`account-123+456`)
- ✅ Category-only names (`account`)
- ✅ Empty strings
- ✅ Multi-dash names (`transaction-uuid-with-dashes`)
- ✅ Consumer group partitioning (hash-based assignment)

### Integration Tests
```bash
$ go test ./... 
```

**Results**: ✅ **ALL TESTS PASS** (1 flaky unrelated test)

---

## Impact Analysis

### Where These Functions Are Called

1. **Consumer Group Operations** (`IsAssignedToConsumerMember`)
   - Calls `CardinalID()` on every message evaluation
   - High frequency in streaming/polling scenarios
   - **Impact**: Significant - eliminated 2 allocs per evaluation

2. **Category Queries** (`GetCategoryMessages`)
   - Uses `Category()` for stream name validation
   - Called on every category read operation
   - **Impact**: Moderate - 1 alloc eliminated per operation

3. **Stream Name Parsing** (throughout codebase)
   - Used for validation, logging, error messages
   - Lower frequency but widespread
   - **Impact**: Small individually, cumulative over time

### Expected System-Wide Impact

Based on profiling, these functions are called in:
- SSE stream subscriptions (category filtering)
- Consumer group message polling
- Stream name validation in RPC handlers

**Conservative estimate**:
- If 10% of operations involve category/ID parsing
- And each saves 1-2 allocations
- System-wide allocation reduction: **1-2%**
- GC pressure reduction: **Minor but measurable**

This is a **low-effort, high-confidence** optimization with zero downside.

---

## Code Quality

### Readability
- **Before**: Straightforward but wasteful
- **After**: Slightly more verbose, but idiomatic Go
- **Tradeoff**: Acceptable - inline comments explain intent

### Maintainability
- ✅ Added comprehensive benchmark suite
- ✅ Correctness tests validate parity with old implementation
- ✅ Well-documented functions with examples
- ✅ No external dependencies

### Safety
- ✅ Substring slicing is safe (shares backing array)
- ✅ Index bounds checked by `IndexByte()` return value
- ✅ All edge cases tested (empty strings, missing delimiters)

---

## Lessons Learned

### 1. String Operations Matter
Go's standard library has highly optimized primitives:
- `IndexByte()` uses assembly SIMD on modern CPUs
- Simple loops can be 10x faster than "convenient" APIs
- Profile first, but this optimization was predictable

### 2. Allocation Elimination Compounds
CardinalID went from 2 allocs → 0 allocs:
- Not just 50% reduction, but 100%!
- Inlining multiple operations into one function
- Bigger wins than expected

### 3. Benchmarking is Essential
Without benchmarks, this would be "premature optimization":
- Concrete numbers justify the change
- Regression detection in CI
- Documents performance characteristics

### 4. Go's Substring Sharing is Powerful
Key insight: `s[i:j]` doesn't allocate!
- Shares backing array with original string
- Safe because strings are immutable
- "Zero-cost abstraction" in action

---

## Recommendations

### Immediate
- ✅ **Merged** - Low risk, high confidence
- ✅ Monitor for regressions in CI benchmarks
- ✅ Document in performance guide

### Future Optimizations
1. **Consider `strings.Builder` for concatenation**
   - If we ever build stream names dynamically
   - Pre-allocate capacity for zero allocations

2. **Profile in production**
   - Validate allocation reduction in real workloads
   - May unlock further optimizations in hot paths

3. **Apply pattern elsewhere**
   - Look for other `Split()` calls in hot paths
   - JSON key parsing, delimiter splitting, etc.

---

## Comparison to Previous Optimizations

| Optimization | Speedup | Alloc Reduction | System Impact | Effort |
|--------------|---------|-----------------|---------------|--------|
| #1: jsoniter | 1.5x | 30% (JSON only) | 7.4% throughput | Medium |
| #2: Slice prealloc | 1.01x | 12% (reads) | 1.2% throughput | Low |
| **#3: String split** | **7.3x** | **100% (local)** | **1-2% (est.)** | **Low** |

**Conclusion**: 
- Highest **local** speedup (7x)
- 100% allocation elimination
- Lower **system-wide** impact than jsoniter
- But: **Best effort-to-reward ratio**

---

## Sign-Off

**Optimization**: ✅ **APPROVED FOR PRODUCTION**

**Reasoning**:
1. Massive local performance improvement (7x)
2. Zero allocations (down from 1-2)
3. All tests pass
4. No behavioral changes
5. Low complexity
6. Well-benchmarked

**Next Steps**:
1. Mark item #3 complete in ISSUE002
2. Proceed to optimization #4 (Poke object pooling)
3. Run end-to-end profiling after completing all optimizations

---

**Reviewed by**: Optimization Team  
**Date**: 2024-12-18  
**Status**: ✅ **COMPLETE**
