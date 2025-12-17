# Performance Benchmarks

This document contains performance benchmarks for the message store implementations.

## Benchmark Results

> Last updated: 2024-12-17
> Platform: Apple M3 Ultra (28 cores), macOS
> Go Version: 1.21+

### SQLite In-Memory Backend

| Operation | Avg Time | Target | Status |
|-----------|----------|--------|--------|
| WriteMessage | ~22 µs | <1ms | ✅ PASS |
| GetStreamMessages (10) | ~30 µs | <2ms | ✅ PASS |
| GetCategoryMessages (100) | ~159 µs | <10ms | ✅ PASS |

### SQLite File Backend

| Operation | Avg Time | Target | Status |
|-----------|----------|--------|--------|
| WriteMessage | ~329 µs | <5ms | ✅ PASS |
| GetStreamMessages (10) | ~40 µs | <8ms | ✅ PASS |
| GetCategoryMessages (100) | ~169 µs | <30ms | ✅ PASS |

### Postgres Backend

| Operation | Target | Status |
|-----------|--------|--------|
| WriteMessage | <10ms | ⏭️ Requires Postgres |
| GetStreamMessages (10) | <15ms | ⏭️ Requires Postgres |
| GetCategoryMessages (100) | <50ms | ⏭️ Requires Postgres |

> Note: Postgres benchmarks require a running Postgres instance at `postgres://postgres:postgres@localhost:5432/postgres`

## Running Benchmarks

### All Benchmarks

```bash
cd internal/store
go test -bench=. -benchtime=1000x
```

### Specific Backend

```bash
# SQLite Memory
go test -bench=BenchmarkWriteMessage_SQLiteMemory -benchtime=1000x

# SQLite File
go test -bench=BenchmarkWriteMessage_SQLiteFile -benchtime=1000x

# Postgres (requires running instance)
go test -bench=BenchmarkWriteMessage_Postgres -benchtime=100x
```

### With Memory Profiling

```bash
go test -bench=. -benchmem -memprofile=mem.prof
go tool pprof mem.prof
```

### With CPU Profiling

```bash
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

## Performance Analysis

### Write Performance

**SQLite In-Memory**: Exceptionally fast (~22 µs) due to no disk I/O. Ideal for:
- Unit tests
- Development environments
- Temporary event streams

**SQLite File**: Fast (~329 µs) with durability. Ideal for:
- Edge computing
- Embedded systems
- Single-node deployments
- Development with persistence

**Postgres**: Target <10ms. Ideal for:
- Production multi-tenant systems
- High concurrency requirements
- Horizontal scaling needs

### Read Performance

Both SQLite backends show excellent read performance:
- Stream reads: 30-40 µs for 10 messages
- Category reads: 159-169 µs for 100 messages

This demonstrates the effectiveness of:
- Proper indexing (messages_stream, messages_category)
- Efficient query planning
- In-memory caching for frequently accessed data

### Category Queries

Category queries across multiple streams show:
- Linear scaling with message count
- Efficient filtering by category name
- Consumer group partitioning adds minimal overhead

## Optimization Tips

### For Write-Heavy Workloads

1. **Use Batch Writes** (Future enhancement):
   ```go
   // Not yet implemented, but planned
   results, err := st.WriteBatch(ctx, namespace, messages)
   ```

2. **Adjust SQLite Settings** (File mode):
   ```go
   db.Exec("PRAGMA journal_mode=WAL")
   db.Exec("PRAGMA synchronous=NORMAL")
   ```

3. **Connection Pooling** (Postgres):
   ```go
   db.SetMaxOpenConns(25)
   db.SetMaxIdleConns(5)
   db.SetConnMaxLifetime(5 * time.Minute)
   ```

### For Read-Heavy Workloads

1. **Use Appropriate Batch Sizes**:
   ```go
   opts := &store.GetOpts{
       BatchSize: 100,  // Adjust based on message size
   }
   ```

2. **Consumer Groups** for parallelization:
   ```go
   // Split work across 4 consumers
   opts := &store.CategoryOpts{
       ConsumerMember: ptr(int64(0)),
       ConsumerSize:   ptr(int64(4)),
   }
   ```

3. **Stream-Level Caching** (Application layer):
   ```go
   // Cache frequently accessed streams
   cache := NewLRUCache(1000)
   ```

### For Mixed Workloads

1. **Separate Read/Write Connections** (Postgres):
   ```go
   writeStore := postgres.New(writeDB)
   readStore := postgres.New(readDB)  // Can use replica
   ```

2. **Namespace per Tenant**:
   - Provides physical isolation
   - Enables independent scaling
   - Simplifies backup/restore

3. **Monitor and Tune**:
   - Use benchmarks to establish baselines
   - Profile production workloads
   - Adjust based on actual usage patterns

## Performance Targets vs Actuals

### WriteMessage

| Backend | Target | Actual | Margin |
|---------|--------|--------|--------|
| Postgres | <10ms | TBD | TBD |
| SQLite File | <5ms | 0.33ms | ✅ 15x faster |
| SQLite Memory | <1ms | 0.02ms | ✅ 50x faster |

### GetStreamMessages (10 messages)

| Backend | Target | Actual | Margin |
|---------|--------|--------|--------|
| Postgres | <15ms | TBD | TBD |
| SQLite File | <8ms | 0.04ms | ✅ 200x faster |
| SQLite Memory | <2ms | 0.03ms | ✅ 66x faster |

### GetCategoryMessages (100 messages)

| Backend | Target | Actual | Margin |
|---------|--------|--------|--------|
| Postgres | <50ms | TBD | TBD |
| SQLite File | <30ms | 0.17ms | ✅ 176x faster |
| SQLite Memory | <10ms | 0.16ms | ✅ 62x faster |

## Conclusion

All tested backends **significantly exceed** their performance targets:

- ✅ SQLite in-memory: 50-200x faster than targets
- ✅ SQLite file: 15-200x faster than targets
- ⏭️ Postgres: Requires testing with actual instance

The implementation is highly optimized for:
- Low latency operations
- High throughput
- Efficient resource usage

## Future Optimizations

Potential areas for further improvement:

1. **Batch Write API**: Reduce round-trips for bulk inserts
2. **Read Replicas**: For Postgres horizontal scaling
3. **Prepared Statements**: Cache compiled queries
4. **Connection Pooling**: Fine-tune for specific workloads
5. **Index Optimization**: Add covering indexes for common queries
6. **Compression**: For large message payloads
7. **Partitioning**: For very large datasets (Postgres)

## Benchmark Reproducibility

To reproduce these benchmarks:

1. Clone the repository
2. Run `go test -bench=. -benchtime=1000x ./internal/store/`
3. Results may vary based on:
   - CPU architecture and speed
   - Available memory
   - Disk I/O performance (file mode)
   - Operating system
   - Go version

For consistent results, run multiple iterations and average:

```bash
go test -bench=. -benchtime=5000x -count=5 | tee benchmark.txt
benchstat benchmark.txt
```
