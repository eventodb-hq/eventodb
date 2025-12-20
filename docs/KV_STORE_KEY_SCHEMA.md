# KV Store Key Schema - Quick Reference

## Architecture

**Physical Isolation**: Each namespace = separate Pebble database instance

```
/data/eventodb/
├── _metadata/          # Namespace registry (Pebble DB)
├── namespace1/         # Namespace "namespace1" (Pebble DB)
├── namespace2/         # Namespace "namespace2" (Pebble DB)
└── production/         # Namespace "production" (Pebble DB)
```

**Benefits**:
- ✅ No namespace prefix in keys (30-40% size reduction)
- ✅ Physical isolation (security + performance)
- ✅ Independent LSM compaction per namespace

---

## Key Schemas (Per Namespace DB)

All keys below exist within a single namespace's Pebble DB instance.

### 1. Message Data (Primary)
```
Key:   M:{global_position_20}
Value: {message_json}

Example:
M:00000000000000001234 → {"id":"uuid","streamName":"account-123","type":"Deposited",...}
```

### 2. Stream Index
```
Key:   SI:{stream_name}:{position_20}
Value: {global_position_20}

Example:
SI:account-123:00000000000000000005 → 00000000000000001234
```

### 3. Category Index
```
Key:   CI:{category}:{global_position_20}
Value: {stream_name}

Example:
CI:account:00000000000000001234 → account-123
```

### 4. Version Index
```
Key:   VI:{stream_name}
Value: {position_20}

Example:
VI:account-123 → 00000000000000000005
```

### 5. Global Position Counter
```
Key:   GP
Value: {next_global_position_20}

Example:
GP → 00000000000000001235
```

---

## Metadata DB Schema

Separate Pebble DB at `/data/eventodb/_metadata`

### Namespace Registry
```
Key:   NS:{namespace_id}
Value: {namespace_metadata_json}

Example:
NS:production → {"id":"production","tokenHash":"...","dbPath":"/data/eventodb/production",...}
```

---

## Query Patterns

### GetStreamMessages(stream, position, batchSize)
```
1. Range scan: SI:{stream}:{position_20} → SI:{stream}:999...
2. Point lookups: M:{gp} for each result
```

### GetCategoryMessages(category, globalPosition, batchSize)
```
1. Range scan: CI:{category}:{gp_20} → CI:{category}:999...
2. Point lookups: M:{gp} for each result
```

### GetLastStreamMessage(stream, type?)
```
Without type:
1. Get: VI:{stream} → position
2. Get: SI:{stream}:{position} → global_position
3. Get: M:{global_position}

With type:
1. Reverse scan: SI:{stream}: from end
2. Get: M:{gp} for each until type matches
```

### GetStreamVersion(stream)
```
1. Get: VI:{stream} → position (or -1 if not exists)
```

### WriteMessage(stream, msg)
```
Atomic batch (with namespace write mutex):
1. Get: VI:{stream} → currentPosition (-1 if new)
2. Check: expectedVersion == currentPosition (if specified)
3. Get+Increment: GP → nextGP
4. Batch write:
   - M:{nextGP} = message
   - SI:{stream}:{nextPosition} = nextGP
   - CI:{category}:{nextGP} = stream
   - VI:{stream} = nextPosition
   - GP = nextGP + 1
```

---

## Key Size Analysis

### Without Namespace Prefix (Chosen)
```
M:{gp_20}                    = ~22 bytes
SI:{stream}:{pos_20}         = ~40 bytes (avg stream name 15 chars)
CI:{category}:{gp_20}        = ~35 bytes (avg category 10 chars)
VI:{stream}                  = ~25 bytes

Total per message: ~140-160 bytes overhead
```

### With Namespace Prefix (Alternative)
```
M:{ns}:{gp_20}               = ~35 bytes (+13 bytes)
SI:{ns}:{stream}:{pos_20}    = ~53 bytes (+13 bytes)
CI:{ns}:{category}:{gp_20}   = ~48 bytes (+13 bytes)
VI:{ns}:{stream}             = ~38 bytes (+13 bytes)

Total per message: ~190-210 bytes overhead (+50 bytes = 35% larger)
```

**Savings for 1B messages**: 50 GB

---

## Implementation Notes

### Atomicity
- All writes serialized per namespace via `writeMu sync.Mutex`
- Single atomic batch write (5 keys per message)
- Global position counter updated within batch

### Resource Management
```go
type PebbleStore struct {
    metadataDB *pebble.DB                  // Namespace registry
    namespaces map[string]*namespaceHandle // Lazy-loaded namespace DBs
    mu         sync.RWMutex
}

type namespaceHandle struct {
    db      *pebble.DB
    writeMu sync.Mutex  // Serializes writes
}
```

### Pebble Options (Per Namespace)
```go
&pebble.Options{
    Cache:        pebble.NewCache(128 << 20), // 128MB cache
    MemTableSize: 64 << 20,                   // 64MB memtable
    Compression:  pebble.DefaultCompression,  // Snappy
}
```

### Lazy Loading + LRU Eviction
- Open namespace DBs on first access
- Track last access time
- Close LRU namespaces when approaching file descriptor limits
- Reopen on next access

---

## Performance Expectations

| Operation | Complexity | Expected Latency |
|-----------|-----------|------------------|
| WriteMessage | O(log N) | <1ms |
| GetStreamMessages (100) | O(100 * log N) | 1-5ms |
| GetCategoryMessages (100) | O(100 * log N) | 1-5ms |
| GetLastStreamMessage | O(log N) | <1ms |
| GetStreamVersion | O(log N) | <0.5ms |

**N** = total messages in namespace

### LSM Tree Advantages
- Sequential writes are extremely fast (append to log)
- Range scans leverage bloom filters + SST file organization
- Compaction happens in background
- Write amplification < 10x (vs 20-30x for B-trees)

---

## Future Optimizations

### Optional Indexes

1. **Type Index** (for frequent type-filtered last message queries):
   ```
   TI:{stream}:{type}:{position_20_desc} → {global_position_20}
   ```

2. **Correlation Index** (for frequent correlation filtering):
   ```
   CCI:{category}:{correlation_category}:{gp_20} → {stream_name}
   ```

3. **Message in Index** (to avoid point lookups):
   ```
   SI:{stream}:{position_20} → {full_message_json}
   ```
   Trade-off: Larger index, no point lookups needed

### Consumer Group Optimization
Pre-compute consumer assignment in CI value:
```
CI:{category}:{gp_20} → {stream_name}|{consumer_hash_mod_256}
```
Allows server-side filtering without reading messages.

---

## Testing Strategy

1. **Unit Tests**: Key encoding/decoding, range scan logic
2. **Integration Tests**: Reuse existing SQL integration tests
3. **Benchmarks**: Compare WriteMessage, GetStreamMessages, GetCategoryMessages vs SQLite
4. **Consistency Tests**: Verify atomicity of batch writes
5. **Load Tests**: Concurrent writes, consumer groups, large result sets

---

## Files to Create

1. `internal/store/pebble/store.go` - Main store implementation
2. `internal/store/pebble/write.go` - WriteMessage logic
3. `internal/store/pebble/read.go` - Get* operations
4. `internal/store/pebble/namespace.go` - Namespace CRUD
5. `internal/store/pebble/keys.go` - Key encoding/decoding
6. `internal/store/pebble/utils.go` - Helper functions

---

## Summary

**Key Design Choice**: Separate Pebble DB per namespace (no namespace prefix in keys)

**Benefits**:
- 30-40% smaller keys
- Physical isolation
- Independent performance characteristics
- Easy namespace deletion

**Trade-offs**:
- More complex resource management
- Higher memory overhead (~128MB per active namespace)
- File descriptor limits (1024-65536)

**Recommended for**:
- High-throughput event sourcing
- Many isolated namespaces
- Storage cost sensitivity
- Horizontal scalability needs
