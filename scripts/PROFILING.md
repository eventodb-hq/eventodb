# Performance Profiling Scripts

This directory contains scripts for profiling MessageDB performance across different backends.

## Available Scripts

### 1. `profile-pebble.sh` - Pebble Backend Profiling

Profiles the Pebble key-value store backend.

```bash
./scripts/profile-pebble.sh
```

**What it does:**
- Cleans up any existing Pebble data
- Starts MessageDB server with Pebble backend
- Runs 30-second load test with 10 workers
- Captures CPU, memory, and allocation profiles
- Measures Pebble-specific metrics (disk usage, namespace count)

**Output:**
- `profiles/YYYYMMDD_HHMMSS-pebble/` directory with:
  - `cpu.prof` - CPU profile during load
  - `heap-before.prof`, `heap-after.prof` - Memory snapshots
  - `allocs-before.prof`, `allocs-after.prof` - Allocation profiles
  - `goroutine.prof` - Goroutine snapshot
  - `load-test-results.json` - Load test metrics
  - `pebble-disk-usage.txt` - Disk usage per namespace
  - `pebble-namespace-count.txt` - Number of open namespaces
  - `README.md` - Analysis guide

**Environment Variables:**
- `PEBBLE_DATA_DIR` - Pebble data directory (default: `/tmp/messagedb-pebble-profile`)

### 2. `profile-baseline.sh` - Multi-Backend Profiling

Profiles SQLite, PostgreSQL, or TimescaleDB backends.

```bash
# SQLite (default)
./scripts/profile-baseline.sh

# PostgreSQL
./scripts/profile-baseline.sh postgres

# TimescaleDB
./scripts/profile-baseline.sh timescale
```

**What it does:**
- Same profiling flow as `profile-pebble.sh`
- Supports multiple SQL backends

**Output:**
- `profiles/YYYYMMDD_HHMMSS-{backend}/` directory with profiles

**Environment Variables (PostgreSQL/TimescaleDB):**
- `POSTGRES_HOST` - PostgreSQL host (default: `localhost`)
- `POSTGRES_PORT` - PostgreSQL port (default: `5432`)
- `POSTGRES_USER` - PostgreSQL user (default: `postgres`)
- `POSTGRES_PASSWORD` - PostgreSQL password (default: `postgres`)
- `POSTGRES_DB` - PostgreSQL database (default: `message_store`)

### 3. `profile-compare.sh` - SQLite vs Pebble Comparison

Runs both SQLite and Pebble profiling back-to-back and generates a comparison report.

```bash
./scripts/profile-compare.sh
```

**What it does:**
1. Profiles SQLite backend (30 seconds)
2. Profiles Pebble backend (30 seconds)
3. Generates side-by-side comparison report

**Output:**
- `profiles/YYYYMMDD_HHMMSS-comparison/` directory with:
  - `sqlite/` - SQLite profiles
  - `pebble/` - Pebble profiles
  - `COMPARISON.md` - Side-by-side comparison report

## Analyzing Profiles

### Quick Analysis (Terminal)

```bash
# View top allocations
go tool pprof -top -alloc_space profiles/TIMESTAMP/allocs-after.prof

# View top CPU consumers
go tool pprof -top profiles/TIMESTAMP/cpu.prof

# Interactive mode
go tool pprof profiles/TIMESTAMP/allocs-after.prof
# Commands: top, list, web, etc.
```

### Web UI (Recommended)

```bash
# Open interactive web UI
go tool pprof -http=:9090 profiles/TIMESTAMP/allocs-after.prof
```

Then open http://localhost:9090 in your browser.

**Useful Views:**
- **Top** - Hottest allocation sites
- **Graph** - Visual call graph
- **Flame Graph** - Hierarchical view
- **Source** - Source code with annotations

### Comparing Profiles

```bash
# Compare before/after allocations
go tool pprof -base=profiles/TIMESTAMP/allocs-before.prof \
              profiles/TIMESTAMP/allocs-after.prof

# Compare SQLite vs Pebble
go tool pprof -base=profiles/TIMESTAMP-comparison/sqlite/allocs-after.prof \
              profiles/TIMESTAMP-comparison/pebble/allocs-after.prof
```

## Key Metrics to Look For

### Allocations (`allocs-after.prof`)
- Total allocated bytes (`-alloc_space`)
- Number of allocations (`-alloc_objects`)
- Look for unexpected allocations in hot paths

### CPU (`cpu.prof`)
- Time spent in each function
- Identify bottlenecks
- Check for unnecessary work in tight loops

### Heap (`heap-after.prof`)
- In-use memory (`-inuse_space`)
- Memory leaks (growing over time)
- Large object allocations

### Goroutines (`goroutine.prof`)
- Number of goroutines
- Goroutine leaks (check for unbounded growth)
- Lock contention (blocked goroutines)

## Pebble-Specific Metrics

### Disk Usage
Shows space used by each namespace:
```bash
cat profiles/TIMESTAMP-pebble/pebble-disk-usage.txt
```

### Namespace Count
Number of open Pebble DB instances:
```bash
cat profiles/TIMESTAMP-pebble/pebble-namespace-count.txt
```

## Load Test Configuration

All profiling scripts use the same load test parameters:
- **Duration**: 30 seconds
- **Workers**: 10 concurrent workers
- **Operations**: Mix of writes and reads
- **Token**: Deterministic token for reproducibility

Modify `scripts/load-test.go` to change test parameters.

## Troubleshooting

### Server Fails to Start
- Check port 8080 is not in use: `lsof -i :8080`
- Check logs in server output
- Increase wait time in script

### Empty Profile Files
- Ensure server has pprof enabled (default)
- Check profile capture commands succeeded
- Verify timeout is sufficient

### Profile Analysis Fails
- Ensure Go toolchain is installed
- Profile files must not be empty
- Use `file` command to check format: `file profile.prof`

### Comparison Shows No Difference
- Ensure load test ran successfully
- Check `load-test-results.json` for actual metrics
- Increase load test duration for more significant results

## Example Workflow

```bash
# 1. Quick Pebble profile
./scripts/profile-pebble.sh

# 2. Analyze allocations
go tool pprof -http=:9090 profiles/LATEST/allocs-after.prof

# 3. Compare with SQLite
./scripts/profile-compare.sh

# 4. View comparison
cat profiles/LATEST-comparison/COMPARISON.md
```

## Tips

1. **Consistent Environment**: Run comparisons on the same machine with minimal background load
2. **Warm-up**: First run may show compilation/initialization overhead
3. **Multiple Runs**: Run multiple times and average results
4. **Profile Before Optimizing**: Always profile before making performance changes
5. **Focus on Hot Paths**: Optimize the top 3-5 allocation/CPU sources first

## References

- [Go pprof Documentation](https://pkg.go.dev/net/http/pprof)
- [Profiling Go Programs](https://go.dev/blog/pprof)
- [go tool pprof Guide](https://github.com/google/pprof/blob/main/doc/README.md)
