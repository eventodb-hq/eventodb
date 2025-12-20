# EventoDB Performance Tuning Guide

This guide covers performance optimization for the EventoDB Go server.

## Performance Targets

| Operation | Target (p95) | Acceptable (p99) |
|-----------|--------------|------------------|
| Stream write | < 10ms | < 20ms |
| Stream read (100 msgs) | < 20ms | < 40ms |
| Category read (100 msgs) | < 30ms | < 60ms |
| SSE poke delivery | < 5ms | < 10ms |
| Throughput | > 1000 writes/sec | > 500 writes/sec |

## Benchmarking

### Running Benchmarks

```bash
cd test_external

# Run all benchmarks
bun run bench

# Run specific benchmark
bun test benchmarks/performance.bench.ts
```

### Sample Results

```
┌─────────────────────────────┬──────────┬──────────┬──────────┐
│ Benchmark                   │ ops/sec  │ avg (ms) │ p99 (ms) │
├─────────────────────────────┼──────────┼──────────┼──────────┤
│ stream.write (single)       │ 5,234    │ 0.19     │ 0.45     │
│ stream.get (100 messages)   │ 3,456    │ 0.29     │ 0.72     │
│ category.get (100 messages) │ 2,123    │ 0.47     │ 1.23     │
│ concurrent writes (10)      │ 12,345   │ 0.81     │ 2.34     │
└─────────────────────────────┴──────────┴──────────┴──────────┘
```

## Optimization Strategies

### 1. Batch Operations

#### Batch Reads

Instead of multiple small reads, use larger batch sizes:

```javascript
// ❌ Slow: Multiple small batches
for (let i = 0; i < 10; i++) {
  const batch = await client.getStream('account-123', {
    position: i * 10,
    batchSize: 10
  });
}

// ✅ Fast: Single large batch
const messages = await client.getStream('account-123', {
  batchSize: 100
});
```

#### Batch Size Guidelines

| Use Case | Recommended Batch Size |
|----------|----------------------|
| Real-time processing | 10-50 |
| Catch-up/replay | 100-1000 |
| Export/backup | 1000-10000 |

### 2. Connection Management

#### HTTP Keep-Alive

Reuse HTTP connections to avoid connection overhead:

```javascript
// Node.js/Bun: Keep-alive is default
const client = new EventoDBClient('http://localhost:8080', { token });

// Make multiple requests on same connection
await client.writeMessage('stream-1', msg1);
await client.writeMessage('stream-2', msg2);
```

#### Connection Pooling

For high-throughput scenarios, use connection pooling:

```javascript
// Using undici (Node.js)
import { Pool } from 'undici';

const pool = new Pool('http://localhost:8080', {
  connections: 10,
  pipelining: 1
});
```

### 3. Optimistic Locking Strategy

#### When to Use

Use `expectedVersion` only when needed:

```javascript
// ❌ Unnecessary: Event streams without conflicts
await client.writeMessage('events-123', event, { expectedVersion: 0 });

// ✅ Necessary: Aggregate streams with business logic
await client.writeMessage('account-123', command, { expectedVersion: currentVersion });
```

#### Retry Strategy

Implement exponential backoff for conflicts:

```javascript
async function writeWithRetry(client, stream, message, maxRetries = 3) {
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      const version = await client.getStreamVersion(stream) ?? -1;
      return await client.writeMessage(stream, message, {
        expectedVersion: version
      });
    } catch (error) {
      if (error.message !== 'STREAM_VERSION_CONFLICT') throw error;
      if (attempt === maxRetries - 1) throw error;
      await sleep(Math.pow(2, attempt) * 10); // 10ms, 20ms, 40ms
    }
  }
}
```

### 4. Consumer Groups

#### Optimal Group Size

Balance between parallelism and overhead:

| Scenario | Recommended Size |
|----------|-----------------|
| Low volume (< 100 msgs/sec) | 1-2 consumers |
| Medium volume (100-1000 msgs/sec) | 4-8 consumers |
| High volume (> 1000 msgs/sec) | 8-16 consumers |

#### Even Distribution

Ensure even distribution by using descriptive stream IDs:

```javascript
// ❌ Poor distribution: Sequential IDs
'account-1', 'account-2', 'account-3'

// ✅ Better distribution: UUID-based IDs
'account-a1b2c3d4', 'account-e5f6g7h8', 'account-i9j0k1l2'
```

### 5. SSE Subscriptions

#### Subscription Positioning

Start subscriptions from the last processed position:

```javascript
// Load last processed position from state store
const lastPosition = await loadCheckpoint('account-consumer-0');

// Subscribe from checkpoint
client.subscribeToCategory('account', {
  position: lastPosition,
  consumerGroup: { member: 0, size: 4 },
  onPoke: async (poke) => {
    // Process and checkpoint
    await processMessages(poke.globalPosition);
    await saveCheckpoint('account-consumer-0', poke.globalPosition);
  }
});
```

#### Backpressure Handling

Don't let pokes pile up:

```javascript
let processing = false;

client.subscribeToCategory('account', {
  onPoke: async (poke) => {
    if (processing) return; // Skip if still processing
    processing = true;
    try {
      await processMessages();
    } finally {
      processing = false;
    }
  }
});
```

### 6. Query Optimization

#### Use Appropriate Filters

```javascript
// ❌ Fetch all, filter in client
const all = await client.getCategory('account');
const filtered = all.filter(m => m[5].metadata?.correlation === 'workflow-123');

// ✅ Server-side filtering
const filtered = await client.getCategory('account', {
  correlation: 'workflow'
});
```

#### Position-Based Pagination

Use global position for deterministic pagination:

```javascript
let position = 0;

while (true) {
  const messages = await client.getCategory('account', {
    globalPosition: position,
    batchSize: 100
  });
  
  if (messages.length === 0) break;
  
  // Process messages
  for (const msg of messages) {
    await process(msg);
  }
  
  // Update position to after last message
  position = messages[messages.length - 1][4] + 1;
}
```

## Server Configuration

### Go Runtime

Tune Go runtime for your workload:

```bash
# Increase GOMAXPROCS for CPU-bound workloads
export GOMAXPROCS=8

# Reduce GC pressure for memory-intensive workloads
export GOGC=200

./eventodb serve
```

### HTTP Server

Server defaults are tuned for high concurrency:

```go
server := &http.Server{
    ReadTimeout:       30 * time.Second,
    WriteTimeout:      30 * time.Second,
    IdleTimeout:       120 * time.Second,
    ReadHeaderTimeout: 10 * time.Second,
    MaxHeaderBytes:    1 << 20, // 1 MB
}
```

### SQLite (Test Mode)

For test mode, optimize SQLite:

```go
// Already configured in code:
// - WAL mode for concurrent reads
// - Shared cache for memory databases
// - IMMEDIATE transactions for write safety
```

### PostgreSQL (Production)

Optimize PostgreSQL for EventoDB:

```sql
-- postgresql.conf

# Memory
shared_buffers = 2GB
effective_cache_size = 6GB
work_mem = 64MB

# WAL
wal_level = replica
max_wal_size = 4GB

# Connections
max_connections = 200

# Query planning
random_page_cost = 1.1  # For SSD
effective_io_concurrency = 200

# Checkpoints
checkpoint_completion_target = 0.9
```

## Monitoring Performance

### Key Metrics

1. **Request Latency**: Track p50, p95, p99
2. **Throughput**: Requests per second
3. **Error Rate**: Failed requests percentage
4. **Connection Pool**: Active/idle connections
5. **SSE Connections**: Active subscriptions

### Prometheus Metrics (Planned)

```yaml
# Example Grafana dashboard queries
- name: Request Latency
  query: histogram_quantile(0.95, rate(eventodb_request_duration_seconds_bucket[5m]))

- name: Throughput
  query: rate(eventodb_requests_total[1m])

- name: Error Rate
  query: rate(eventodb_requests_total{status="error"}[5m]) / rate(eventodb_requests_total[5m])
```

### Log Analysis

Parse request logs for performance insights:

```bash
# Find slow requests (> 100ms)
grep "duration" eventodb.log | awk '$NF > 100 {print}'

# Count requests by method
grep "method" eventodb.log | awk '{print $4}' | sort | uniq -c | sort -rn
```

## Common Performance Issues

### Issue: High Latency

**Symptoms**: Request times > 100ms

**Causes**:
1. Network latency between client and server
2. Database connection pool exhaustion
3. Large batch sizes
4. Slow disk I/O

**Solutions**:
1. Deploy closer to clients (same region/zone)
2. Increase connection pool size
3. Reduce batch sizes
4. Use SSD storage

### Issue: Low Throughput

**Symptoms**: < 500 requests/sec

**Causes**:
1. Single-threaded client
2. Sequential request pattern
3. High lock contention
4. Inadequate resources

**Solutions**:
1. Parallelize client requests
2. Batch operations
3. Use optimistic locking sparingly
4. Scale horizontally

### Issue: SSE Disconnections

**Symptoms**: Subscriptions dropping

**Causes**:
1. Network timeouts
2. Proxy buffering
3. Server resource exhaustion

**Solutions**:
1. Increase timeout settings
2. Disable proxy buffering for SSE
3. Scale server resources
4. Implement reconnection logic

## Benchmarking Your Setup

### Quick Performance Check

```bash
# Write benchmark (100 messages)
time for i in {1..100}; do
  curl -s -X POST http://localhost:8080/rpc \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "[\"stream.write\", \"bench-$i\", {\"type\": \"Test\", \"data\": {}}]"
done

# Read benchmark
time curl -s -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.get", "bench-1", {"batchSize": 100}]'
```

### Load Testing

Use tools like `wrk` or `hey`:

```bash
# Install hey
go install github.com/rakyll/hey@latest

# Run load test
hey -n 10000 -c 50 -m POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.get", "bench", {"batchSize": 10}]' \
  http://localhost:8080/rpc
```

### Continuous Benchmarking

Run benchmarks in CI to detect regressions:

```yaml
# .github/workflows/benchmark.yml
- name: Run benchmarks
  run: bun run bench > results.json

- name: Compare with baseline
  run: |
    # Fail if performance degraded > 20%
    ./compare-benchmark.sh results.json baseline.json 20
```

## Summary

1. **Batch operations** - Larger batches for throughput, smaller for latency
2. **Reuse connections** - HTTP keep-alive, connection pooling
3. **Optimistic locking** - Use only when needed, implement retry
4. **Consumer groups** - Scale processing horizontally
5. **SSE positioning** - Start from checkpoints, handle backpressure
6. **Monitor metrics** - Track latency, throughput, errors
7. **Tune configuration** - Adjust Go runtime, database settings
