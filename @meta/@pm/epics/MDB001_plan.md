# EPIC MDB001: Core Storage & Migrations Implementation Plan

## Test-Driven Development Approach

### Phase 1: Foundation (Days 1-3)
**Migrations + Store Interface + Error Handling**

#### Phase MDB001_1A: CODE: Complete Foundation
- [ ] Create `migrations/` directory structure
- [ ] Create `migrations/embed.go` with embed directives
- [ ] Create `migrations/metadata/postgres/001_namespace_registry.sql`
- [ ] Create `migrations/metadata/sqlite/001_namespace_registry.sql`
- [ ] Create `migrations/namespace/postgres/001_message_db.sql` (template with {{SCHEMA_NAME}})
- [ ] Create `migrations/namespace/sqlite/001_message_db.sql`
- [ ] Create `internal/migrate/migrate.go` with Migrator struct
- [ ] Implement `AutoMigrate()` method
- [ ] Implement `ApplyNamespaceMigration()` with template substitution
- [ ] Add `schema_migrations` table creation
- [ ] Create `internal/store/store.go` with Store interface
- [ ] Define Message, WriteResult, GetOpts, CategoryOpts, Namespace structs
- [ ] Add error types (ErrVersionConflict, ErrNamespaceNotFound, etc.)

#### Phase MDB001_1A: TESTS: Foundation Tests
- [ ] **MDB001_1A_T1: Test AutoMigrate creates schema_migrations table**
- [ ] **MDB001_1A_T2: Test AutoMigrate applies pending migrations**
- [ ] **MDB001_1A_T3: Test AutoMigrate skips already-applied migrations**
- [ ] **MDB001_1A_T4: Test ApplyNamespaceMigration substitutes {{SCHEMA_NAME}}**
- [ ] **MDB001_1A_T5: Test migration tracking records version and timestamp**
- [ ] **MDB001_1A_T6: Test Message struct creation and JSON serialization**
- [ ] **MDB001_1A_T7: Test WriteResult struct**
- [ ] **MDB001_1A_T8: Test GetOpts validation**
- [ ] **MDB001_1A_T9: Test CategoryOpts validation**

### Phase 2: Postgres Backend - Setup & Namespaces (Days 4-6)
**Complete Postgres infrastructure and namespace operations**

#### Phase MDB001_2A: CODE: Postgres Setup & Namespaces
- [ ] Create `internal/store/postgres/store.go`
- [ ] Implement PostgresStore struct
- [ ] Implement New() constructor and Close() method
- [ ] Add helper methods `getSchemaName()` and `sanitizeSchemaName()`
- [ ] Implement CreateNamespace() (CREATE SCHEMA, apply migrations, insert registry)
- [ ] Implement DeleteNamespace() (DROP SCHEMA CASCADE, delete from registry)
- [ ] Implement GetNamespace() and ListNamespaces()

#### Phase MDB001_2A: TESTS: Postgres Setup & Namespace Tests
- [ ] **MDB001_2A_T1: Test PostgresStore creation and connection**
- [ ] **MDB001_2A_T2: Test Close() cleanup**
- [ ] **MDB001_2A_T3: Test CreateNamespace creates schema**
- [ ] **MDB001_2A_T4: Test CreateNamespace applies migrations**
- [ ] **MDB001_2A_T5: Test CreateNamespace inserts into registry**
- [ ] **MDB001_2A_T6: Test DeleteNamespace drops schema**
- [ ] **MDB001_2A_T7: Test DeleteNamespace removes from registry**
- [ ] **MDB001_2A_T8: Test GetNamespace returns correct data**
- [ ] **MDB001_2A_T9: Test ListNamespaces returns all namespaces**

### Phase 3: Postgres Backend - Messages (Days 7-9)
**Write and read operations with stored procedures**

#### Phase MDB001_3A: CODE: Postgres Message Operations
- [ ] Implement WriteMessage() (call stored procedure, handle version conflict)
- [ ] Implement stored procedure write_message in migration template
- [ ] Implement GetStreamMessages() (call stored procedure, parse results)
- [ ] Implement GetCategoryMessages() (handle consumer groups)
- [ ] Implement GetLastStreamMessage() and GetStreamVersion()
- [ ] Implement stored procedures in migration template

#### Phase MDB001_3A: TESTS: Postgres Message Tests
- [ ] **MDB001_3A_T1: Test WriteMessage writes to correct schema**
- [ ] **MDB001_3A_T2: Test WriteMessage assigns correct position and global_position**
- [ ] **MDB001_3A_T3: Test WriteMessage with expected_version success**
- [ ] **MDB001_3A_T4: Test WriteMessage with expected_version conflict**
- [ ] **MDB001_3A_T5: Test WriteMessage increments position correctly**
- [ ] **MDB001_3A_T6: Test GetStreamMessages returns correct messages**
- [ ] **MDB001_3A_T7: Test GetStreamMessages with position offset and batch size**
- [ ] **MDB001_3A_T8: Test GetCategoryMessages returns from multiple streams**
- [ ] **MDB001_3A_T9: Test GetCategoryMessages with consumer groups**
- [ ] **MDB001_3A_T10: Test GetCategoryMessages consumer group partition**
- [ ] **MDB001_3A_T11: Test GetLastStreamMessage returns last**
- [ ] **MDB001_3A_T12: Test GetLastStreamMessage with type filter**
- [ ] **MDB001_3A_T13: Test GetStreamVersion returns correct version**

### Phase 4: SQLite Backend - Setup & Namespaces (Days 10-11)
**Complete SQLite infrastructure with in-memory and file modes**

#### Phase MDB001_4A: CODE: SQLite Setup & Namespaces
- [ ] Create `internal/store/sqlite/store.go`
- [ ] Implement SQLiteStore struct with namespace DB map and mutex
- [ ] Implement New() constructor and Close() method
- [ ] Implement getOrCreateNamespaceDB() with lazy loading
- [ ] Implement CreateNamespace() (determine db_path, insert metadata)
- [ ] Implement DeleteNamespace() (close connection, delete file, remove from metadata)
- [ ] Implement GetNamespace() and ListNamespaces()

#### Phase MDB001_4A: TESTS: SQLite Setup & Namespace Tests
- [ ] **MDB001_4A_T1: Test SQLiteStore creation**
- [ ] **MDB001_4A_T2: Test SQLiteStore with testMode=true (in-memory)**
- [ ] **MDB001_4A_T3: Test SQLiteStore with testMode=false (file-based)**
- [ ] **MDB001_4A_T4: Test Close() cleanup**
- [ ] **MDB001_4A_T5: Test CreateNamespace in test mode (in-memory)**
- [ ] **MDB001_4A_T6: Test CreateNamespace in file mode**
- [ ] **MDB001_4A_T7: Test DeleteNamespace closes connection and deletes file**
- [ ] **MDB001_4A_T8: Test getOrCreateNamespaceDB lazy-loads**
- [ ] **MDB001_4A_T9: Test GetNamespace returns correct data**
- [ ] **MDB001_4A_T10: Test ListNamespaces returns all namespaces**

### Phase 5: SQLite Backend - Messages (Days 12-14)
**Write and read operations in pure Go**

#### Phase MDB001_5A: CODE: SQLite Message Operations
- [ ] Implement WriteMessage() in Go (transaction, version check, position calc, insert)
- [ ] Implement GetStreamMessages() in Go (query, deserialize JSON)
- [ ] Implement GetCategoryMessages() in Go (category extraction, consumer group hash)
- [ ] Implement GetLastStreamMessage() and GetStreamVersion()

#### Phase MDB001_5A: TESTS: SQLite Message Tests
- [ ] **MDB001_5A_T1: Test WriteMessage writes to correct namespace DB**
- [ ] **MDB001_5A_T2: Test WriteMessage assigns correct position and global_position**
- [ ] **MDB001_5A_T3: Test WriteMessage with expected_version success**
- [ ] **MDB001_5A_T4: Test WriteMessage with expected_version conflict**
- [ ] **MDB001_5A_T5: Test WriteMessage serializes JSON correctly**
- [ ] **MDB001_5A_T6: Test GetStreamMessages returns correct messages**
- [ ] **MDB001_5A_T7: Test GetStreamMessages with position offset and batch size**
- [ ] **MDB001_5A_T8: Test GetCategoryMessages returns from multiple streams**
- [ ] **MDB001_5A_T9: Test GetCategoryMessages with consumer groups**
- [ ] **MDB001_5A_T10: Test GetCategoryMessages hash partitioning matches Postgres**
- [ ] **MDB001_5A_T11: Test GetLastStreamMessage returns last**
- [ ] **MDB001_5A_T12: Test GetLastStreamMessage with type filter**
- [ ] **MDB001_5A_T13: Test GetStreamVersion returns correct version**

### Phase 6: Cross-Backend Integration (Days 15-16)
**Ensure both backends behave identically**

#### Phase MDB001_6A: CODE: Backend Parity
- [ ] Create test suite that runs against both backends
- [ ] Add backend factory for tests
- [ ] Ensure identical behavior (modulo type differences)

#### Phase MDB001_6A: TESTS: Cross-Backend Tests
- [ ] **MDB001_6A_T1: Test namespace isolation (both backends)**
- [ ] **MDB001_6A_T2: Test write/read parity (both backends)**
- [ ] **MDB001_6A_T3: Test category queries parity**
- [ ] **MDB001_6A_T4: Test consumer group partitioning parity**
- [ ] **MDB001_6A_T5: Test optimistic locking parity**
- [ ] **MDB001_6A_T6: Test concurrent writes to different namespaces**
- [ ] **MDB001_6A_T7: Test concurrent writes to same stream**

### Phase 7: Performance & Documentation (Day 17)
**Benchmarks, docs, and final validation**

#### Phase MDB001_7A: CODE: Performance & Docs
- [ ] Add performance benchmarks for all operations
- [ ] Document Store interface with godoc
- [ ] Add usage examples
- [ ] Create README for internal/store

#### Phase MDB001_7A: TESTS: Performance Tests
- [ ] **MDB001_7A_T1: Benchmark WriteMessage (Postgres)**
- [ ] **MDB001_7A_T2: Benchmark WriteMessage (SQLite file)**
- [ ] **MDB001_7A_T3: Benchmark WriteMessage (SQLite memory)**
- [ ] **MDB001_7A_T4: Benchmark GetStreamMessages (all backends)**
- [ ] **MDB001_7A_T5: Benchmark GetCategoryMessages (all backends)**
- [ ] **MDB001_7A_T6: Test performance targets met**

## Development Workflow Per Phase

For **EACH** phase:

1. **Implement Code** (Phase XA CODE)
2. **Write Tests IMMEDIATELY** (Phase XA TESTS)
3. **Run Tests & Verify** - All tests must pass (`go test ./...`)
4. **Run Linters** - `go vet` and `golangci-lint run`
5. **Commit with good message** - Only if tests pass
6. **NEVER move to next phase with failing tests**

## File Structure

```
messagedb-go/
├── go.mod
├── go.sum
├── cmd/
│   └── messagedb/
│       └── main.go                 # Server binary (later epic)
├── internal/
│   ├── store/
│   │   ├── store.go                # Interface + types
│   │   ├── errors.go               # Error types
│   │   ├── postgres/
│   │   │   ├── store.go            # PostgresStore implementation
│   │   │   ├── namespace.go        # Namespace operations
│   │   │   ├── write.go            # Write operations
│   │   │   └── read.go             # Read operations
│   │   └── sqlite/
│   │       ├── store.go            # SQLiteStore implementation
│   │       ├── namespace.go        # Namespace operations
│   │       ├── write.go            # Write operations (Go logic)
│   │       └── read.go             # Read operations (Go logic)
│   └── migrate/
│       ├── migrate.go              # Migrator implementation
│       └── template.go             # Template substitution
├── migrations/
│   ├── embed.go                    # Embed directives
│   ├── metadata/
│   │   ├── postgres/
│   │   │   └── 001_namespace_registry.sql
│   │   └── sqlite/
│   │       └── 001_namespace_registry.sql
│   └── namespace/
│       ├── postgres/
│       │   └── 001_message_db.sql  # Template with {{SCHEMA_NAME}}
│       └── sqlite/
│           └── 001_message_db.sql
└── internal/store/
    ├── store_test.go               # Interface tests
    ├── postgres/
    │   ├── store_test.go
    │   ├── namespace_test.go
    │   ├── write_test.go
    │   └── read_test.go
    ├── sqlite/
    │   ├── store_test.go
    │   ├── namespace_test.go
    │   ├── write_test.go
    │   └── read_test.go
    └── integration_test.go         # Cross-backend tests
```

## Code Size Estimates

```
migrations/                 ~400 lines  (SQL)
internal/migrate/           ~200 lines  (Migrator)
internal/store/store.go     ~150 lines  (Interface + types)
internal/store/postgres/    ~600 lines  (Full implementation)
internal/store/sqlite/      ~800 lines  (Full implementation + Go logic)

Total implementation:       ~2150 lines
Tests:                      ~2500 lines (60+ test scenarios)
```

## Key Implementation Details

**Consumer Group Hash (must match between backends):**
```go
func hashStreamName(streamName string, consumerSize int64) int64 {
    h := fnv.New64a()
    h.Write([]byte(streamName))
    return int64(h.Sum64() % uint64(consumerSize))
}

// Use in WHERE clause
// Postgres: WHERE hash_64(stream_name) % $consumer_size = $consumer_member
// SQLite: Implement in Go, filter results
```

**Template Substitution:**
```go
func applyTemplate(template, schemaName string) string {
    return strings.ReplaceAll(template, "{{SCHEMA_NAME}}", schemaName)
}
```

**Schema Name Sanitization:**
```go
func sanitizeSchemaName(namespace string) string {
    // Only allow alphanumeric and underscore
    reg := regexp.MustCompile("[^a-zA-Z0-9_]+")
    return reg.ReplaceAllString(namespace, "_")
}
```

## Test Distribution Summary

- **Phase 1 Tests:** 9 scenarios (Migrations + Store interface + Types)
- **Phase 2 Tests:** 9 scenarios (Postgres setup + namespaces)
- **Phase 3 Tests:** 13 scenarios (Postgres message operations)
- **Phase 4 Tests:** 10 scenarios (SQLite setup + namespaces)
- **Phase 5 Tests:** 13 scenarios (SQLite message operations)
- **Phase 6 Tests:** 7 scenarios (Cross-backend integration)
- **Phase 7 Tests:** 6 scenarios (Performance benchmarks)

**Total: 67 test scenarios covering all Epic MDB001 acceptance criteria**

## Dependencies

**Go Packages:**
- `database/sql` - Database interface
- `github.com/lib/pq` - Postgres driver
- `github.com/mattn/go-sqlite3` - SQLite driver
- `github.com/google/uuid` - UUID generation

## Error Codes

- `ErrVersionConflict` - Optimistic locking violation
- `ErrNamespaceNotFound` - Namespace doesn't exist
- `ErrNamespaceExists` - Namespace already exists
- `ErrStreamNotFound` - Stream doesn't exist
- `ErrInvalidStreamName` - Invalid stream name format
- `ErrMigrationFailed` - Migration execution failed

## Performance Targets

| Operation | Postgres | SQLite (file) | SQLite (memory) |
|-----------|----------|---------------|-----------------|
| WriteMessage | <10ms | <5ms | <1ms |
| GetStreamMessages (10) | <15ms | <8ms | <2ms |
| GetCategoryMessages (100) | <50ms | <30ms | <10ms |
| CreateNamespace | <100ms | <50ms | <20ms |
| DeleteNamespace | <200ms | <100ms | <50ms |

---

## Implementation Status

### EPIC MDB001: CORE STORAGE & MIGRATIONS - PENDING
### Current Status: READY FOR IMPLEMENTATION

### Progress Tracking
- [ ] Phase MDB001_1A: Foundation (Migrations + Store Interface)
- [ ] Phase MDB001_2A: Postgres Backend - Setup & Namespaces
- [ ] Phase MDB001_3A: Postgres Backend - Messages
- [ ] Phase MDB001_4A: SQLite Backend - Setup & Namespaces
- [ ] Phase MDB001_5A: SQLite Backend - Messages
- [ ] Phase MDB001_6A: Cross-Backend Integration
- [ ] Phase MDB001_7A: Performance & Documentation

### Definition of Done
- [ ] Migration system with AutoMigrate working
- [ ] Metadata migrations for both backends
- [ ] Namespace migrations with template support
- [ ] Store interface defined
- [ ] Postgres backend fully implemented
- [ ] SQLite backend fully implemented
- [ ] All Message operations work
- [ ] All Category operations work
- [ ] All Namespace operations work
- [ ] Optimistic locking enforced
- [ ] Physical isolation verified
- [ ] Test mode (in-memory SQLite) working
- [ ] Both backends pass same test suite
- [ ] All 67 test scenarios passing
- [ ] Performance targets met
- [ ] Code documented with godoc
- [ ] Linters pass (go vet, golangci-lint)

### Important Rules
- ✅ Code compiles and tests pass before next phase
- ✅ Epic ID + test ID in test names (MDB001_XA_TN)
- ✅ Template substitution for namespace schemas
- ✅ Consumer group hash must match between backends
- ✅ Physical isolation (separate schemas/DBs)
- ✅ Test mode uses in-memory SQLite
- ✅ Comprehensive error handling
