# ISSUE002 Quick Start Guide

**Zero-Allocation Performance Optimizations**

This is the TL;DR companion to ISSUE002-performance-optimizations.md

---

## 🎯 Goal

Optimize hot paths in the Go codebase with data-driven, measurable improvements.

**Scope**: HIGH priority items only (5 optimizations)

---

## 📋 Prerequisites

```bash
# Install required tools (if not already installed)
brew install graphviz  # macOS (for pprof web UI graphs)
# or: sudo apt-get install graphviz  # Linux

# Install jq for JSON parsing (comparison script)
brew install jq  # macOS
# or: sudo apt-get install jq  # Linux
```

---

## 🚀 Quick Workflow

### 1. Capture Baseline

```bash
make profile-baseline
```

**Output**: `profiles/YYYYMMDD_HHMMSS/` with all profile data

**Time**: ~1 minute

### 2. Implement Optimization

Pick one from the 5 high-priority items in ISSUE002.

### 3. Capture Optimized Profile

```bash
make profile-baseline
```

**Output**: New `profiles/YYYYMMDD_HHMMSS/` directory

### 4. Compare Results

```bash
make profile-compare \
  BASELINE=profiles/20241218_120000 \
  OPTIMIZED=profiles/20241218_130000
```

**Shows**:
- ✅ Throughput change (ops/sec)
- ✅ Latency change (ms)
- ✅ Allocation delta (by function)

### 5. Verify Success

**Must meet all criteria**:
- [ ] 20%+ allocation reduction in targeted code
- [ ] 10%+ throughput improvement (or latency reduction)
- [ ] All tests pass: `cd golang && go test ./...`
- [ ] No CPU regression (check cpu.prof)

---

## 🎯 5 High-Priority Targets

### #1: JSON Marshal/Unmarshal
**Impact**: 🔴 CRITICAL  
**Files**: All `*/{read,write}.go` in store layer  
**Strategy**: Replace stdlib with `jsoniter` or use `sync.Pool`  
**Expected**: 30-50% allocation reduction

### #2: Message Slice Pre-allocation
**Impact**: 🟡 HIGH  
**Files**: `*/read.go:106` in store layer  
**Strategy**: `make([]*Message, 0, batchSize)`  
**Expected**: 10-15% allocation reduction

### #3: String Splitting
**Impact**: 🟡 HIGH  
**Files**: `internal/store/utils.go`  
**Strategy**: Replace `strings.Split` with `strings.IndexByte`  
**Expected**: 5-10% allocation reduction

### #4: Poke Object Pooling
**Impact**: 🟡 HIGH  
**Files**: `internal/api/sse.go`  
**Strategy**: `sync.Pool` for Poke objects  
**Expected**: 15-25% allocation reduction in SSE

### #5: Response Map Pooling
**Impact**: 🟡 HIGH  
**Files**: `internal/api/handlers.go`  
**Strategy**: Typed structs + `sync.Pool`  
**Expected**: 10-20% allocation reduction in RPC

---

## 📊 Reading Profile Results

### Allocation Profile (Most Important)

```bash
go tool pprof -http=:9090 profiles/YYYYMMDD/allocs-after.prof
```

**What to look for**:
- Red boxes = hot allocation spots
- Box size = total allocations
- Arrows = call paths

### Comparison Output

```
Top allocation changes (negative = improvement):

      flat  flat%   sum%        cum   cum%
  -15.21MB 35.00% 35.00%   -15.21MB 35.00%  encoding/json.Marshal
   -8.45MB 19.47% 54.47%    -8.45MB 19.47%  strings.SplitN
   ...
```

**Negative values = GOOD** (less allocation in optimized version)

---

## 🧪 Example: Optimizing String Splitting

### Before

```go
func Category(streamName string) string {
    parts := strings.SplitN(streamName, "-", 2)  // Allocates slice
    return parts[0]
}
```

### After

```go
func Category(streamName string) string {
    if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
        return streamName[:idx]  // No allocation
    }
    return streamName
}
```

### Verify

```bash
# 1. Run baseline
make profile-baseline

# 2. Make change (edit utils.go)

# 3. Run optimized profile
make profile-baseline

# 4. Compare
make profile-compare BASELINE=profiles/20241218_120000 OPTIMIZED=profiles/20241218_130000

# Expected output:
# Throughput: 5000 -> 5200 ops/sec (+4%)
# Top changes:
#   -2.15MB  strings.SplitN  (eliminated)
```

---

## 📝 Checklist for Each Optimization

- [ ] Create feature branch: `git checkout -b perf/opt-name`
- [ ] Capture baseline: `make profile-baseline`
- [ ] Implement change
- [ ] Add/update tests
- [ ] Run tests: `cd golang && go test ./...`
- [ ] Capture optimized profile: `make profile-baseline`
- [ ] Compare: `make profile-compare BASELINE=... OPTIMIZED=...`
- [ ] Verify meets success criteria (20%+ alloc reduction, etc.)
- [ ] Document results in commit message
- [ ] Create PR with before/after profile comparison

---

## 🐛 Troubleshooting

### "Server failed to start"

```bash
# Check if port 8080 is in use
lsof -i :8080

# Kill if needed
kill -9 <PID>
```

### "Profile is empty"

Check load test ran:
```bash
cat profiles/YYYYMMDD/load-test-results.json
```

If errors > 0, check server logs.

### "Can't compare profiles"

Ensure both directories exist:
```bash
ls -la profiles/
```

Use full paths if needed.

---

## 📚 More Info

- **Detailed guide**: `@meta/@pm/issues/ISSUE002-performance-optimizations.md`
- **Script docs**: `scripts/README.md`
- **Go profiling**: https://go.dev/blog/pprof

---

## 🎓 Learning Resources

### First Time Profiling?

1. Read: https://go.dev/blog/pprof
2. Run baseline profile
3. Open web UI: `go tool pprof -http=:9090 profiles/.../allocs-after.prof`
4. Click around - it's interactive!
5. Focus on red boxes (hot allocations)

### Understanding Flame Graphs

- **Width** = time/allocations spent
- **Height** = call stack depth
- **Color** = different packages
- **Click** = zoom into that function

---

**Remember**: Profile first, optimize second, measure always! 📈
