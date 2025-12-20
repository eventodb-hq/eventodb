# KV Store Backend Design for EventoDB

## Executive Summary

This document proposes a key structure design for implementing EventoDB on top of **Pebble** (LSM-based key-value store from CockroachDB) while maintaining full compatibility with the existing SQL-based API.

### Architecture Decision: Separate Pebble DB Per Namespace ✅

**Each namespace gets its own Pebble database instance** for true physical isolation.

**Key Benefits**:
- **30-40% smaller keys**: No namespace prefix overhead
- **Physical isolation**: Independent LSM trees, better security
- **Better performance**: Focused compaction, no cross-namespace interference
- **Easy deletion**: Just remove directory
- **Storage savings**: 55 GB saved per 1B messages (vs prefix approach)

**Directory Structure**:
```
/data/eventodb/
├── _metadata/          # Namespace registry (Pebble DB)
├── production/         # Namespace "production" (Pebble DB)
├── staging/            # Namespace "staging" (Pebble DB)
└── customer-123/       # Namespace "customer-123" (Pebble DB)
```

**See also**: 
- `./KV_STORE_KEY_SCHEMA.md` - Quick reference for key schemas
- `./KV_COMPARISON.md` - Detailed comparison with key-prefix approach

## Current API Analysis

### Core Operations to Support

1. **Write Operations**
   - `WriteMessage(namespace, streamName, msg)` - Sequential writes with optimistic locking
   - Auto-increment position per stream
   - Auto-increment global position across namespace

2. **Read Operations**
   - `GetStreamMessages(namespace, stream, position, batchSize)` - Range query by stream position
   - `GetCategoryMessages(namespace, category, globalPosition, batchSize)` - Range query by global position
   - `GetLastStreamMessage(namespace, stream, msgType?)` - Point lookup + reverse scan
   - `GetStreamVersion(namespace, stream)` - Get max position for stream

3. **Query Features**
   - Correlation filtering: `metadata.correlationStreamName`
   - Consumer group partitioning: Hash-based distribution
   - Batch size limits
   - Position-based pagination

4. **Namespace Operations**
   - Physical isolation between namespaces
   - CRUD operations on namespace metadata

---

## Key Structure Design

### Design Principles

1. **Physical Namespace Isolation**: Each namespace = separate Pebble database instance
2. **Lexicographic Ordering**: Keys designed for efficient range scans
3. **Zero-padding**: Numeric values padded for correct lexicographic sorting
4. **Composite Keys**: Multiple indexes for different query patterns
5. **Minimal Duplication**: Balance between query efficiency and storage overhead
6. **No Namespace in Keys**: Keys are shorter without namespace prefix

### Key Prefixes Convention

**Note**: All keys are scoped to a single namespace's Pebble DB instance.

```
M:      Message data (primary)
SI:     Stream Index (stream + position → global_position)
CI:     Category Index (category + global_position → stream)
VI:     Version Index (stream → latest position)
GP:     Global Position Counter (auto-increment)
```

**Namespace metadata** is stored in a separate metadata Pebble DB.

---

## Detailed Key Schemas

### 1. Namespace Metadata (Metadata DB)

**Purpose**: Store namespace configuration and metadata in separate metadata Pebble DB

**Key Structure**:
```
NS:{namespace_id}
```

**Value Structure** (JSON):
```json
{
  "id": "myapp",
  "tokenHash": "sha256_hash",
  "description": "My Application",
  "dbPath": "/data/eventodb/myapp.db",
  "createdAt": "2024-01-15T10:30:00Z",
  "metadata": {}
}
```

**Example**:
```
Key:   NS:myapp
Value: {"id":"myapp","tokenHash":"abc123...","dbPath":"/data/eventodb/myapp.db",...}
```

**Storage Location**: Separate metadata Pebble DB at `/data/eventodb/_metadata`

---

### 2. Message Data (Primary Storage)

**Purpose**: Store complete message data, indexed by global position

**Key Structure**:
```
M:{global_position_20}
```

Where `global_position_20` is zero-padded to 20 digits for correct lexicographic ordering.

**Value Structure** (JSON):
```json
{
  "id": "uuid-v4",
  "streamName": "account-123",
  "type": "AccountCreated",
  "position": 0,
  "globalPosition": 1,
  "data": {...},
  "metadata": {...},
  "time": "2024-01-15T10:30:00.123456789Z"
}
```

**Example** (in namespace "myapp"'s Pebble DB):
```
Key:   M:00000000000000001001
Value: {"id":"a1b2c3...","streamName":"account-123",...}

Key:   M:00000000000000001002
Value: {"id":"d4e5f6...","streamName":"account-456",...}
```

**Why this design?**
- Natural ordering by global position
- Efficient for category queries (scan by global position)
- Supports position-based pagination
- Single source of truth for message data

---

### 3. Stream Index

**Purpose**: Map (stream, position) → global_position for efficient stream queries

**Key Structure**:
```
SI:{stream_name}:{position_20}
```

**Value**: 
```
{global_position_20}
```
(20-digit zero-padded integer as string or binary)

**Example** (in namespace "myapp"'s Pebble DB):
```
Key:   SI:account-123:00000000000000000000
Value: 00000000000000001001

Key:   SI:account-123:00000000000000000001
Value: 00000000000000001003

Key:   SI:account-456:00000000000000000000
Value: 00000000000000001002
```

**Query Pattern**:
```
GetStreamMessages("account-123", position=0, batchSize=10):
1. Range scan: SI:account-123:00000000000000000000 → SI:account-123:99999999999999999999
2. Limit to 10 keys
3. Extract global positions from values
4. Lookup messages: M:{global_position} for each
```

---

### 4. Category Index

**Purpose**: Map (category, global_position) → stream_name for category queries

**Key Structure**:
```
CI:{category}:{global_position_20}
```

**Value**:
```
{stream_name}
```

**Example** (in namespace "myapp"'s Pebble DB):
```
Key:   CI:account:00000000000000001001
Value: account-123

Key:   CI:account:00000000000000001002
Value: account-456

Key:   CI:account:00000000000000001003
Value: account-123
```

**Query Pattern**:
```
GetCategoryMessages("account", position=1000, batchSize=100):
1. Range scan: CI:account:00000000000000001000 → CI:account:99999999999999999999
2. Limit to 100 keys
3. Extract global positions from keys
4. Lookup messages: M:{global_position} for each
```

**Correlation Filtering**:
```
GetCategoryMessages("account", correlation="workflow"):
1. Range scan: CI:account:{start_gp} → CI:account:{end_gp}
2. For each message, check if metadata.correlationStreamName starts with "workflow-"
3. Filter in application layer (or store correlation in value for index filtering)
```

**Consumer Group Filtering**:
```
GetCategoryMessages("account", consumerMember=0, consumerSize=4):
1. Range scan: CI:account:{start_gp} → CI:account:{end_gp}
2. For each stream_name in value:
   - Extract cardinalID from stream_name
   - hash = Hash64(cardinalID)
   - if (hash % consumerSize) == consumerMember: include
3. Stop when batchSize messages collected
```

---

### 5. Version Index

**Purpose**: Track latest position for each stream (for optimistic locking)

**Key Structure**:
```
VI:{stream_name}
```

**Value**:
```
{position_20}
```

**Example** (in namespace "myapp"'s Pebble DB):
```
Key:   VI:account-123
Value: 00000000000000000005

Key:   VI:account-456
Value: 00000000000000000002
```

**Query Pattern**:
```
GetStreamVersion("account-123"):
1. Get: VI:account-123
2. Parse position value (or return -1 if not exists)
```

---

### 6. Global Position Counter

**Purpose**: Generate monotonically increasing global positions per namespace

**Key Structure**:
```
GP
```
(Single key per namespace DB)

**Value**:
```
{next_global_position_20}
```

**Example** (in namespace "myapp"'s Pebble DB):
```
Key:   GP
Value: 00000000000000001004
```

**Write Transaction Pattern**:
```
WriteMessage(streamName="account-123", msg):
// All operations in namespace "myapp"'s Pebble DB

1. Get current version: VI:account-123 → position=5 (or -1)
2. Check expected version (optimistic locking)
3. Increment & get: GP → globalPosition=1004
4. nextPosition = currentPosition + 1 = 6
5. Batch write:
   - M:00000000000000001004 = {message with position=6, globalPosition=1004}
   - SI:account-123:00000000000000000006 = 00000000000000001004
   - CI:account:00000000000000001004 = account-123
   - VI:account-123 = 00000000000000000006
   - GP = 00000000000000001005 (incremented)
6. Return WriteResult{position=6, globalPosition=1004}
```

---

## Additional Indexes (Optional Optimizations)

### 7. Type Index (Optional - for GetLastStreamMessage with type filter)

**Key Structure**:
```
TI:{stream_name}:{type}:{position_20_desc}
```

Where `position_20_desc` is inverted (99999999999999999999 - position) for reverse ordering.

**Value**:
```
{global_position_20}
```

**Example** (in namespace "myapp"'s Pebble DB):
```
Key:   TI:account-123:Deposited:99999999999999999994
Value: 00000000000000001005
```

**Query Pattern**:
```
GetLastStreamMessage("account-123", type="Deposited"):
1. Range scan: TI:account-123:Deposited: (prefix scan)
2. Take first key (highest position due to desc ordering)
3. Lookup message: M:{global_position}
```

**Alternative**: Without this index, scan Stream Index in reverse (depends on KV store reverse iteration support).

---

### 8. Correlation Index (Optional - for efficient correlation filtering)

**Key Structure**:
```
CCI:{category}:{correlation_category}:{global_position_20}
```

**Value**:
```
{stream_name}
```

**Example** (in namespace "myapp"'s Pebble DB):
```
Key:   CCI:account:workflow:00000000000000001005
Value: account-123
```

**Trade-off**: 
- Adds storage overhead
- Eliminates application-layer filtering for correlation queries
- Only needed if correlation queries are frequent

---

## Namespace Isolation Strategy

### Separate Pebble DB Instances (Chosen Approach)

**Architecture**: Each namespace gets its own Pebble database instance in a separate directory.

**Pros**:
- ✅ **True Physical Isolation**: No namespace prefix overhead in keys
- ✅ **Shorter Keys**: Saves 10-30 bytes per key (significant with billions of messages)
- ✅ **Independent Performance**: One namespace can't affect another's performance
- ✅ **Easy Deletion**: Just close & remove directory
- ✅ **Matches SQLite Pattern**: Consistent with current implementation
- ✅ **Better Compaction**: Each namespace's LSM tree optimized independently

**Cons**:
- More complex resource management
- Higher memory overhead per namespace (~10-50MB per Pebble instance)
- File descriptor limits (typically 1024-65536)

**Directory Structure**:
```
/data/eventodb/
├── _metadata/          # Metadata Pebble DB
│   ├── 000001.log
│   ├── MANIFEST
│   └── ...
├── myapp/             # Namespace "myapp" Pebble DB
│   ├── 000001.log
│   ├── MANIFEST
│   └── ...
└── production/        # Namespace "production" Pebble DB
    ├── 000001.log
    ├── MANIFEST
    └── ...
```

**Implementation**:
```go
type PebbleStore struct {
    metadataDB *pebble.DB                    // Namespace registry
    namespaces map[string]*namespaceHandle   // One DB per namespace
    dataDir    string
    mu         sync.RWMutex
}

type namespaceHandle struct {
    db      *pebble.DB
    writeMu sync.Mutex  // Serializes writes for GP counter
}

func (s *PebbleStore) getNamespaceDB(ns string) (*namespaceHandle, error) {
    s.mu.RLock()
    if handle, ok := s.namespaces[ns]; ok {
        s.mu.RUnlock()
        return handle, nil
    }
    s.mu.RUnlock()
    
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Double-check
    if handle, ok := s.namespaces[ns]; ok {
        return handle, nil
    }
    
    // Get namespace metadata
    nsMetadata := s.getNamespaceMetadata(ns)
    
    // Open Pebble DB
    dbPath := filepath.Join(s.dataDir, ns)
    db, err := pebble.Open(dbPath, &pebble.Options{
        // Optimized for write-heavy workloads
        MemTableSize: 64 << 20, // 64MB
        Cache:        pebble.NewCache(128 << 20), // 128MB per namespace
    })
    if err != nil {
        return nil, err
    }
    
    handle := &namespaceHandle{db: db}
    s.namespaces[ns] = handle
    return handle, nil
}
```

**Resource Management**:
- Lazy loading: Open Pebble DBs on-demand
- LRU eviction: Close least-recently-used namespaces when hitting limits
- Graceful shutdown: Close all DBs before exit

---

## Write Transaction Guarantees

### Atomicity Requirements

All writes must be atomic across multiple keys:

```
WriteMessage("account-123", msg) must atomically write:
1. M:myapp:{gp}         (message data)
2. SI:myapp:account-123:{pos}  (stream index)
3. CI:myapp:account:{gp}      (category index)
4. VI:myapp:account-123       (version update)
5. GP:myapp                   (counter increment)
```

### LSM KV Store Support

#### Pebble
```go
batch := db.NewBatch()
batch.Set(msgKey, msgValue, nil)
batch.Set(siKey, siValue, nil)
batch.Set(ciKey, ciValue, nil)
batch.Set(viKey, viValue, nil)
// Increment GP (read-modify-write needs mutex or optimistic)
batch.Commit(pebble.Sync)
```

#### BadgerDB
```go
err := db.Update(func(txn *badger.Txn) error {
    txn.Set(msgKey, msgValue)
    txn.Set(siKey, siValue)
    txn.Set(ciKey, ciValue)
    txn.Set(viKey, viValue)
    // Increment GP within transaction
    return nil
})
```

### Global Position Generation

**Challenge**: Auto-increment requires read-modify-write atomicity

**Solution 1**: Mutex per namespace (Chosen - matches SQLite pattern)
```go
type namespaceHandle struct {
    db      *pebble.DB
    writeMu sync.Mutex
}

func (s *Store) WriteMessage(ns, stream string, msg *Message) (*WriteResult, error) {
    handle := s.getNamespaceDB(ns)
    handle.writeMu.Lock()
    defer handle.writeMu.Unlock()
    
    // Read current GP from key "GP"
    currentGP := s.getGlobalPosition(handle.db)
    nextGP := currentGP + 1
    
    // Create atomic batch write
    batch := handle.db.NewBatch()
    // ... write M:, SI:, CI:, VI: keys
    batch.Set([]byte("GP"), encodeInt64(nextGP), pebble.Sync)
    return batch.Commit(pebble.Sync)
}
```

**Solution 2**: Optimistic locking with retry
```go
for retries := 0; retries < 10; retries++ {
    currentGP := getGlobalPosition(ns)
    nextGP := currentGP + 1
    
    batch := ...
    if compareAndSwap(GP_key, currentGP, nextGP) {
        batch.Commit()
        return
    }
    // Retry
}
```

**Solution 3**: Separate sequence generator (if KV supports)
```go
// Some KV stores have built-in sequences
nextGP := db.GetSequence("GP:" + ns).Next()
```

**Recommendation**: Solution 1 (Mutex) - Simple, matches SQLite implementation, adequate for most workloads.

---

## Query Performance Analysis

### GetStreamMessages
```
Query: Get 100 messages from "account-123" starting at position 50

Steps (in namespace "myapp"'s Pebble DB):
1. Range scan: SI:account-123:00000000000000000050 to ...00000000000000000149
   - Reads 100 keys from Stream Index (~8KB)
2. Point lookups: M:{gp} for each global position
   - 100 point lookups from Message store (~100-500KB depending on message size)

Complexity: O(batchSize * log N)
Optimization: Could store message data directly in SI value to avoid point lookups
```

### GetCategoryMessages
```
Query: Get 100 messages from "account" starting at global position 1000

Steps (in namespace "myapp"'s Pebble DB):
1. Range scan: CI:account:00000000000000001000 to ...
   - Reads 100 keys from Category Index (~7KB)
2. Point lookups: M:{gp} for each
   - 100 point lookups from Message store (~100-500KB)

Complexity: O(batchSize * log N)
```

### GetCategoryMessages with Consumer Group
```
Query: Get 100 messages from "account" for consumer 0/4

Steps (in namespace "myapp"'s Pebble DB):
1. Range scan: CI:account:00000000000000001000 to ...
   - Must read MORE than 100 to filter to 100 matching messages
   - Average: 4x batch size reads (for size=4) = ~400 keys (~28KB)
2. For each stream_name in value:
   - Hash cardinalID
   - Filter by modulo (hash % 4 == 0)
3. Point lookups for ~100 matching messages (~100-500KB)

Complexity: O(consumerSize * batchSize * log N)
Optimization: Pre-compute consumer assignment in value or separate index
```

### GetLastStreamMessage
```
Query: Get last message from "account-123"

Steps (in namespace "myapp"'s Pebble DB):
1. Point lookup: VI:account-123 → latest position
2. Point lookup: SI:account-123:{pos} → global position
3. Point lookup: M:{gp} → message data

Complexity: O(log N)
```

### GetLastStreamMessage with Type Filter
```
Query: Get last "Deposited" message from "account-123"

Without Type Index (in namespace "myapp"'s Pebble DB):
1. Reverse range scan: SI:account-123: (start from end)
2. Point lookup each message until type matches
3. Worst case: O(stream_length * log N)

With Type Index:
1. Prefix scan: TI:account-123:Deposited: (first key)
2. Point lookup: M:{gp}
3. Complexity: O(log N)
```

---

## Storage Overhead Analysis

### Per Message Storage

**Primary Data** (in namespace Pebble DB):
```
M:{gp_20}  →  {full_message_json}
Size: ~22 bytes key + message size (100-1000 bytes typical)
```

**Indexes** (per message):
```
SI:{stream}:{pos_20}  →  {gp_20}     (~40 bytes key + 20 bytes value)
CI:{cat}:{gp_20}      →  {stream}    (~35 bytes key + 20 bytes value)
VI:{stream}           →  {pos_20}    (~25 bytes key + 20 bytes value - updated, not added)
```

**Total per message**: ~140-160 bytes overhead + message size

**Savings vs SQL approach**: ~50-90 bytes per message by eliminating namespace prefix
- For 100M messages: **5-9 GB saved**
- For 1B messages: **50-90 GB saved**

### Comparison with SQL

**SQL Storage** (single table):
```
Table: messages (id, stream_name, type, position, global_position, data, metadata, time)
Indexes: 
- PRIMARY KEY (global_position)
- UNIQUE (id)
- UNIQUE (stream_name, position)
- INDEX category(substr(stream_name), global_position)

Overhead: Similar (indexes stored separately)
```

**Verdict**: 
- KV store overhead is comparable to SQL indexes
- **Separate DBs per namespace provide 30-40% key size reduction**
- LSM compaction provides better compression over time
- No namespace prefix overhead in hot query paths

---

## Implementation Checklist

### Phase 1: Core Implementation
- [ ] Define key encoding/decoding functions
- [ ] Implement namespace isolation (Option 1: Key Prefix)
- [ ] Implement WriteMessage with atomicity
- [ ] Implement GetStreamMessages
- [ ] Implement GetCategoryMessages
- [ ] Implement GetStreamVersion
- [ ] Implement GetLastStreamMessage

### Phase 2: Namespace Management
- [ ] CreateNamespace / DeleteNamespace
- [ ] GetNamespace / ListNamespaces
- [ ] Namespace token authentication

### Phase 3: Advanced Features
- [ ] Consumer group filtering
- [ ] Correlation filtering
- [ ] Type-filtered last message
- [ ] Optimistic locking (expected version)

### Phase 4: Performance Optimization
- [ ] Batch read optimizations
- [ ] Prefetch for sequential scans
- [ ] Caching layer (if needed)
- [ ] Compaction strategies

### Phase 5: Testing
- [ ] Unit tests for key encoding
- [ ] Integration tests (reuse existing SQL tests)
- [ ] Performance benchmarks vs SQL
- [ ] Consistency verification tools

---

## Recommended Implementation: Pebble

### Why Pebble?

1. **Pure Go**: No CGo dependencies
2. **Production-Ready**: Used by CockroachDB
3. **Rich Feature Set**:
   - Batch writes (atomic)
   - Range scans (forward/reverse)
   - Snapshots for consistent reads
   - Compaction control
   - Bloom filters for point lookups
4. **Performance**: Comparable to RocksDB
5. **Active Development**: Well-maintained

### Sample Code Structure

```go
package kvstore

import (
    "context"
    "encoding/json"
    "fmt"
    "path/filepath"
    "sync"
    
    "github.com/cockroachdb/pebble"
)

type PebbleStore struct {
    metadataDB *pebble.DB                    // Namespace registry
    namespaces map[string]*namespaceHandle   // One Pebble DB per namespace
    dataDir    string
    mu         sync.RWMutex
}

type namespaceHandle struct {
    db      *pebble.DB
    writeMu sync.Mutex  // Serializes writes for GP counter atomicity
}

func New(dataDir string) (*PebbleStore, error) {
    // Open metadata DB
    metadataDB, err := pebble.Open(filepath.Join(dataDir, "_metadata"), nil)
    if err != nil {
        return nil, err
    }
    
    return &PebbleStore{
        metadataDB: metadataDB,
        namespaces: make(map[string]*namespaceHandle),
        dataDir:    dataDir,
    }, nil
}

func (s *PebbleStore) getNamespaceDB(ns string) (*namespaceHandle, error) {
    // Fast path with read lock
    s.mu.RLock()
    if handle, ok := s.namespaces[ns]; ok {
        s.mu.RUnlock()
        return handle, nil
    }
    s.mu.RUnlock()
    
    // Slow path with write lock
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Double-check
    if handle, ok := s.namespaces[ns]; ok {
        return handle, nil
    }
    
    // Verify namespace exists in metadata
    nsKey := formatKey("NS", ns)
    _, closer, err := s.metadataDB.Get(nsKey)
    if err != nil {
        return nil, ErrNamespaceNotFound
    }
    closer.Close()
    
    // Open namespace Pebble DB
    dbPath := filepath.Join(s.dataDir, ns)
    db, err := pebble.Open(dbPath, &pebble.Options{
        Cache:        pebble.NewCache(128 << 20), // 128MB per namespace
        MemTableSize: 64 << 20,                   // 64MB memtable
    })
    if err != nil {
        return nil, fmt.Errorf("failed to open namespace DB: %w", err)
    }
    
    handle := &namespaceHandle{db: db}
    s.namespaces[ns] = handle
    return handle, nil
}

func (s *PebbleStore) WriteMessage(ctx context.Context, ns, stream string, msg *Message) (*WriteResult, error) {
    handle, err := s.getNamespaceDB(ns)
    if err != nil {
        return nil, err
    }
    
    // Serialize writes to this namespace
    handle.writeMu.Lock()
    defer handle.writeMu.Unlock()
    
    // Generate keys (no namespace prefix!)
    viKey := formatKey("VI", stream)
    gpKey := []byte("GP")
    
    // Get current stream version
    currentPos := s.getStreamVersionFromDB(handle.db, stream) // -1 if not exists
    
    // Check optimistic locking
    if msg.ExpectedVersion != nil && *msg.ExpectedVersion != currentPos {
        return nil, ErrVersionConflict
    }
    
    // Get and increment global position
    currentGP := s.getGlobalPosition(handle.db)
    nextGP := currentGP + 1
    nextPos := currentPos + 1
    
    // Prepare message
    msg.Position = nextPos
    msg.GlobalPosition = nextGP
    if msg.ID == "" {
        msg.ID = uuid.New().String()
    }
    
    // Build atomic batch
    batch := handle.db.NewBatch()
    defer batch.Close()
    
    // Write message data
    msgKey := formatKey("M", fmt.Sprintf("%020d", nextGP))
    msgValue, _ := json.Marshal(msg)
    batch.Set(msgKey, msgValue, pebble.Sync)
    
    // Write stream index
    siKey := formatKey("SI", stream, fmt.Sprintf("%020d", nextPos))
    siValue := []byte(fmt.Sprintf("%020d", nextGP))
    batch.Set(siKey, siValue, pebble.Sync)
    
    // Write category index
    category := Category(stream)
    ciKey := formatKey("CI", category, fmt.Sprintf("%020d", nextGP))
    ciValue := []byte(stream)
    batch.Set(ciKey, ciValue, pebble.Sync)
    
    // Update version index
    viValue := []byte(fmt.Sprintf("%020d", nextPos))
    batch.Set(viKey, viValue, pebble.Sync)
    
    // Update global position counter
    gpValue := []byte(fmt.Sprintf("%020d", nextGP))
    batch.Set(gpKey, gpValue, pebble.Sync)
    
    // Commit atomically
    if err := batch.Commit(pebble.Sync); err != nil {
        return nil, err
    }
    
    return &WriteResult{Position: nextPos, GlobalPosition: nextGP}, nil
}

func formatKey(parts ...string) []byte {
    return []byte(strings.Join(parts, ":"))
}

func (s *PebbleStore) getGlobalPosition(db *pebble.DB) int64 {
    value, closer, err := db.Get([]byte("GP"))
    if err != nil {
        return 0 // Start from 1
    }
    defer closer.Close()
    
    gp, _ := strconv.ParseInt(string(value), 10, 64)
    return gp
}
```

---

## Design Decisions Summary

1. **Namespace Isolation**: ✅ Separate Pebble DB per namespace
   - **Rationale**: Eliminates namespace prefix overhead (30-40% key size reduction)

2. **Type Index**: Add in Phase 3 if needed
   - **Rationale**: Reverse iteration is fast enough for most cases

3. **Correlation Index**: Application-layer filtering initially
   - **Rationale**: Matches SQL implementation, add index if needed later

4. **Message Data in Index**: Keep indexes separate
   - **Rationale**: Keep indexes small, point lookups are fast in LSM

5. **Reverse Iteration**: Use Pebble native reverse iteration
   - **Rationale**: More efficient than inverted keys

6. **Compression**: ✅ Enable Pebble default (Snappy)
   - **Rationale**: Good balance of speed and compression ratio

7. **Write Serialization**: ✅ Mutex per namespace
   - **Rationale**: Simple, matches SQLite implementation, adequate performance

8. **Resource Management**: Lazy load + LRU eviction
   - **Rationale**: Handle many namespaces without excessive memory

---

## Conclusion

This key structure design provides:

✅ **Full API Compatibility**: All existing operations supported  
✅ **Physical Namespace Isolation**: Separate Pebble DB per namespace  
✅ **Efficient Storage**: 30-40% smaller keys (no namespace prefix)  
✅ **Efficient Range Scans**: Leverages LSM tree strengths  
✅ **Atomicity**: Batch writes ensure consistency  
✅ **Performance**: Better than SQL for sequential reads (LSM optimization)  
✅ **Scalability**: Independent LSM trees per namespace  

### Key Benefits vs SQL

| Aspect | Pebble (Separate DBs) | SQL (Single Table) |
|--------|----------------------|-------------------|
| Key size | 20-40 bytes | N/A (row storage) |
| Namespace isolation | Physical (separate DBs) | Logical (WHERE clause) |
| Sequential reads | Excellent (LSM optimized) | Good (B-tree) |
| Write amplification | Low (LSM compaction) | Medium (B-tree splits) |
| Storage overhead | ~140-160 bytes/msg | ~200-250 bytes/msg |
| Scalability | Per-namespace | Shared resources |

### Storage Savings Example

For **1 billion messages** across 100 namespaces:
- **Key size savings**: 50-90 GB (no namespace prefix)
- **Better compression**: 10-20% additional savings from LSM compaction
- **Total estimated savings**: 60-110 GB vs key-prefix approach

### Next Steps

1. **Implement Core**: WriteMessage + GetStreamMessages + GetCategoryMessages
2. **Implement Namespace Ops**: CreateNamespace, DeleteNamespace, GetNamespace, ListNamespaces
3. **Add Advanced Features**: GetLastStreamMessage, GetStreamVersion, optimistic locking
4. **Test**: Reuse existing integration tests (same Store interface)
5. **Benchmark**: Compare performance characteristics with SQLite/Postgres implementations
6. **Resource Tuning**: Implement LRU eviction for namespace handles if needed
