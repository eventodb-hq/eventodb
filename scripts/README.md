# EventoDB Profiling Scripts

Consistent, repeatable profiling infrastructure for performance optimization work.

## Quick Start

### 1. Run Baseline Profile

```bash
make profile-baseline
```

This will:
- Build and start the server in test mode
- Run a 30-second load test with 10 concurrent workers
- Capture CPU, memory, and allocation profiles
- Save results to `profiles/YYYYMMDD_HHMMSS/`
- Display top allocation sources

### 2. Make Your Optimization

Edit code, implement your optimization...

### 3. Run Optimized Profile

```bash
make profile-baseline
```

(Creates a new profile directory with timestamp)

### 4. Compare Results

```bash
make profile-compare BASELINE=profiles/20241218_120000 OPTIMIZED=profiles/20241218_130000
```

This will show:
- Throughput improvement (ops/sec)
- Latency improvement (ms)
- Allocation changes (by function)
- Top allocation sources before/after

## Files Generated

Each profile run creates a timestamped directory with:

```
profiles/YYYYMMDD_HHMMSS/
├── README.md                 # Summary of this run
├── cpu.prof                  # CPU profile (10 sec during load)
├── heap-before.prof          # Heap before load test
├── heap-after.prof           # Heap after load test
├── allocs-before.prof        # Allocations before load
├── allocs-after.prof         # PRIMARY: Allocation analysis
├── goroutine.prof            # Goroutine dump
└── load-test-results.json    # Performance metrics
```

## Analysis Commands

### View Top Allocations

```bash
go tool pprof -top -alloc_space profiles/YYYYMMDD_HHMMSS/allocs-after.prof
```

### Interactive Analysis

```bash
go tool pprof profiles/YYYYMMDD_HHMMSS/allocs-after.prof
```

Commands inside pprof:
- `top` - Show top allocations
- `list functionName` - Show annotated source for function
- `web` - Generate graph (requires graphviz)
- `help` - Show all commands

### Web UI (Recommended)

```bash
go tool pprof -http=:9090 profiles/YYYYMMDD_HHMMSS/allocs-after.prof
```

Open http://localhost:9090 in browser for interactive visualization.

### Compare Two Profiles

```bash
go tool pprof -base=profiles/baseline/allocs-after.prof profiles/optimized/allocs-after.prof
```

Shows delta (negative values = improvement).

## Load Test Parameters

Configurable in `scripts/load-test.go`:

- **Duration**: 30 seconds (default)
- **Workers**: 10 concurrent workers
- **Mix**: 90% writes, 10% reads
- **Streams**: 100 unique streams (cycling)
- **Payload**: Realistic nested JSON data

## Understanding Results

### Good Optimization Indicators

✅ **Allocation reduction**: 20%+ fewer allocs/op  
✅ **Throughput increase**: 10%+ more ops/sec  
✅ **Latency decrease**: 5%+ lower avg latency  
✅ **No regression**: CPU usage stays same or better

### Red Flags

⚠️ **CPU increase**: New hotspots appearing  
⚠️ **Correctness**: Tests failing  
⚠️ **Memory leak**: Heap-after > heap-before by large margin  
⚠️ **Goroutine leak**: Goroutine count growing

## Manual Profiling

### Start Server with Profiling

```bash
./bin/eventodb --test-mode --port 8080
```

pprof endpoints are automatically available at:
- `http://localhost:8080/debug/pprof/`
- `http://localhost:8080/debug/pprof/heap`
- `http://localhost:8080/debug/pprof/profile?seconds=30`
- `http://localhost:8080/debug/pprof/allocs`

### Capture Specific Profile

```bash
# CPU profile (30 seconds)
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof

# Heap snapshot
curl http://localhost:8080/debug/pprof/heap > heap.prof

# Allocation profile
curl http://localhost:8080/debug/pprof/allocs > allocs.prof
```

### Run Custom Load Test

```bash
go run scripts/load-test.go \
  --duration 60s \
  --workers 20 \
  --url http://localhost:8080 \
  --profile-dir ./my-profile
```

## Tips

### Focus on Hot Paths

Use `-alloc_space` flag to see total allocations (not just current heap):

```bash
go tool pprof -top -alloc_space allocs-after.prof
```

### Find Allocation Sources

```bash
go tool pprof -list=functionName -alloc_space allocs-after.prof
```

Shows line-by-line allocations in that function.

### Flame Graphs

```bash
go tool pprof -http=:9090 allocs-after.prof
```

Then click "View → Flame Graph" in web UI.

### Compare Specific Functions

```bash
go tool pprof -base=baseline.prof optimized.prof
(pprof) list MyFunction
```

Shows allocation delta for specific function.

## Common Issues

### Server Won't Start

- Check port 8080 is available: `lsof -i :8080`
- Check logs for database connection issues

### Profile Empty

- Make sure load test ran successfully
- Check `load-test-results.json` for errors
- Increase load test duration or workers

### Can't Install pprof

Already included with Go! Just use `go tool pprof`.

For graphviz (for `web` command):
```bash
# macOS
brew install graphviz

# Ubuntu
sudo apt-get install graphviz
```

## References

- [Go pprof Documentation](https://pkg.go.dev/net/http/pprof)
- [Profiling Go Programs](https://go.dev/blog/pprof)
- [Go Performance Tips](https://github.com/dgryski/go-perfbook)
