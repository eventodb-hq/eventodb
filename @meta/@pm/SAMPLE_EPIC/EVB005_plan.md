# EPIC EVB005: Event Schema Validation Implementation Plan

## Test-Driven Development Approach

### Phase 1: Core Types & Validation (Day 1 - 0.5 day)

#### Phase EVB005_1A: CODE: Implement Schema Types
- [ ] Add EventSchema struct to `src/types.rs`
- [ ] Add SchemaField struct with name, field_type, required
- [ ] Add FieldType enum (String, Int64, Float64, Boolean, Timestamp, Any)
- [ ] Implement EventSchema::validate() method
- [ ] Add SchemaInfo struct for query responses
- [ ] Export new types from lib.rs

#### Phase EVB005_1A: TESTS: Schema Type Tests
- [ ] **EVB005_1A_T1: Test EventSchema creation with valid fields**
- [ ] **EVB005_1A_T2: Test EventSchema validation rejects version != 1**
- [ ] **EVB005_1A_T3: Test EventSchema validation rejects empty fields**
- [ ] **EVB005_1A_T4: Test SchemaField validation rejects empty name**

### Phase 2: Database Schema & Compression (Day 1 - 0.5 day)

#### Phase EVB005_2A: CODE: Database Migration and Compression
- [ ] Create migration in `src/schema.rs` to extend event_types table
- [ ] Add schema_definition BLOB column
- [ ] Add schema_version INTEGER DEFAULT 1
- [ ] Add accept_any INTEGER DEFAULT 0
- [ ] Add compression_algo TEXT DEFAULT 'lz4'
- [ ] Add created_at INTEGER NOT NULL
- [ ] Add lz4_flex dependency to Cargo.toml
- [ ] Implement compress_lz4() function
- [ ] Implement decompress_lz4() function
- [ ] Add compression helpers to lib.rs exports

#### Phase EVB005_2A: TESTS: Compression Tests
- [ ] **EVB005_2A_T1: Test LZ4 compression reduces JSON size**
- [ ] **EVB005_2A_T2: Test LZ4 decompression restores original data**
- [ ] **EVB005_2A_T3: Test compression/decompression roundtrip**

### Phase 3: Schema Registration & Deletion (Day 2)

#### Phase EVB005_3A: CODE: Implement Schema Management
- [ ] Implement Domain::register_schema() method
- [ ] Add validation: schema required if accept_any=false
- [ ] Compress schema JSON before storing
- [ ] Handle INSERT OR REPLACE for updates
- [ ] Implement Domain::delete_schema() method
- [ ] Add EXISTS check to prevent deletion of in-use schemas
- [ ] Add SchemaNotFound error variant
- [ ] Add SchemaInUse error variant
- [ ] Add InvalidSchemaDefinition error variant

#### Phase EVB005_3A: TESTS: Schema Management Tests
- [ ] **EVB005_3A_T1: Test register_schema stores schema in database**
- [ ] **EVB005_3A_T2: Test register_schema compresses schema definition**
- [ ] **EVB005_3A_T3: Test register_schema with accept_any=true**
- [ ] **EVB005_3A_T4: Test register_schema rejects empty schema when accept_any=false**
- [ ] **EVB005_3A_T5: Test delete_schema removes schema**
- [ ] **EVB005_3A_T6: Test delete_schema fails when events exist (SchemaInUse)**
- [ ] **EVB005_3A_T7: Test delete_schema succeeds when no events**
- [ ] **EVB005_3A_T8: Test delete_schema returns SchemaNotFound for missing schema**

### Phase 4: Validation on Append (Day 3)

#### Phase EVB005_4A: CODE: Integrate Validation
- [ ] Implement validate_event_data() method in Domain
- [ ] Load schema definition from event_types table
- [ ] Skip validation if accept_any=1
- [ ] Skip validation if no schema registered
- [ ] Decompress and parse schema JSON
- [ ] Validate required fields present
- [ ] Implement validate_field_type() helper
- [ ] Add type checking for String, Int64, Float64, Boolean, Timestamp
- [ ] Allow extra fields not in schema (permissive mode)
- [ ] Add ValidationFailed error variant
- [ ] Integrate validation into append_to_stream()
- [ ] Compress event data with LZ4 before storing

#### Phase EVB005_4A: TESTS: Validation Tests
- [ ] **EVB005_4A_T1: Test validation passes with all required fields**
- [ ] **EVB005_4A_T2: Test validation fails with missing required field**
- [ ] **EVB005_4A_T3: Test validation passes with extra fields (permissive)**
- [ ] **EVB005_4A_T4: Test validation fails with wrong type**
- [ ] **EVB005_4A_T5: Test validation skips when accept_any=true**
- [ ] **EVB005_4A_T6: Test validation passes with optional field missing**
- [ ] **EVB005_4A_T7: Test validation passes with type=any field**
- [ ] **EVB005_4A_T8: Test event data compressed before storage**

### Phase 5: Schema Query Operations (Day 3 - 0.5 day)

#### Phase EVB005_5A: CODE: Implement Schema Queries
- [ ] Implement Domain::list_schemas() method
- [ ] Query event_types for schemas
- [ ] Decompress schema definitions
- [ ] Return Vec<SchemaInfo>
- [ ] Implement Domain::get_schema() method
- [ ] Handle schema not found case
- [ ] Add proper error handling

#### Phase EVB005_5A: TESTS: Schema Query Tests
- [ ] **EVB005_5A_T1: Test list_schemas returns all registered schemas**
- [ ] **EVB005_5A_T2: Test list_schemas includes accept_any schemas**
- [ ] **EVB005_5A_T3: Test get_schema returns correct schema**
- [ ] **EVB005_5A_T4: Test get_schema returns SchemaNotFound for missing**

### Phase 6: Sandbox Schema Copying (Day 4 - 0.5 day)

#### Phase EVB005_6A: CODE: Implement Schema Copying
- [ ] Implement Domain::copy_schemas_from() method
- [ ] Query schemas from source connection
- [ ] Insert schemas into target connection
- [ ] Use INSERT OR IGNORE to handle duplicates
- [ ] Return count of schemas copied
- [ ] Add integration with sandbox creation

#### Phase EVB005_6A: TESTS: Schema Copying Tests
- [ ] **EVB005_6A_T1: Test copy_schemas_from copies all schemas**
- [ ] **EVB005_6A_T2: Test copy_schemas_from does not copy events**
- [ ] **EVB005_6A_T3: Test copy_schemas_from returns correct count**
- [ ] **EVB005_6A_T4: Test copied schemas work for validation**

### Phase 7: Integration & Performance (Day 4 - 0.5 day)

#### Phase EVB005_7A: CODE: Integration and Docs
- [ ] Add documentation comments to all public APIs
- [ ] Update error Display implementations
- [ ] Run clippy and fix warnings
- [ ] Format code with rustfmt
- [ ] Verify all error paths covered

#### Phase EVB005_7A: TESTS: Integration Tests
- [ ] **EVB005_7A_T1: Test complete workflow: register → validate → append**
- [ ] **EVB005_7A_T2: Test schema deletion prevents append after delete**
- [ ] **EVB005_7A_T3: Test multiple event types with different schemas**
- [ ] **EVB005_7A_T4: Test performance: validation on 1K events <5ms avg**
- [ ] **EVB005_7A_T5: Test performance: compression on 1K events <2ms avg**
- [ ] **EVB005_7A_T6: Test complex schema with all field types**

## Development Workflow Per Phase

For **EACH** phase:

1. **Implement Code** (Phase XA CODE)
2. **Write Tests IMMEDIATELY** (Phase XA TESTS)
3. **Run Tests & Verify** - All tests must pass (`cargo test`)
4. **Run `cargo clippy && cargo test`** - Lint and test
5. **Commit with good message** - Only if tests pass
6. **NEVER move to next phase with failing tests**

## File Structure

```
eventobase-core/
├── Cargo.toml                  # Add lz4_flex dependency
├── src/
│   ├── lib.rs                  # Export schema types and functions
│   ├── schema.rs               # Add migration for event_types
│   ├── types.rs                # Add EventSchema, SchemaField, FieldType
│   ├── domain.rs               # Add validation and schema management
│   ├── error.rs                # Add schema-related errors
│   └── compression.rs          # NEW: LZ4 helpers
└── tests/
    ├── schema_types_tests.rs   # Phase 1 tests
    ├── compression_tests.rs    # Phase 2 tests
    ├── schema_mgmt_tests.rs    # Phase 3 tests
    ├── validation_tests.rs     # Phase 4 tests
    ├── schema_query_tests.rs   # Phase 5 tests
    ├── schema_copy_tests.rs    # Phase 6 tests
    └── schema_integration_tests.rs # Phase 7 tests
```

## Code Size Estimates

```
types.rs additions:     ~60 lines  (EventSchema, SchemaField, FieldType)
compression.rs:         ~30 lines  (LZ4 helpers)
domain.rs extensions:   ~200 lines (validation, registration, queries)
schema.rs migration:    ~10 lines  (ALTER TABLE statements)
error.rs additions:     ~15 lines  (new error variants)

Total new code:         ~315 lines of implementation
Tests:                  ~500 lines (30 test scenarios)
```

## Key Implementation Details

**Validation Pattern:**
```rust
fn validate_event_data(&self, event_type_id: i64, data: &Value) -> Result<()> {
    // Load schema config
    let (schema_def, accept_any) = self.conn.query_row(
        "SELECT schema_definition, accept_any FROM event_types WHERE id = ?",
        [event_type_id],
        |row| Ok((row.get(0)?, row.get(1)?))
    ).optional()?.unwrap_or((None, 0));

    // Skip if accept_any or no schema
    if accept_any == 1 || schema_def.is_none() {
        return Ok(());
    }

    // Validate against schema
    let schema: EventSchema = decompress_and_parse(&schema_def.unwrap())?;
    validate_required_fields(&schema, data)?;
    validate_field_types(&schema, data)?;

    Ok(())
}
```

**Compression Pattern:**
```rust
pub fn compress_lz4(data: &[u8]) -> Vec<u8> {
    lz4_flex::compress_prepend_size(data)
}

pub fn decompress_lz4(compressed: &[u8]) -> Result<Vec<u8>> {
    lz4_flex::decompress_size_prepended(compressed)
        .map_err(|e| Error::DecompressionFailed(e.to_string()))
}
```

**Safe Deletion Pattern:**
```rust
pub fn delete_schema(&mut self, event_type: &str) -> Result<()> {
    let tx = self.conn.transaction()?;

    let event_type_id: i64 = tx.query_row(
        "SELECT id FROM event_types WHERE name = ?",
        [event_type],
        |row| row.get(0)
    ).map_err(|_| Error::SchemaNotFound(event_type.to_string()))?;

    // Fast EXISTS check (no COUNT)
    let has_events: bool = tx.query_row(
        "SELECT EXISTS(SELECT 1 FROM events WHERE event_type_id = ? LIMIT 1)",
        [event_type_id],
        |row| row.get(0)
    )?;

    if has_events {
        return Err(Error::SchemaInUse(event_type.to_string()));
    }

    tx.execute("DELETE FROM event_types WHERE id = ?", [event_type_id])?;
    tx.commit()?;
    Ok(())
}
```

## Test Distribution Summary

- **Phase 1 Tests:** 4 scenarios (Schema type validation)
- **Phase 2 Tests:** 3 scenarios (Compression)
- **Phase 3 Tests:** 8 scenarios (Registration and deletion)
- **Phase 4 Tests:** 8 scenarios (Validation on append)
- **Phase 5 Tests:** 4 scenarios (Schema queries)
- **Phase 6 Tests:** 4 scenarios (Schema copying)
- **Phase 7 Tests:** 6 scenarios (Integration and performance)

**Total: 37 test scenarios covering all Epic EVB005 acceptance criteria**

## Dependencies

- **EVB001:** Core storage, vocabulary management, append logic
- **lz4_flex:** Rust LZ4 compression library

## Validation Rules

1. Required fields must be present
2. Field types must match (except `any`)
3. Extra fields are accepted (permissive)
4. Validation only at write time
5. Skip validation if `accept_any=true`

## Error Codes

- `SCHEMA_VALIDATION_FAILED` - Event data doesn't match schema
- `SCHEMA_IN_USE` - Cannot delete schema with events
- `SCHEMA_NOT_FOUND` - Schema doesn't exist
- `INVALID_SCHEMA_DEFINITION` - Malformed schema

## Performance Targets

| Operation | Target |
|-----------|--------|
| Schema validation | <5ms per event |
| LZ4 compression | <2ms per event |
| Schema registration | <20ms |
| Schema deletion check | <10ms |

---

## Implementation Status

### EPIC EVB005: EVENT SCHEMA VALIDATION - PENDING
### Current Status: READY FOR IMPLEMENTATION

### Progress Tracking
- [ ] Phase EVB005_1A: Core Types & Validation
- [ ] Phase EVB005_2A: Database Schema & Compression
- [ ] Phase EVB005_3A: Schema Registration & Deletion
- [ ] Phase EVB005_4A: Validation on Append
- [ ] Phase EVB005_5A: Schema Query Operations
- [ ] Phase EVB005_6A: Sandbox Schema Copying
- [ ] Phase EVB005_7A: Integration & Performance

### Definition of Done
- [ ] EventSchema, SchemaField, FieldType types implemented
- [ ] Database migration for event_types columns
- [ ] LZ4 compression/decompression working
- [ ] register_schema with compression
- [ ] delete_schema with EXISTS check
- [ ] Validation on append with all field types
- [ ] Permissive mode (extra fields accepted)
- [ ] Accept-any mode working
- [ ] list_schemas and get_schema implemented
- [ ] copy_schemas_from for sandboxes
- [ ] All 37 test scenarios passing
- [ ] Performance targets met
- [ ] Code passes clippy and rustfmt
- [ ] Documentation complete

### Important Rules
- ✅ Code compiles and tests pass before next phase
- ✅ Epic ID + test ID in test names (EVB005_XA_TN)
- ✅ EXISTS query for deletion (not COUNT)
- ✅ Permissive validation (accept extra fields)
- ✅ LZ4 compression on all event data
- ✅ No nested validation (use `any` type)
- ✅ Co-located tests in tests/ directory
