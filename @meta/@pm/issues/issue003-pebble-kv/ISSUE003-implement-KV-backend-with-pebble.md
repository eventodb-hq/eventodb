# ISSUE003: Implement KV Backend with Pebble

**Status**: Open  
**Priority**: High  
**Type**: Feature Implementation  
**Created**: 2024-12-19  
**Estimated Effort**: 3-5 days  

---

## Overview

Implement a Pebble-based key-value store backend for MessageDB to enable high-performance event sourcing with physical namespace isolation. This implementation will complement the existing SQL backends (SQLite/Postgres) and provide better performance for write-heavy workloads.

**Related Documents**:
- `docs/KV_STORE_DESIGN.md` - Detailed design specification
- `docs/KV_STORE_KEY_SCHEMA.md` - Key schema quick reference

---

## Critical Sequencing Requirements

**⚠️ IMPORTANT**: Phases MUST be executed in order due to strict dependencies:

1. **Phase 0 (Key Encoding)** - Foundation for all other phases
   - No dependencies
   - Used by all subsequent phases
   - Must be complete and tested first

2. **Phase 1 (Metadata & Namespaces)** - Establishes namespace infrastructure
   - Depends on: Phase 0 (keys.go)
   - **Why first**: Namespaces must exist before any message operations
   - **What it does**: Creates metadata DB, implements namespace CRUD
   - **Deliverable**: Can create/list/delete namespaces (no messages yet)

3. **Phase 2 (Namespace DB + Writes)** - Connects metadata to actual Pebble DBs
   - Depends on: Phase 0 (keys.go) + Phase 1 (namespace must exist)
   - **Why second**: Must verify namespace exists before opening its DB
   - **What it does**: Lazy load namespace DBs, implement WriteMessage
   - **Deliverable**: Can write messages to namespaces

4. **Phase 3 (Read Operations)** - Implements queries
   - Depends on: Phase 2 (need data to read for testing)
   - **What it does**: Implement all Get* methods
   - **Deliverable**: Full CRUD cycle works

5. **Phase 4 (Resource Management)** - OPTIONAL optimization
   - Depends on: Phase 1-3 (optimization layer)
   - **Can be deferred**: Not required for correctness
   - **What it does**: LRU eviction for namespace handles

6. **Phase 5 (Testing)** - Comprehensive validation
   - Depends on: Phase 0-3 minimum (Phase 4 optional)
   - **What it does**: Integration tests, benchmarks, validation

**Common Mistake**: Trying to implement WriteMessage before namespace infrastructure exists  
**Why it fails**: WriteMessage needs getNamespaceDB() which needs metadata DB which needs namespace to exist

**Bottom Line**: Follow the sequence strictly. Each phase builds on the previous.

### Quick Sanity Check

Before starting each phase, verify:

- [ ] **Before Phase 0**: No prerequisites
- [ ] **Before Phase 1**: Phase 0 complete (keys.go compiles and tests pass)
- [ ] **Before Phase 2**: Phase 1 complete (can CreateNamespace, GetNamespace, ListNamespaces)
- [ ] **Before Phase 3**: Phase 2 complete (can WriteMessage successfully)
- [ ] **Before Phase 4**: Phase 3 complete (can read messages written in Phase 2)
- [ ] **Before Phase 5**: Phase 0-3 complete (full CRUD cycle works)

### Testing Strategy: Test Continuously!

**⚠️ CRITICAL**: Testing is NOT just Phase 5 - it happens throughout development!

| Phase | QA Command | Must Pass Before Next Phase |
|-------|-----------|----------------------------|
| **Phase 0** | `bin/qa_check.sh` | ✅ Unit tests for keys.go |
| **Phase 1** | `bin/qa_check.sh` | ✅ Unit tests for namespace ops |
| **Phase 2** | `bin/qa_check.sh` | ✅ Unit tests for write ops |
| **Phase 3** | `bin/qa_check.sh` + `bin/run_external_tests_pebble.sh` | ✅ All tests + external integration |
| **Phase 4** | `bin/qa_check.sh` + `bin/run_external_tests_pebble.sh` | ✅ No regression |
| **Phase 5** | Full validation suite | ✅ Production-ready |

**Golden Rule**: If `bin/qa_check.sh` doesn't pass, DO NOT proceed to next phase!

**External Tests**: Create `bin/run_external_tests_pebble.sh` in Phase 3, run it from Phase 3 onwards

---

## Goals

1. ✅ **Full API Compatibility**: Implement complete `Store` interface
2. ✅ **Physical Isolation**: Separate Pebble DB instance per namespace
3. ✅ **Atomicity**: Ensure all multi-key writes are atomic
4. ✅ **Performance**: Match or exceed SQL backend performance
5. ✅ **Resource Efficiency**: Lazy loading + LRU eviction for namespace DBs
6. ✅ **Storage Efficiency**: 30-40% key size reduction vs prefix-based approach

---

## Architecture

### Namespace Isolation Strategy

**Chosen Approach**: Separate Pebble DB per namespace

```
/data/messagedb/
├── _metadata/          # Namespace registry (Pebble DB)
│   ├── 000001.log
│   ├── MANIFEST
│   └── OPTIONS
├── production/         # Namespace "production" (Pebble DB)
│   ├── 000001.log
│   ├── MANIFEST
│   └── OPTIONS
└── staging/           # Namespace "staging" (Pebble DB)
    ├── 000001.log
    ├── MANIFEST
    └── OPTIONS
```

**Benefits**:
- No namespace prefix overhead in keys (30-40% size reduction)
- Physical isolation (security + independent performance)
- Easy namespace deletion (just remove directory)
- Independent LSM compaction per namespace
- **Estimated savings**: 50-90 GB per 1 billion messages

### Key Schema (Per Namespace DB)

All keys scoped to a single namespace's Pebble DB:

| Prefix | Key Format | Value | Purpose |
|--------|-----------|-------|---------|
| `M:` | `M:{gp_20}` | `{message_json}` | Message data (primary) |
| `SI:` | `SI:{stream}:{pos_20}` | `{gp_20}` | Stream index (stream+position→gp) |
| `CI:` | `CI:{category}:{gp_20}` | `{stream}` | Category index (category+gp→stream) |
| `VI:` | `VI:{stream}` | `{pos_20}` | Version index (stream→latest_position) |
| `GP` | `GP` | `{next_gp_20}` | Global position counter |

**Note**: `_20` suffix indicates zero-padded 20-digit integers for lexicographic ordering.

### Metadata DB Schema

Separate Pebble DB at `/data/messagedb/_metadata`:

| Key Format | Value | Purpose |
|-----------|-------|---------|
| `NS:{namespace_id}` | `{namespace_metadata_json}` | Namespace registry |

---

## Implementation Plan

### Dependency Graph & Critical Path

```
Phase 0: Key Encoding (foundational, no deps)
    ↓
Phase 1: Metadata DB + Namespace CRUD (needs Phase 0)
    ↓
Phase 2: Namespace DB Management + WriteMessage (needs Phase 0 + Phase 1)
    ↓
Phase 3: Read Operations (needs Phase 2 for test data)
    ↓
Phase 4: Resource Management (OPTIONAL optimization, needs Phase 1-3)
    ↓
Phase 5: Testing & Validation (needs Phase 0-3, optionally Phase 4)
```

**Critical Path**: 
1. ✅ Phase 0 → Phase 1 → Phase 2 → Phase 3 (MUST be sequential)
2. ⚠️ Phase 4 can be deferred or done in parallel with Phase 3
3. ✅ Phase 5 requires Phase 0-3 minimum

**Why This Sequence**:
- **Phase 0 first**: Key encoding used by all other phases
- **Phase 1 second**: Metadata DB is foundational - namespaces must exist before operations
- **Phase 2 third**: Connects metadata to actual Pebble DBs, implements writes
- **Phase 3 fourth**: Reads need data written in Phase 2
- **Phase 4 optional**: Optimization layer, not required for correctness
- **Phase 5 last**: Integration testing needs working implementation

---

### Phase 0: Key Encoding Foundation (Day 1 - Morning, ~2-3 hours)

**Files to Create**:
- `internal/store/pebble/keys.go` - Key encoding/decoding utilities
- `internal/store/pebble/utils.go` - Helper functions

**Dependencies**: None ✅

**Tasks**:

1. **Implement Key Encoding Functions**
   ```go
   // Key formatting
   formatKey(parts ...string) []byte           // Join parts with ":"
   formatMessageKey(gp int64) []byte           // M:{gp_20}
   formatStreamIndexKey(stream string, pos int64) []byte  // SI:{stream}:{pos_20}
   formatCategoryIndexKey(category string, gp int64) []byte  // CI:{category}:{gp_20}
   formatVersionIndexKey(stream string) []byte  // VI:{stream}
   formatGlobalPositionKey() []byte             // GP
   formatNamespaceKey(nsID string) []byte       // NS:{nsID}
   ```

2. **Implement Integer Encoding**
   ```go
   encodeInt64(n int64) []byte                  // Zero-pad to 20 digits
   decodeInt64(b []byte) (int64, error)         // Parse padded integer
   ```

3. **Implement Utilities**
   ```go
   extractCategory(stream string) string        // "account-123" → "account"
   extractCardinalID(stream string) string      // "account-123" → "123"
   hashCardinalID(cardinalID string) uint64     // Hash for consumer groups
   ```

**Acceptance Criteria**:
- [ ] All key formatting functions work correctly
- [ ] Integer encoding produces correct lexicographic ordering
- [ ] Category extraction handles edge cases (no dash, multiple dashes)
- [ ] 100% unit test coverage for all functions
- [ ] No dependencies on other packages (except stdlib)
- [ ] **bin/qa_check.sh passes** (go fmt, go vet, go test, go build)

**Testing** (MUST PASS BEFORE PHASE 1):
```bash
# Unit tests for keys.go
cd golang
go test ./internal/store/pebble -v -run TestKeys

# Example tests:
# - TestFormatMessageKey
# - TestEncodeInt64_LexicographicOrdering
# - TestExtractCategory
# - TestHashCardinalID
```

```go
// Test lexicographic ordering
keys := []string{
    formatMessageKey(1),
    formatMessageKey(10),
    formatMessageKey(100),
}
assert(sort.IsSorted(keys)) // Must be true
```

**QA Gate**: Run `bin/qa_check.sh` - MUST PASS before proceeding to Phase 1

---

### Phase 1: Metadata & Namespace Foundation (Day 1 - Afternoon, ~3-4 hours)

**Files to Create**:
- `internal/store/pebble/store.go` - Basic store struct and initialization
- `internal/store/pebble/namespace.go` - Namespace CRUD operations

**Dependencies**: Phase 0 (keys.go, utils.go) ✅

**Tasks**:

1. **Setup Basic Store Structure** (store.go)
   ```go
   type PebbleStore struct {
       metadataDB *pebble.DB      // CRITICAL: Namespace registry (opened first)
       dataDir    string           // Base directory
       mu         sync.RWMutex     // Protects namespaces map
       
       // NOTE: Namespace handles added in Phase 2
   }
   ```

2. **Initialize Store** (store.go)
   ```go
   func New(dataDir string) (*PebbleStore, error) {
       // 1. Create data directory if needed
       // 2. Open metadata DB at {dataDir}/_metadata
       // 3. Return store
   }
   
   func (s *PebbleStore) Close() error {
       // Close metadata DB
       // (Namespace DBs added in Phase 2)
   }
   ```

3. **Implement Namespace Operations** (namespace.go)
   
   **CRITICAL**: These operate ONLY on metadata DB
   
   ```go
   // Write namespace metadata to metadata DB
   func (s *PebbleStore) CreateNamespace(ctx context.Context, ns *Namespace) error {
       // 1. Validate namespace (ID, token)
       // 2. Hash token
       // 3. Create namespace directory: {dataDir}/{nsID}
       // 4. Write to metadata DB: NS:{nsID} → {namespace_json}
       // 5. Return success
       // NOTE: Does NOT open namespace Pebble DB yet (Phase 2)
   }
   
   // Read namespace metadata from metadata DB
   func (s *PebbleStore) GetNamespace(ctx context.Context, id string) (*Namespace, error) {
       // 1. Read from metadata DB: NS:{id}
       // 2. Deserialize JSON
       // 3. Return namespace
   }
   
   // List all namespaces from metadata DB
   func (s *PebbleStore) ListNamespaces(ctx context.Context) ([]*Namespace, error) {
       // 1. Range scan metadata DB with prefix "NS:"
       // 2. Deserialize each value
       // 3. Return list
   }
   
   // Delete namespace metadata and directory
   func (s *PebbleStore) DeleteNamespace(ctx context.Context, id string) error {
       // 1. Delete from metadata DB: NS:{id}
       // 2. Remove directory: {dataDir}/{id}
       // 3. Return success
       // NOTE: If namespace DB is open (Phase 2), close it first
   }
   ```

**Acceptance Criteria**:
- [ ] Metadata DB opens successfully on New()
- [ ] CreateNamespace writes to metadata DB correctly
- [ ] CreateNamespace creates namespace directory
- [ ] GetNamespace reads from metadata DB correctly
- [ ] ListNamespaces returns all namespaces
- [ ] DeleteNamespace removes metadata and directory
- [ ] Close() closes metadata DB cleanly
- [ ] Multiple namespaces can be created/retrieved
- [ ] Namespace token is hashed (not stored in plaintext)
- [ ] **bin/qa_check.sh passes** (all tests including Phase 0 + Phase 1)

**Testing** (MUST PASS BEFORE PHASE 2):
```bash
# Unit tests for namespace operations
cd golang
go test ./internal/store/pebble -v -run TestNamespace

# Example tests:
# - TestCreateNamespace
# - TestGetNamespace
# - TestListNamespaces
# - TestDeleteNamespace
# - TestNamespaceTokenHashing
# - TestMetadataDBPersistence (close + reopen)

# QA check (includes fmt, vet, test, race, build)
cd ..
bin/qa_check.sh
```

**Integration Testing**:
```go
// Test full namespace lifecycle
store := pebble.New("/tmp/test")
defer store.Close()

// Create
ns := &Namespace{ID: "test", Token: "secret123"}
store.CreateNamespace(ctx, ns)

// Read
retrieved := store.GetNamespace(ctx, "test")
assert(retrieved.ID == "test")
assert(retrieved.TokenHash != "secret123") // Hashed

// List
all := store.ListNamespaces(ctx)
assert(len(all) == 1)

// Delete
store.DeleteNamespace(ctx, "test")
assert(!dirExists("/tmp/test/test"))
```

**QA Gate**: Run `bin/qa_check.sh` - MUST PASS before proceeding to Phase 2

**Why This Order**:
1. ✅ Metadata DB is foundational - needed before ANY namespace operations
2. ✅ Namespace CRUD must work before message operations
3. ✅ Directory structure established early
4. ✅ No dependencies on message operations (not implemented yet)

---

### Phase 2: Namespace DB Management & Write Operations (Day 2)

**Dependencies**: Phase 0 + Phase 1 (keys + metadata + namespace CRUD) ✅

**Files to Create/Update**:
- `internal/store/pebble/write.go` - WriteMessage implementation
- `internal/store/pebble/store.go` - Add namespace handle management

**Tasks**:

1. **Add Namespace Handle Management** (store.go)
   
   **CRITICAL**: Now we connect metadata to actual Pebble DBs
   
   ```go
   type PebbleStore struct {
       metadataDB *pebble.DB                    // Metadata (Phase 1)
       namespaces map[string]*namespaceHandle   // NEW: Lazy-loaded namespace DBs
       dataDir    string
       mu         sync.RWMutex
   }
   
   type namespaceHandle struct {
       db      *pebble.DB      // Actual namespace Pebble DB
       writeMu sync.Mutex      // Serializes writes for GP counter
   }
   
   // Lazy load namespace Pebble DB
   func (s *PebbleStore) getNamespaceDB(ctx context.Context, nsID string) (*namespaceHandle, error) {
       // Fast path: check if already open
       s.mu.RLock()
       if handle, ok := s.namespaces[nsID]; ok {
           s.mu.RUnlock()
           return handle, nil
       }
       s.mu.RUnlock()
       
       // Slow path: open namespace DB
       s.mu.Lock()
       defer s.mu.Unlock()
       
       // Double-check (another goroutine might have opened it)
       if handle, ok := s.namespaces[nsID]; ok {
           return handle, nil
       }
       
       // Verify namespace exists in metadata DB
       _, err := s.GetNamespace(ctx, nsID)
       if err != nil {
           return nil, fmt.Errorf("namespace not found: %w", err)
       }
       
       // Open namespace Pebble DB
       dbPath := filepath.Join(s.dataDir, nsID)
       db, err := pebble.Open(dbPath, &pebble.Options{
           Cache:        pebble.NewCache(128 << 20),  // 128MB cache
           MemTableSize: 64 << 20,                    // 64MB memtable
       })
       if err != nil {
           return nil, fmt.Errorf("failed to open namespace DB: %w", err)
       }
       
       // Cache handle
       handle := &namespaceHandle{db: db}
       s.namespaces[nsID] = handle
       return handle, nil
   }
   ```
   
   **Update Close()**:
   ```go
   func (s *PebbleStore) Close() error {
       // Close all namespace DBs
       s.mu.Lock()
       for _, handle := range s.namespaces {
           handle.db.Close()
       }
       s.namespaces = nil
       s.mu.Unlock()
       
       // Close metadata DB
       return s.metadataDB.Close()
   }
   ```

2. **Implement WriteMessage** (write.go)
   ```go
   func (s *PebbleStore) WriteMessage(ctx context.Context, ns, stream string, msg *Message) (*WriteResult, error)
   ```

   **Steps**:
   - Get namespace handle (lazy load if needed)
   - Acquire write mutex (`handle.writeMu.Lock()`)
   - Get current stream version from `VI:{stream}` (or -1 if not exists)
   - Check optimistic locking (`expectedVersion` matches current)
   - Get and increment global position from `GP` key
   - Prepare message (set position, globalPosition, ID if empty)
   - Create atomic batch:
     - `M:{gp}` → full message JSON
     - `SI:{stream}:{position}` → global position
     - `CI:{category}:{gp}` → stream name
     - `VI:{stream}` → new position
     - `GP` → incremented global position
   - Commit batch with `pebble.Sync`
   - Release write mutex
   - Return `WriteResult{Position, GlobalPosition}`

2. **Handle Edge Cases**
   - Empty stream name → error
   - Invalid namespace → error
   - Version conflict → return specific error
   - Batch commit failure → rollback (batch discarded on error)

3. **Add Helper Functions**
   - `getStreamVersion(db *pebble.DB, stream string) int64` - Read from `VI:` or return -1
   - `getGlobalPosition(db *pebble.DB) int64` - Read from `GP` or return 0
   - `extractCategory(stream string) string` - Parse category from stream name

**Acceptance Criteria**:
- [ ] getNamespaceDB() lazy loads namespace Pebble DB on first access
- [ ] getNamespaceDB() verifies namespace exists in metadata DB
- [ ] getNamespaceDB() caches open handles (no duplicate opens)
- [ ] getNamespaceDB() is thread-safe (double-check locking works)
- [ ] Single message writes succeed
- [ ] Global position auto-increments correctly (starts at 1)
- [ ] Stream position auto-increments per stream (starts at 0)
- [ ] Optimistic locking rejects version conflicts
- [ ] All 5 keys written atomically (verified with concurrent writes)
- [ ] Concurrent writes to same stream serialize correctly
- [ ] Concurrent writes to different streams run in parallel
- [ ] Close() closes all namespace DBs + metadata DB
- [ ] **bin/qa_check.sh passes** (all tests including Phase 0-2)
- [ ] **NO external tests yet** (read operations needed for external tests)

**Testing** (MUST PASS BEFORE PHASE 3):
```bash
# Unit tests for write operations
cd golang
go test ./internal/store/pebble -v -run TestWrite

# Example tests:
# - TestGetNamespaceDB_LazyLoad
# - TestGetNamespaceDB_Caching
# - TestGetNamespaceDB_ThreadSafe
# - TestWriteMessage_SingleStream
# - TestWriteMessage_GlobalPosition
# - TestWriteMessage_OptimisticLocking
# - TestWriteMessage_Atomicity
# - TestWriteMessage_ConcurrentSameStream
# - TestWriteMessage_ConcurrentDifferentStreams

# QA check
cd ..
bin/qa_check.sh
```

**Integration Testing**:
```go
// Full write flow
store := pebble.New("/tmp/test")
defer store.Close()

// Create namespace
ns := &Namespace{ID: "test", Token: "secret"}
store.CreateNamespace(ctx, ns)

// Write message
msg := &Message{
    StreamName: "account-123",
    Type:       "AccountCreated",
    Data:       json.RawMessage(`{"name":"Alice"}`),
}
result := store.WriteMessage(ctx, "test", "account-123", msg)
assert(result.Position == 0)        // First message
assert(result.GlobalPosition == 1)  // First global position

// Verify all 5 keys exist in namespace DB
handle := store.getNamespaceDB(ctx, "test")
assert(keyExists(handle.db, "M:00000000000000000001"))
assert(keyExists(handle.db, "SI:account-123:00000000000000000000"))
assert(keyExists(handle.db, "CI:account:00000000000000000001"))
assert(keyExists(handle.db, "VI:account-123"))
assert(keyExists(handle.db, "GP"))
```

**Stress Testing**:
```bash
# Concurrent write test
go test ./internal/store/pebble -v -run TestWriteMessage_Concurrent_10k

# Multi-namespace test
go test ./internal/store/pebble -v -run TestWriteMessage_100Namespaces
```

**QA Gate**: Run `bin/qa_check.sh` - MUST PASS before proceeding to Phase 3

**Why This Order**:
1. ✅ Namespace metadata must exist before opening namespace DB
2. ✅ getNamespaceDB() connects Phase 1 (metadata) to Phase 2 (message ops)
3. ✅ WriteMessage is first message operation - foundational for reads
4. ✅ Can test full flow: CreateNamespace → WriteMessage → verify in DB

---

### Phase 3: Read Operations (Day 3)

**Dependencies**: Phase 2 (need WriteMessage to create test data) ✅

**Files to Create**:
- `internal/store/pebble/read.go` - Get* operations implementation

**Tasks**:

1. **Implement GetStreamMessages**
   ```go
   func (s *PebbleStore) GetStreamMessages(ctx context.Context, ns, stream string, 
       position int64, batchSize int, condition *Condition) ([]*Message, error)
   ```

   **Steps**:
   - Get namespace handle
   - Create range scan iterator:
     - Start: `SI:{stream}:{position_20}`
     - End: `SI:{stream}:99999999999999999999`
   - Collect up to `batchSize` global positions from values
   - For each global position:
     - Point lookup `M:{gp}` → message JSON
     - Deserialize message
   - Apply condition filters (if specified)
   - Return messages in order

2. **Implement GetCategoryMessages**
   ```go
   func (s *PebbleStore) GetCategoryMessages(ctx context.Context, ns, category string, 
       globalPosition int64, batchSize int, condition *Condition) ([]*Message, error)
   ```

   **Steps**:
   - Get namespace handle
   - Create range scan iterator:
     - Start: `CI:{category}:{globalPosition_20}`
     - End: `CI:{category}:99999999999999999999`
   - Handle consumer group filtering:
     - If `condition.ConsumerGroupMember != nil`:
       - Read more keys (up to `batchSize * consumerGroupSize`)
       - For each stream name in value:
         - Extract cardinal ID
         - Hash and modulo: `hash(cardinalID) % consumerGroupSize == consumerGroupMember`
         - Include if matches
       - Stop when `batchSize` matching messages collected
   - Handle correlation filtering:
     - If `condition.CorrelationStreamName != nil`:
       - Filter messages where `metadata.correlationStreamName` starts with correlation value
   - Point lookup messages: `M:{gp}` for each
   - Return messages in order

3. **Implement GetLastStreamMessage**
   ```go
   func (s *PebbleStore) GetLastStreamMessage(ctx context.Context, ns, stream string, 
       msgType *string) (*Message, error)
   ```

   **Without type filter**:
   - Get version: `VI:{stream}` → latest position
   - Get global position: `SI:{stream}:{position}` → gp
   - Get message: `M:{gp}` → message

   **With type filter**:
   - Create reverse iterator on `SI:{stream}:`
   - For each position (descending):
     - Get global position from value
     - Get message: `M:{gp}`
     - If `message.Type == msgType`: return message
   - Return nil if not found

4. **Implement GetStreamVersion**
   ```go
   func (s *PebbleStore) GetStreamVersion(ctx context.Context, ns, stream string) (int64, error)
   ```

   **Steps**:
   - Get namespace handle
   - Point lookup: `VI:{stream}` → position
   - Return position (or -1 if key doesn't exist)

**Acceptance Criteria**:
- [ ] GetStreamMessages returns correct messages in order
- [ ] GetCategoryMessages returns correct messages across streams
- [ ] Consumer group filtering distributes messages correctly
- [ ] Correlation filtering works correctly
- [ ] GetLastStreamMessage returns most recent message
- [ ] GetLastStreamMessage with type filter returns correct message
- [ ] GetStreamVersion returns correct position (or -1 for new stream)
- [ ] All reads handle empty results gracefully (empty slice, not error)
- [ ] All reads work with data written in Phase 2
- [ ] **bin/qa_check.sh passes** (all tests including Phase 0-3)
- [ ] **bin/run_external_tests_pebble.sh passes** (external integration tests)

**Testing** (MUST PASS BEFORE PHASE 4):
```bash
# Unit tests for read operations
cd golang
go test ./internal/store/pebble -v -run TestRead

# Example tests:
# - TestGetStreamMessages
# - TestGetStreamMessages_Pagination
# - TestGetCategoryMessages
# - TestGetCategoryMessages_ConsumerGroup
# - TestGetCategoryMessages_Correlation
# - TestGetLastStreamMessage
# - TestGetLastStreamMessage_WithType
# - TestGetStreamVersion

# QA check
cd ..
bin/qa_check.sh
```

**Integration Testing**:
```go
// Full CRUD cycle
store := pebble.New("/tmp/test")
defer store.Close()

// Setup
store.CreateNamespace(ctx, &Namespace{ID: "test", Token: "secret"})
for i := 0; i < 100; i++ {
    store.WriteMessage(ctx, "test", "account-123", &Message{...})
    store.WriteMessage(ctx, "test", "account-456", &Message{...})
}

// Read stream messages
msgs := store.GetStreamMessages(ctx, "test", "account-123", 0, 10, nil)
assert(len(msgs) == 10)
assert(msgs[0].Position == 0)
assert(msgs[9].Position == 9)

// Read category messages
msgs = store.GetCategoryMessages(ctx, "test", "account", 0, 50, nil)
assert(len(msgs) == 50)
// Should interleave account-123 and account-456

// Consumer group
msgs = store.GetCategoryMessages(ctx, "test", "account", 0, 100, &Condition{
    ConsumerGroupMember: 0,
    ConsumerGroupSize: 4,
})
// Verify distribution

// Last message
last := store.GetLastStreamMessage(ctx, "test", "account-123", nil)
assert(last.Position == 99)

// Stream version
version := store.GetStreamVersion(ctx, "test", "account-123")
assert(version == 99)
```

**External Integration Tests** (CRITICAL):

Create `bin/run_external_tests_pebble.sh`:
```bash
#!/bin/bash
set -e

PORT=6789
SERVER_BIN="./golang/messagedb"
TEST_DIR="./test_external"
DATA_DIR="/tmp/messagedb_pebble_test"
DEFAULT_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Cleanup
rm -rf "$DATA_DIR"
killall messagedb 2>/dev/null || true

# Start server with Pebble backend
$SERVER_BIN -port $PORT -db-url "pebble://$DATA_DIR" -token "$DEFAULT_TOKEN" &
SERVER_PID=$!

# Wait for ready
for i in {1..30}; do
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        break
    fi
    sleep 0.2
done

# Run external tests
cd $TEST_DIR
MESSAGEDB_URL="http://localhost:$PORT" bun test --max-concurrency=1
TEST_EXIT=$?

# Cleanup
kill $SERVER_PID
rm -rf "$DATA_DIR"

exit $TEST_EXIT
```

**Run External Tests**:
```bash
cd ..
bin/run_external_tests_pebble.sh
```

**QA Gate**: 
1. Run `bin/qa_check.sh` - MUST PASS
2. Run `bin/run_external_tests_pebble.sh` - MUST PASS
3. All existing external tests must pass (same as Postgres/SQLite)

**Why This Order**:
1. ✅ Reads need data - Phase 2 WriteMessage provides it
2. ✅ Can test full CRUD cycle
3. ✅ External tests validate API compatibility
4. ✅ Read operations don't depend on each other (can be implemented in any order)

---

### Phase 4: Resource Management & Optimization (Day 4)

**Dependencies**: Phase 1-3 (optimization layer on top of working implementation) ✅

**NOTE**: This phase is OPTIONAL for basic functionality. Core features work without it.

**Files to Create/Update**:
- `internal/store/pebble/resource.go` - LRU eviction logic
- `internal/store/pebble/store.go` - Update getNamespaceDB() with eviction

**Tasks**:

1. **Add LRU Tracking**
   ```go
   type namespaceHandle struct {
       db         *pebble.DB
       writeMu    sync.Mutex
       lastAccess time.Time  // Track for LRU eviction
   }
   ```

2. **Implement Eviction Policy**
   - Track number of open namespace DBs
   - When approaching limit (e.g., 100 open DBs):
     - Sort by last access time
     - Close least-recently-used DBs
     - Remove from `s.namespaces` map
   - Reopen on next access

3. **Configure Pebble Options**
   ```go
   &pebble.Options{
       Cache:                 pebble.NewCache(128 << 20),  // 128MB cache per namespace
       MemTableSize:          64 << 20,                    // 64MB memtable
       MaxConcurrentCompactions: 2,                        // 2 concurrent compactions
       Compression:           pebble.DefaultCompression,   // Snappy
       L0CompactionThreshold: 4,                           // Trigger compaction at 4 files
       L0StopWritesThreshold: 12,                          // Stop writes at 12 files
   }
   ```

4. **Add Resource Limits**
   - Max open namespaces (default: 100)
   - Max memory per namespace (via cache + memtable size)
   - Graceful degradation when limits hit

5. **Add Metrics/Logging**
   - Track number of open DBs
   - Log namespace open/close events
   - Track eviction count

**Acceptance Criteria**:
- [ ] LRU eviction closes least-used namespaces
- [ ] Evicted namespaces can be reopened automatically
- [ ] Resource limits prevent excessive memory usage
- [ ] Metrics track namespace lifecycle (open/close/evict counts)
- [ ] No resource leaks under load
- [ ] All Phase 1-3 tests still pass (no regression)
- [ ] **bin/qa_check.sh passes** (all tests including Phase 0-4)
- [ ] **bin/run_external_tests_pebble.sh passes** (no regression)

**Testing** (MUST PASS BEFORE PHASE 5):
```bash
# Unit tests for resource management
cd golang
go test ./internal/store/pebble -v -run TestResource

# Example tests:
# - TestLRUEviction_1000Namespaces
# - TestLRUEviction_Reopen
# - TestResourceLimits_MaxOpenNamespaces
# - TestNoFileDescriptorLeaks
# - TestNoMemoryLeaks

# QA check
cd ..
bin/qa_check.sh

# External tests (verify no regression)
bin/run_external_tests_pebble.sh
```

**Stress Testing**:
```go
// Access many namespaces to trigger eviction
store := pebble.New("/tmp/test")
defer store.Close()

// Create 1000 namespaces
for i := 0; i < 1000; i++ {
    ns := &Namespace{ID: fmt.Sprintf("ns%d", i), Token: "secret"}
    store.CreateNamespace(ctx, ns)
}

// Access all namespaces (should trigger eviction)
for i := 0; i < 1000; i++ {
    nsID := fmt.Sprintf("ns%d", i)
    store.WriteMessage(ctx, nsID, "stream-1", &Message{...})
}

// Verify eviction happened (fewer than 1000 open DBs)
assert(len(store.namespaces) < 1000)

// Verify reopening works
for i := 0; i < 100; i++ {
    nsID := fmt.Sprintf("ns%d", i)
    msgs := store.GetStreamMessages(ctx, nsID, "stream-1", 0, 10, nil)
    assert(len(msgs) == 1) // Message still there
}
```

**Resource Monitoring**:
```bash
# Monitor memory during stress test
go test ./internal/store/pebble -v -run TestLRUEviction_1000Namespaces -memprofile=/tmp/mem.prof

# Check for file descriptor leaks
lsof -p $(pgrep messagedb) | wc -l  # Should be bounded
```

**QA Gate**:
1. Run `bin/qa_check.sh` - MUST PASS
2. Run `bin/run_external_tests_pebble.sh` - MUST PASS (no regression)

**Why This Order**:
1. ✅ Resource management is an optimization, not required for correctness
2. ✅ Can be deferred if time is tight (Phase 5 tests work without it)
3. ✅ Easier to test eviction when full read/write functionality works
4. ⚠️ **Alternative**: Can be done in parallel with Phase 3 if developer is comfortable

---

### Phase 5: Comprehensive Testing & Validation (Day 5)

**Dependencies**: Phase 0-3 (REQUIRED) ✅  
**Optional**: Phase 4 (resource management) ⚠️

**CRITICAL**: This phase is about comprehensive validation, but **most tests should already be passing** from Phases 0-4!

**Tasks**:

1. **Verify All QA Checks Pass**
   ```bash
   # Must run clean
   bin/qa_check.sh
   ```
   - [ ] go fmt (no changes)
   - [ ] go vet (no warnings)
   - [ ] go test (all tests pass)
   - [ ] go test -race (no race conditions)
   - [ ] go build (compiles successfully)

2. **Verify External Integration Tests Pass**
   ```bash
   # Create bin/run_external_tests_pebble.sh (see Phase 3)
   bin/run_external_tests_pebble.sh
   ```
   - [ ] All external tests pass (same as Postgres/SQLite)
   - [ ] API compatibility verified
   - [ ] No regressions

3. **Run Existing Test Suites**
   
   **IMPORTANT**: Pebble backend must be compatible with existing tests!
   
   ```bash
   # All existing integration tests should pass
   cd golang
   go test ./internal/store/pebble -v
   
   # Run with coverage
   go test ./internal/store/pebble -coverprofile=/tmp/coverage.out
   go tool cover -html=/tmp/coverage.out
   
   # Target: >80% coverage
   ```

4. **Performance Benchmarks**
   ```bash
   # Compare with SQLite
   cd golang
   go test ./internal/store/pebble -bench=. -benchmem -benchtime=10s
   go test ./internal/store/sqlite -bench=. -benchmem -benchtime=10s
   
   # Compare results
   ```
   
   **Benchmarks to run**:
   - [ ] BenchmarkWriteMessage_SingleStream
   - [ ] BenchmarkWriteMessage_100Streams
   - [ ] BenchmarkGetStreamMessages_100
   - [ ] BenchmarkGetCategoryMessages_100
   - [ ] BenchmarkGetLastStreamMessage
   - [ ] BenchmarkGetStreamVersion

5. **Data Consistency Verification**
   ```bash
   # Run consistency tests
   go test ./internal/store/pebble -v -run TestConsistency
   ```
   
   **Tests**:
   - [ ] No duplicate global positions (10k concurrent writes)
   - [ ] No gaps in stream positions
   - [ ] Category index matches message data
   - [ ] Version index matches actual stream positions
   - [ ] GP counter never goes backwards

6. **Error Handling Validation**
   ```bash
   # Run error handling tests
   go test ./internal/store/pebble -v -run TestError
   ```
   
   **Tests**:
   - [ ] Invalid namespace returns proper error
   - [ ] Version conflicts return ErrVersionConflict
   - [ ] Disk full scenarios handled gracefully
   - [ ] Corrupt DB recovery (open with errors)
   - [ ] Concurrent access errors handled

7. **Storage Analysis**
   ```bash
   # Create test script to measure storage
   go test ./internal/store/pebble -v -run TestStorageOverhead
   ```
   
   **Measurements**:
   - [ ] Write 1M messages
   - [ ] Measure actual key sizes
   - [ ] Verify compression ratios (Snappy)
   - [ ] Compare with SQL backend
   - [ ] Verify overhead is 140-160 bytes per message

8. **Stress Testing**
   ```bash
   # Long-running stress test
   go test ./internal/store/pebble -v -run TestStress -timeout=30m
   ```
   
   **Tests**:
   - [ ] 1M messages across 100 streams
   - [ ] 10k concurrent writes (no failures)
   - [ ] 1000 namespaces (verify LRU if Phase 4 done)
   - [ ] No memory leaks (monitor for 10 minutes)
   - [ ] No file descriptor leaks

9. **Cross-Backend Compatibility**
   ```bash
   # Verify same behavior as SQL backends
   go test ./internal/store -v -run TestStoreInterface
   ```
   
   **Goal**: All backends (SQLite, Postgres, Pebble) pass same interface tests

**Acceptance Criteria**:
- [ ] **bin/qa_check.sh passes** (all checks green)
- [ ] **bin/run_external_tests_pebble.sh passes** (100% external tests)
- [ ] All unit tests pass (>80% coverage)
- [ ] All integration tests pass
- [ ] No data consistency issues under stress
- [ ] Performance meets targets (>10k writes/sec, <5ms reads)
- [ ] Error handling is robust (no panics, proper errors)
- [ ] Storage overhead within expected range
- [ ] No memory leaks or file descriptor leaks
- [ ] Cross-backend compatibility verified

**QA Gate (FINAL)**:
```bash
# Full validation suite
cd ..
bin/qa_check.sh && \
bin/run_external_tests_pebble.sh && \
echo "✅ All tests passed! Pebble backend is production-ready."
```

**Deliverables**:
1. All tests passing
2. Benchmark results documented
3. Coverage report (>80%)
4. Performance comparison with SQL backends
5. Storage analysis report
6. Updated documentation

---

## Performance Targets

### Write Performance
- **Target**: >10,000 writes/sec (single stream)
- **Target**: >50,000 writes/sec (100 streams, parallelized)
- **Latency**: <1ms p50, <5ms p99

### Read Performance
- **GetStreamMessages**: 1-5ms for 100 messages
- **GetCategoryMessages**: 1-5ms for 100 messages
- **GetLastStreamMessage**: <1ms
- **GetStreamVersion**: <0.5ms

### Storage Efficiency
- **Overhead**: 140-160 bytes per message (indexes)
- **Compression**: 30-50% reduction with Snappy
- **Key size savings**: 30-40% vs prefix-based approach

---

## File Structure & Build Order

**Build in this order** (matches phase sequence):

```
internal/store/pebble/

Phase 0 (Day 1 AM):
├── keys.go            # ✅ Key encoding/decoding utilities (NO dependencies)
└── utils.go           # ✅ Helper functions (NO dependencies)

Phase 1 (Day 1 PM):
├── store.go           # ✅ Basic store struct, metadata DB init (needs keys.go)
└── namespace.go       # ✅ Namespace CRUD on metadata DB (needs store.go, keys.go)

Phase 2 (Day 2):
├── store.go           # ⚠️ UPDATE: Add getNamespaceDB(), namespace handle map
└── write.go           # ✅ WriteMessage implementation (needs store.go, keys.go, utils.go)

Phase 3 (Day 3):
└── read.go            # ✅ All Get* operations (needs store.go, keys.go, utils.go)

Phase 4 (Day 4 - OPTIONAL):
├── resource.go        # ✅ LRU eviction logic (needs store.go)
└── store.go           # ⚠️ UPDATE: Add eviction to getNamespaceDB()

Phase 5 (Day 5):
internal/store/pebble/test/
├── keys_test.go           # Unit tests for key encoding
├── namespace_test.go      # Unit tests for namespace ops
├── write_test.go          # Unit tests for write logic
├── read_test.go           # Unit tests for read logic
├── integration_test.go    # Integration tests (reuse SQL test patterns)
├── concurrent_test.go     # Concurrency tests
├── benchmark_test.go      # Performance benchmarks
└── consistency_test.go    # Data consistency verification
```

**Key Files**:
- **keys.go**: Foundational, built first
- **store.go**: Built incrementally (Phase 1 basic, Phase 2 adds handles, Phase 4 adds eviction)
- **namespace.go**: Depends on keys.go + basic store.go
- **write.go**: Depends on store.go with namespace handle support
- **read.go**: Depends on write.go (for test data)

---

## Required Deliverables

### 1. External Test Script (Phase 3)

**File**: `bin/run_external_tests_pebble.sh`

**Purpose**: Run the same external integration tests that Postgres/SQLite backends use

**Template** (based on `bin/run_external_tests_postgres.sh`):
```bash
#!/bin/bash
#
# Run external tests against MessageDB server with Pebble backend
#

set -e

PORT=6789
SERVER_BIN="./golang/messagedb"
TEST_DIR="./test_external"
DATA_DIR="/tmp/messagedb_pebble_test"

# Known tokens - must match what tests expect
DEFAULT_TOKEN="ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000"

# Pebble DB URL
DB_URL="pebble://${DATA_DIR}"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${YELLOW}=== MessageDB External Tests (Pebble) ===${NC}"
echo ""
echo -e "${CYAN}Pebble Configuration:${NC}"
echo -e "  Data Directory: ${DATA_DIR}"
echo ""

# Cleanup old data
if [ -d "$DATA_DIR" ]; then
    echo -e "${YELLOW}Cleaning up old test data...${NC}"
    rm -rf "$DATA_DIR"
fi

# Kill any existing server
if lsof -ti:$PORT > /dev/null 2>&1; then
    echo -e "${YELLOW}Killing existing process on port $PORT...${NC}"
    kill $(lsof -ti:$PORT) 2>/dev/null || true
    sleep 1
fi

# Build server if needed
if [ ! -f "$SERVER_BIN" ]; then
    echo -e "${YELLOW}Building server...${NC}"
    cd golang && go build -o messagedb ./cmd/messagedb && cd ..
fi

# Start server with Pebble backend
echo -e "${YELLOW}Starting test server with Pebble backend...${NC}"
$SERVER_BIN -port $PORT -db-url "$DB_URL" -token "$DEFAULT_TOKEN" > /tmp/messagedb_pebble.log 2>&1 &
SERVER_PID=$!

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    kill $SERVER_PID 2>/dev/null || true
    rm -rf "$DATA_DIR"
}
trap cleanup EXIT

# Wait for server to be ready
echo -e "${YELLOW}Waiting for server...${NC}"
for i in {1..30}; do
    if curl -s http://localhost:$PORT/health > /dev/null 2>&1; then
        echo -e "${GREEN}Server ready!${NC}"
        sleep 0.5
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}Server failed to start. Check /tmp/messagedb_pebble.log${NC}"
        cat /tmp/messagedb_pebble.log
        exit 1
    fi
    sleep 0.2
done

# Run tests
echo -e "${YELLOW}Running tests...${NC}"
cd $TEST_DIR
MESSAGEDB_URL="http://localhost:$PORT" bun test --max-concurrency=1
TEST_EXIT_CODE=$?

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}All tests passed with Pebble backend!${NC}"
else
    echo -e "${RED}Tests failed!${NC}"
    echo -e "${YELLOW}Server logs:${NC}"
    cat /tmp/messagedb_pebble.log
fi

exit $TEST_EXIT_CODE
```

**Make executable**:
```bash
chmod +x bin/run_external_tests_pebble.sh
```

**Test it works**:
```bash
bin/run_external_tests_pebble.sh
```

**Expected**: All external tests pass (same as Postgres/SQLite)

---

## Dependencies

### External Libraries
- **Pebble**: `github.com/cockroachdb/pebble v1.1.0` (latest stable)
  - Pure Go, no CGO dependencies
  - Production-ready (used by CockroachDB)
  - Active development

### Internal Dependencies
- `internal/domain` - Message, Namespace types
- `internal/store` - Store interface

---

## Testing Strategy

### Unit Tests (Per Phase)
- Phase 1: Key encoding, namespace CRUD
- Phase 2: Write logic, version handling
- Phase 3: Read logic, filtering
- Phase 4: LRU eviction, resource limits

### Integration Tests (Phase 5)
- Reuse existing SQL backend integration tests
- Verify full Store interface compatibility
- Test error scenarios

### Performance Tests (Phase 5)
- Benchmark all operations
- Compare with SQLite/Postgres backends
- Identify bottlenecks

### Stress Tests (Phase 5)
- 10k concurrent writes
- 1M messages across 100 streams
- 1000+ namespace opens (test LRU)

---

## Migration Considerations

### Future: SQL to Pebble Migration Tool

Not in scope for this issue, but plan for:

1. **Export from SQL**
   - Read all messages ordered by global position
   - Preserve stream positions, metadata

2. **Import to Pebble**
   - Recreate namespace
   - Write messages in batch (preserve positions)
   - Verify consistency

3. **Validation**
   - Compare message counts
   - Verify stream versions match
   - Verify global position ranges

---

## Risk Assessment

### High Risk
- **Atomicity of GP counter**: Mitigated by write mutex per namespace
- **Resource limits**: Mitigated by LRU eviction
- **Pebble compaction stalls**: Mitigated by tuned compaction thresholds

### Medium Risk
- **Performance regression vs SQL**: Benchmark in Phase 5, optimize if needed
- **Storage overhead larger than expected**: Monitor in Phase 5, may need index optimization

### Low Risk
- **Pebble bugs**: Mature library, used in production by CockroachDB
- **Key encoding errors**: Extensive unit tests in Phase 1

---

## Success Criteria

### Functionality
- ✅ All Store interface methods implemented
- ✅ Full API compatibility with SQL backends
- ✅ Physical namespace isolation
- ✅ Atomic multi-key writes

### Performance
- ✅ Write throughput >10k/sec (single stream)
- ✅ Read latency <5ms (100 messages)
- ✅ No performance degradation under concurrent load

### Reliability
- ✅ No data loss under concurrent writes
- ✅ No consistency issues (verified by tests)
- ✅ Graceful error handling

### Efficiency
- ✅ Storage overhead 140-160 bytes per message
- ✅ Key size reduction 30-40% vs prefix approach
- ✅ Memory usage within limits (LRU eviction working)

---

## Future Enhancements (Out of Scope)

### Optional Indexes (Phase 6+)
1. **Type Index**: `TI:{stream}:{type}:{pos_desc}` → `{gp}`
   - For frequent type-filtered last message queries
   - Trade-off: Additional storage vs faster queries

2. **Correlation Index**: `CCI:{category}:{correlation}:{gp}` → `{stream}`
   - For efficient correlation filtering
   - Trade-off: Additional storage vs application-layer filtering

3. **Consumer Assignment Cache**: Pre-compute hash mod in CI value
   - Avoid application-layer filtering
   - Trade-off: Larger index values vs faster queries

### Performance Optimizations
- Batch read optimization (prefetch multiple M: keys)
- Message data in SI: value (avoid point lookups)
- Bloom filter tuning
- Custom compaction strategy

### Monitoring
- Prometheus metrics export
- Compaction statistics
- Write amplification tracking
- Read/write latency histograms

---

## Documentation Updates

### Files to Update
- `README.md` - Add Pebble backend option
- `docs/ARCHITECTURE.md` - Document Pebble implementation
- `docs/DEPLOYMENT.md` - Add Pebble configuration options
- `docs/BENCHMARKS.md` - Add Pebble performance comparison

### New Documentation
- `docs/KV_STORE_MIGRATION.md` - SQL to Pebble migration guide
- `docs/KV_STORE_TUNING.md` - Pebble tuning guide

---

## Timeline

| Phase | Duration | Deliverables | Dependencies |
|-------|----------|--------------|--------------|
| **Phase 0**: Key Encoding | Day 1 AM (2-3h) | Key encoding/decoding, utilities | None |
| **Phase 1**: Metadata & Namespaces | Day 1 PM (3-4h) | Metadata DB, namespace CRUD | Phase 0 |
| **Phase 2**: Namespace DB + Writes | Day 2 (6-8h) | getNamespaceDB(), WriteMessage | Phase 0, 1 |
| **Phase 3**: Read Operations | Day 3 (6-8h) | All Get* methods | Phase 0, 1, 2 |
| **Phase 4**: Resource Management | Day 4 (4-6h) | LRU eviction (OPTIONAL) | Phase 0, 1, 2, 3 |
| **Phase 5**: Testing & Validation | Day 5 (6-8h) | Integration tests, benchmarks | Phase 0-3 (min) |

**Total Estimated Effort**: 3-5 days (single developer)

**Minimum Viable Implementation**: Phase 0 + 1 + 2 + 3 = 2-3 days  
**Full Implementation**: Phase 0 + 1 + 2 + 3 + 4 + 5 = 4-5 days

**Critical Dependencies**:
- ❗ Phase 0 must complete before Phase 1
- ❗ Phase 1 must complete before Phase 2 (namespace must exist before writes)
- ❗ Phase 2 must complete before Phase 3 (need data to read)
- ⚠️ Phase 4 can be deferred (optimization only)

---

## Definition of Done

### Code Complete
- [ ] All Store interface methods implemented
- [ ] All phases 0-3 complete (Phase 4 optional but recommended)
- [ ] No compilation errors or warnings
- [ ] Code follows Go best practices (fmt, vet)

### Testing Complete
- [ ] **bin/qa_check.sh passes** (go fmt, vet, test, race, build)
- [ ] **bin/run_external_tests_pebble.sh created and passing**
- [ ] All unit tests passing (>80% coverage minimum)
- [ ] All integration tests passing
- [ ] All external tests passing (same tests as Postgres/SQLite)
- [ ] Performance benchmarks run and documented
- [ ] Stress tests pass (no leaks, no consistency issues)

### Quality Gates
- [ ] No data consistency issues under concurrent load
- [ ] No race conditions (go test -race passes)
- [ ] No memory leaks (verified with profiling)
- [ ] No file descriptor leaks (verified with lsof)
- [ ] Error handling is robust (no panics)
- [ ] Storage overhead within expected range (140-160 bytes per message)
- [ ] Performance meets or exceeds targets:
  - [ ] Write throughput >10k/sec (single stream)
  - [ ] Read latency <5ms (100 messages)
  - [ ] No performance degradation under load

### Documentation Complete
- [ ] README.md updated with Pebble backend option
- [ ] docs/ARCHITECTURE.md documents Pebble implementation
- [ ] docs/DEPLOYMENT.md includes Pebble configuration
- [ ] Benchmark results documented
- [ ] Performance comparison with SQL backends documented
- [ ] Code comments explain key design decisions

### Deployment Ready
- [ ] Server supports `-db-url pebble://path/to/data` flag
- [ ] Server starts successfully with Pebble backend
- [ ] Health check endpoint works
- [ ] Graceful shutdown works (closes all DBs)
- [ ] Can run alongside SQL backends (same API)

### Final Validation
```bash
# All QA checks must pass
bin/qa_check.sh

# All external tests must pass
bin/run_external_tests_pebble.sh

# Performance benchmarks
cd golang
go test ./internal/store/pebble -bench=. -benchmem

# Coverage check (>80%)
go test ./internal/store/pebble -coverprofile=/tmp/coverage.out
go tool cover -func=/tmp/coverage.out | grep total
```

**WHEN ALL CHECKS PASS**: Pebble backend is production-ready! 🎉

---

## References

- **Design Document**: `docs/KV_STORE_DESIGN.md`
- **Key Schema**: `docs/KV_STORE_KEY_SCHEMA.md`
- **Pebble Documentation**: https://github.com/cockroachdb/pebble
- **LSM Tree Paper**: "The Log-Structured Merge-Tree (LSM-Tree)" by O'Neil et al.

---

## Notes

### Why Pebble?
1. **Pure Go**: No CGo dependencies (easier deployment)
2. **Production-Ready**: Battle-tested in CockroachDB
3. **Performance**: Comparable to RocksDB
4. **Features**: Batch writes, range scans, snapshots, reverse iteration
5. **Active Development**: Well-maintained, responsive maintainers

### Design Decisions
1. **Separate DB per Namespace**: Eliminates prefix overhead (30-40% key size reduction)
2. **Write Mutex**: Simple, matches SQLite pattern, adequate performance
3. **No Type Index Initially**: Add in Phase 6 if needed based on usage patterns
4. **No Correlation Index Initially**: Application-layer filtering adequate
5. **Snappy Compression**: Good balance of speed and compression ratio

### Open Questions
- [ ] Should we add Prometheus metrics from start or defer to Phase 6?
  - **Decision**: Defer to Phase 6, focus on core functionality first
- [ ] Should we implement migration tool in this issue?
  - **Decision**: No, separate issue (out of scope)
- [ ] What's the eviction threshold for LRU?
  - **Decision**: 100 open namespaces (configurable)

---

**End of Issue Document**
