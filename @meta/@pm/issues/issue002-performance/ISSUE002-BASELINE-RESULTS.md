# ISSUE002 - Baseline Profile Results

**Date**: 2024-12-18  
**Profile Location**: `profiles/20251218_211750/`  
**Status**: ✅ Profiling Infrastructure Verified

---

## System Performance Baseline

### Load Test Configuration
- **Duration**: 30 seconds
- **Workers**: 10 concurrent
- **Operations**: 90% writes, 10% reads
- **Streams**: 100 unique streams (cycling)

### Actual Performance

```json
{
  "throughput": "10,986 ops/sec",
  "avg_latency": "0.91 ms",
  "total_ops": 329,586,
  "writes": 299,619,
  "reads": 29,967,
  "errors": 0,
  "duration": "30.0 seconds"
}
```

**Key Observations**:
- ✅ Excellent throughput: ~11K ops/sec
- ✅ Low latency: sub-millisecond average
- ✅ Zero errors during 30s test
- ✅ Handled 330K total operations successfully

---

## Top Allocation Hotspots

Based on `alloc_space` profile (total allocations during test):

| Rank | Function | Allocations | % of Total | Category |
|------|----------|-------------|------------|----------|
| 1 | `encoding/json.(*decodeState).objectInterface` | 495.62 MB | 10.63% | 🔴 JSON |
| 2 | `net/textproto.readMIMEHeader` | 225.55 MB | 4.84% | HTTP |
| 3 | `encoding/json.mapEncoder.encode` | 211.52 MB | 4.54% | 🔴 JSON |
| 4 | `modernc.org/sqlite.interruptOnDone` | 178.51 MB | 3.83% | SQLite |
| 5 | `reflect.copyVal` | 167.00 MB | 3.58% | Reflection |
| 6 | `reflect.mapassign_faststr0` | 164.05 MB | 3.52% | Maps |
| 7 | `encoding/json.(*Decoder).refill` | 158.08 MB | 3.39% | 🔴 JSON |
| 8 | `api.(*RPCHandler).handleStreamWrite` | 155.03 MB | 3.33% | 🟡 RPC |
| 9 | `modernc.org/sqlite.(*conn).columnText` | 134.51 MB | 2.89% | SQLite |
| 10 | `net/http.Header.Clone` | 132.04 MB | 2.83% | HTTP |

**Total Captured**: 4,661.64 MB allocations during 30s

---

## Analysis & Validation

### ✅ Confirms Our Optimization Targets

The profile validates our high-priority focus areas:

#### 1. JSON Operations (Priority #1) 🔴 CRITICAL
**Allocations**: ~865 MB (18.5% of total)
- `json.objectInterface`: 495 MB
- `json.mapEncoder.encode`: 211 MB
- `json.Decoder.refill`: 158 MB

**Impact**: JSON is the #1 allocation source - confirming this should be our first optimization.

#### 2. String/Map Operations (Priority #3) 🟡 HIGH
**Allocations**: ~331 MB (7.1% of total)
- `reflect.copyVal`: 167 MB
- `reflect.mapassign_faststr0`: 164 MB

**Impact**: Map operations and reflection are significant - string splitting optimizations will help.

#### 3. RPC Handler (Priority #5) 🟡 HIGH
**Allocations**: 155 MB (3.3% of total)
- `handleStreamWrite` itself

**Impact**: Response map pooling will reduce this.

### Unexpected Findings

1. **HTTP Layer Overhead**: ~557 MB (12% of total)
   - `textproto.readMIMEHeader`: 225 MB
   - `http.conn.readRequest`: 118 MB
   - `http.Header.Clone`: 132 MB
   
   **Note**: This is stdlib HTTP overhead - may not be easily optimizable.

2. **SQLite Driver**: ~313 MB (6.7% of total)
   - SQLite-specific allocations in text/column handling
   
   **Note**: Test mode uses SQLite - production Postgres profile may differ.

---

## Profiling Infrastructure Validation

### ✅ Working Correctly

1. **pprof Endpoints**: All enabled and accessible
   - `/debug/pprof/heap` ✓
   - `/debug/pprof/allocs` ✓
   - `/debug/pprof/profile` ✓
   - `/debug/pprof/goroutine` ✓

2. **Load Test**: Generating realistic load
   - 10,986 ops/sec sustained
   - Mixed read/write operations
   - Realistic JSON payloads

3. **Profile Quality**: Rich data captured
   - 4.6 GB of allocations tracked
   - 134 unique call paths
   - Clear hotspot identification

---

## Next Steps

### Immediate Actions

1. **Start with JSON optimization** (Priority #1)
   - Replace `encoding/json` with `jsoniter` or `easyjson`
   - Add byte buffer pooling for JSON operations
   - Target: 30-50% reduction in JSON allocations (~400 MB → ~200-280 MB)

2. **Measure Impact**
   ```bash
   # After implementing JSON optimization
   make profile-baseline
   make profile-compare BASELINE=profiles/20251218_211750 OPTIMIZED=profiles/NEW
   ```

3. **Expected First Optimization Results**
   - Throughput: 10,986 → 13,000+ ops/sec (+18%)
   - Latency: 0.91 ms → 0.75 ms (-17%)
   - JSON allocations: 865 MB → 350-450 MB (-48% to -58%)

### Validation Checklist

Before implementing optimizations:
- [x] Baseline profile captured
- [x] Profiling infrastructure verified
- [x] Hotspots identified and prioritized
- [x] Expected improvements estimated
- [ ] Begin implementation of Priority #1

---

## Raw Profile Data

**Directory**: `profiles/20251218_211750/`

**Files Available**:
```
cpu.prof              # CPU profiling (10s during load)
heap-before.prof      # Heap snapshot before load
heap-after.prof       # Heap snapshot after load
allocs-before.prof    # Allocation profile before load
allocs-after.prof     # Allocation profile after load (PRIMARY)
goroutine.prof        # Goroutine dump
load-test-results.json # Performance metrics
```

**Analysis Commands**:
```bash
# Interactive allocation analysis
go tool pprof profiles/20251218_211750/allocs-after.prof

# Web UI (recommended)
go tool pprof -http=:9090 profiles/20251218_211750/allocs-after.prof

# CPU analysis
go tool pprof profiles/20251218_211750/cpu.prof
```

---

## Conclusion

✅ **Profiling infrastructure is fully operational**  
✅ **Baseline established: 10,986 ops/sec @ 0.91ms latency**  
✅ **Top optimization targets confirmed by real data**  
✅ **JSON operations are #1 priority (18.5% of allocations)**  

**Ready to begin optimization implementation!**

---

**Next**: See `ISSUE002-QUICKSTART.md` for optimization workflow
