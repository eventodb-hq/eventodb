# ISSUE002: Zero-Allocation Performance Optimizations

**Status**: ✅ Ready for Implementation  
**Baseline**: ✅ Captured (10,986 ops/sec @ 0.91ms)  
**Infrastructure**: ✅ Validated

---

## 📖 Quick Navigation

| Document | Purpose | When to Use |
|----------|---------|-------------|
| **👉 [QUICKSTART](ISSUE002-QUICKSTART.md)** | Fast workflow guide | Daily optimization work |
| [SUMMARY](ISSUE002-SUMMARY.md) | Project overview | Understanding scope |
| [BASELINE-RESULTS](ISSUE002-BASELINE-RESULTS.md) | Actual measurements | Comparing improvements |
| [Full Specification](ISSUE002-performance-optimizations.md) | Complete details | Implementation reference |

---

## 🚀 Get Started in 30 Seconds

```bash
# 1. Read the quickstart
cat @meta/@pm/issues/ISSUE002-QUICKSTART.md

# 2. See what we found
cat @meta/@pm/issues/ISSUE002-BASELINE-RESULTS.md

# 3. Run your own profile
make profile-baseline

# 4. View results in browser
go tool pprof -http=:9090 profiles/LATEST/allocs-after.prof
```

---

## 📊 Current Baseline

**Performance**: 10,986 ops/sec @ 0.91ms latency  
**Profiled**: 330K operations over 30 seconds  
**Allocations Tracked**: 4.66 GB

### Top Hotspots Identified

| Priority | Target | Current Allocations | Expected Reduction |
|----------|--------|--------------------|--------------------|
| 🔴 #1 | JSON Marshal/Unmarshal | 865 MB (18.5%) | 30-50% |
| 🟡 #2 | Message Slice Pre-alloc | ~200 MB (est.) | 10-15% |
| 🟡 #3 | String Splitting | ~150 MB (est.) | 5-10% |
| 🟡 #4 | Poke Object Pooling | ~100 MB (SSE) | 15-25% |
| 🟡 #5 | Response Map Pooling | 155 MB | 10-20% |

---

## ✅ What's Complete

### Infrastructure
- [x] Profiling scripts with automated load testing
- [x] Baseline profile captured and analyzed
- [x] Comparison tools for before/after analysis
- [x] pprof endpoints integrated into server
- [x] Makefile targets for easy workflow

### Documentation
- [x] Full specification with code examples
- [x] Quick-start workflow guide
- [x] Baseline results analysis
- [x] Success criteria defined
- [x] Scripts documentation

### Validation
- [x] 10,986 ops/sec baseline throughput
- [x] 0.91ms average latency
- [x] Zero errors during 30s sustained load
- [x] All 5 optimization targets confirmed by profiling data

---

## 🎯 Implementation Plan

### Phase 1: JSON Optimization (Week 1)
**Target**: Priority #1 - JSON Marshal/Unmarshal

**Current**: 865 MB allocations (18.5% of total)  
**Goal**: Reduce to 350-450 MB (-48% to -58%)

**Approaches**:
1. Replace `encoding/json` with `jsoniter` (drop-in replacement)
2. Add `sync.Pool` for byte buffers
3. Consider `easyjson` for hot paths

**Success Criteria**:
- [ ] Throughput: 10,986 → 13,000+ ops/sec (+18%)
- [ ] Latency: 0.91 → 0.75 ms (-17%)
- [ ] JSON allocations: -400-500 MB

**Files to Change**:
- `internal/store/timescale/read.go`
- `internal/store/timescale/write.go`
- `internal/store/postgres/read.go`
- `internal/store/postgres/write.go`
- `internal/store/sqlite/read.go`
- `internal/store/sqlite/write.go`

### Phase 2: String & Slice Optimizations (Week 2)
**Targets**: Priority #2 & #3

**Expected Combined Impact**: +10-15% throughput improvement

### Phase 3: Object Pooling (Week 2-3)
**Targets**: Priority #4 & #5

**Expected Combined Impact**: +5-10% throughput improvement

---

## 📈 Expected Final Results

After all 5 optimizations:

| Metric | Baseline | Target | Improvement |
|--------|----------|--------|-------------|
| Throughput | 10,986 ops/sec | 14,000+ ops/sec | +27% |
| Latency | 0.91 ms | 0.68 ms | -25% |
| Allocations | 4,661 MB | 3,200 MB | -31% |

---

## 🔧 Daily Workflow

### Making an Optimization

```bash
# 1. Create branch
git checkout -b perf/json-optimization

# 2. Make your changes
vim golang/internal/store/timescale/write.go

# 3. Test
cd golang && go test ./...

# 4. Profile
cd .. && make profile-baseline

# 5. Compare
make profile-compare \
  BASELINE=profiles/20251218_211750 \
  OPTIMIZED=profiles/YYYYMMDD_HHMMSS

# 6. Verify success criteria met
# - 20%+ allocation reduction? ✓
# - 10%+ throughput gain? ✓
# - All tests pass? ✓
# - No CPU regression? ✓

# 7. Commit with profile comparison in message
git commit -am "perf: optimize JSON marshaling

Replaced encoding/json with jsoniter in store layer.

Performance Impact:
- Throughput: 10,986 → 13,200 ops/sec (+20%)
- Latency: 0.91 → 0.76 ms (-16%)
- JSON allocations: 865 MB → 380 MB (-56%)

See profiles/comparison for details."
```

---

## 📚 Learning Resources

### First Time Profiling Go?

1. **Watch**: [Profiling & Optimizing Go Programs](https://www.youtube.com/watch?v=N3PWzBeLX2M)
2. **Read**: [Go pprof Documentation](https://pkg.go.dev/net/http/pprof)
3. **Try**: `make profile-baseline` and explore the web UI
4. **Practice**: Make a small change and see the impact

### Understanding pprof Output

```bash
# Start web UI
go tool pprof -http=:9090 profiles/20251218_211750/allocs-after.prof

# Then explore:
# - Top view: See biggest allocators
# - Graph view: See call chains (visual)
# - Flame graph: See time/space distribution
# - Source view: See line-by-line allocations
```

---

## ❓ FAQ

### Q: Can I skip the baseline and start optimizing?

**A**: No! The baseline gives you:
- Proof that your optimization worked
- Data to guide what to optimize next
- Protection against making things worse

### Q: What if my optimization makes things worse?

**A**: That's valuable data! Document it and try a different approach. The profiling infrastructure makes this safe to discover.

### Q: How do I know which optimization to do first?

**A**: We've already prioritized based on the baseline profile. Start with #1 (JSON) - it's 18.5% of all allocations.

### Q: Can I run profiles on production?

**A**: Yes! pprof is production-safe. Grab a quick profile:
```bash
curl https://prod-server/debug/pprof/profile?seconds=30 > prod.prof
```

### Q: The comparison shows small improvements. Is that OK?

**A**: Depends on the target. Each optimization has success criteria:
- **Must have**: 20%+ allocation reduction in targeted code
- **Should have**: 10%+ throughput OR 5%+ latency improvement
- **Nice to have**: Combined effects across optimizations

---

## 🎓 What You'll Learn

By completing ISSUE002, you'll gain experience with:

1. **Profile-Driven Optimization**
   - How to identify real bottlenecks (not guesses)
   - How to measure impact objectively
   - How to avoid premature optimization

2. **Go Performance Techniques**
   - sync.Pool for object reuse
   - Slice pre-allocation patterns
   - Zero-allocation string operations
   - JSON optimization strategies

3. **Production Profiling**
   - Continuous profiling setup
   - Regression detection
   - Performance monitoring

---

## 🎉 Success Criteria

ISSUE002 will be considered complete when:

- [ ] All 5 optimizations implemented
- [ ] Each optimization shows measurable improvement
- [ ] Combined throughput increase >25%
- [ ] All tests passing (no regressions)
- [ ] Documentation updated
- [ ] Profiling workflow established for future work

---

## 📞 Need Help?

1. **Check the docs**: Start with QUICKSTART.md
2. **Review baseline**: See what we already found
3. **Run a profile**: Make sure infrastructure works
4. **Compare profiles**: Verify you can see differences

---

**Ready to optimize?** Start here: [QUICKSTART.md](ISSUE002-QUICKSTART.md)
