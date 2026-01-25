# EPIC MDB004: Sparse Export/Import Implementation Plan

## Test-Driven Development Approach

### Phase 1: Store ImportBatch Interface (Day 1 - 0.5 day)

#### Phase MDB004_1A: CODE: Implement ImportBatch Interface
- [ ] Add ImportBatch method to store.Store interface in `store.go`
- [ ] Add ErrPositionExists error type
- [ ] Implement ImportBatch for SQLite in `sqlite/store.go`
- [ ] Implement ImportBatch for PostgreSQL in `postgres/store.go`
- [ ] Implement ImportBatch for Pebble in `pebble/store.go`

#### Phase MDB004_1A: TESTS: ImportBatch Tests
- [ ] **MDB004_1A_T1: Test ImportBatch inserts messages with preserved positions (SQLite)**
- [ ] **MDB004_1A_T2: Test ImportBatch inserts messages with preserved positions (PostgreSQL)**
- [ ] **MDB004_1A_T3: Test ImportBatch inserts messages with preserved positions (Pebble)**
- [ ] **MDB004_1A_T4: Test ImportBatch rejects duplicate global position (SQLite)**
- [ ] **MDB004_1A_T5: Test ImportBatch rejects duplicate global position (PostgreSQL)**
- [ ] **MDB004_1A_T6: Test ImportBatch rejects duplicate global position (Pebble)**
- [ ] **MDB004_1A_T7: Test ImportBatch handles sparse positions with gaps**
- [ ] **MDB004_1A_T8: Test ImportBatch transaction rollback on error**

### Phase 2: Import HTTP Handler (Day 1-2 - 1 day)

#### Phase MDB004_2A: CODE: Implement Import Handler
- [ ] Create `internal/api/import_handler.go`
- [ ] Define ExportRecord struct for NDJSON parsing
- [ ] Implement chunked body reading with bufio.Scanner
- [ ] Implement batch buffering (1000 messages)
- [ ] Implement SSE progress event streaming
- [ ] Implement error handling mid-stream
- [ ] Register POST /import endpoint in server.go
- [ ] Add auth middleware to /import endpoint

#### Phase MDB004_2A: TESTS: Import Handler Tests
- [ ] **MDB004_2A_T1: Test /import accepts valid NDJSON stream**
- [ ] **MDB004_2A_T2: Test /import returns progress events**
- [ ] **MDB004_2A_T3: Test /import returns done event with count**
- [ ] **MDB004_2A_T4: Test /import returns error on invalid JSON line**
- [ ] **MDB004_2A_T5: Test /import returns error on duplicate position**
- [ ] **MDB004_2A_T6: Test /import requires authentication**
- [ ] **MDB004_2A_T7: Test /import handles empty body**
- [ ] **MDB004_2A_T8: Test /import batches correctly (every 1000)**

### Phase 3: Export CLI Command (Day 2-3 - 1 day)

#### Phase MDB004_3A: CODE: Implement Export Command
- [ ] Add "export" case to main.go command switch
- [ ] Create `cmd/eventodb/export.go`
- [ ] Parse export flags (url, token, categories, since, until, gzip, output)
- [ ] Implement category fetching via category.get RPC
- [ ] Implement time filtering (since/until)
- [ ] Implement NDJSON streaming output
- [ ] Implement gzip compression wrapper
- [ ] Handle stdout vs file output
- [ ] Add progress output to stderr

#### Phase MDB004_3A: TESTS: Export Command Tests
- [ ] **MDB004_3A_T1: Test export outputs valid NDJSON format**
- [ ] **MDB004_3A_T2: Test export filters by categories**
- [ ] **MDB004_3A_T3: Test export filters by since date**
- [ ] **MDB004_3A_T4: Test export filters by until date**
- [ ] **MDB004_3A_T5: Test export combines category and time filters**
- [ ] **MDB004_3A_T6: Test export with --gzip produces valid gzip**
- [ ] **MDB004_3A_T7: Test export to stdout works**
- [ ] **MDB004_3A_T8: Test export to file works**
- [ ] **MDB004_3A_T9: Test export all (no category filter)**

### Phase 4: Import CLI Command (Day 3 - 0.5 day)

#### Phase MDB004_4A: CODE: Implement Import Command
- [ ] Add "import" case to main.go command switch
- [ ] Create `cmd/eventodb/import.go`
- [ ] Parse import flags (url, token, gzip, input)
- [ ] Implement file streaming to server via HTTP POST
- [ ] Implement gzip decompression wrapper
- [ ] Implement SSE progress event parsing
- [ ] Implement progress display (count, gpos)
- [ ] Handle stdin vs file input
- [ ] Handle error events from server

#### Phase MDB004_4A: TESTS: Import Command Tests
- [ ] **MDB004_4A_T1: Test import sends file as chunked body**
- [ ] **MDB004_4A_T2: Test import with --gzip decompresses input**
- [ ] **MDB004_4A_T3: Test import displays progress updates**
- [ ] **MDB004_4A_T4: Test import handles server errors**
- [ ] **MDB004_4A_T5: Test import from stdin works**
- [ ] **MDB004_4A_T6: Test import from file works**

### Phase 5: Integration & Documentation (Day 4 - 0.5 day)

#### Phase MDB004_5A: CODE: Integration and Docs
- [ ] Update CLI help text in main.go
- [ ] Update docs/API.md with /import endpoint
- [ ] Add export/import examples to documentation
- [ ] Run go vet and gofmt
- [ ] Verify all error paths covered

#### Phase MDB004_5A: TESTS: Integration Tests
- [ ] **MDB004_5A_T1: Test complete export → import roundtrip**
- [ ] **MDB004_5A_T2: Test roundtrip preserves all message fields**
- [ ] **MDB004_5A_T3: Test roundtrip with gzip compression**
- [ ] **MDB004_5A_T4: Test roundtrip with category filter**
- [ ] **MDB004_5A_T5: Test roundtrip with time filter**
- [ ] **MDB004_5A_T6: Test large file (10K events) constant memory**
- [ ] **MDB004_5A_T7: Test cross-namespace export/import**

## Development Workflow Per Phase

For **EACH** phase:

1. **Implement Code** (Phase XA CODE)
2. **Write Tests IMMEDIATELY** (Phase XA TESTS)
3. **Run Tests & Verify** - All tests must pass (`go test ./...`)
4. **Run `go vet && go test ./...`** - Vet and test
5. **Commit with good message** - Only if tests pass
6. **NEVER move to next phase with failing tests**

## File Structure

```
golang/
├── cmd/eventodb/
│   ├── main.go               # Add export/import command routing
│   ├── export.go             # NEW: Export command implementation
│   └── import.go             # NEW: Import command implementation
├── internal/
│   ├── api/
│   │   ├── server.go         # Register /import endpoint
│   │   └── import_handler.go # NEW: Streaming import handler
│   └── store/
│       ├── store.go          # Add ImportBatch to interface
│       ├── errors.go         # Add ErrPositionExists
│       ├── sqlite/store.go   # Implement ImportBatch
│       ├── postgres/store.go # Implement ImportBatch
│       └── pebble/store.go   # Implement ImportBatch
├── test_integration/
│   ├── import_batch_test.go  # Phase 1 tests
│   ├── import_handler_test.go # Phase 2 tests
│   └── export_import_test.go # Phase 5 integration tests
└── test_unit/
    ├── export_test.go        # Phase 3 tests
    └── import_cli_test.go    # Phase 4 tests
```

## Code Size Estimates

```
store.go additions:         ~10 lines  (interface method)
errors.go additions:        ~5 lines   (ErrPositionExists)
sqlite/store.go additions:  ~50 lines  (ImportBatch)
postgres/store.go additions: ~50 lines (ImportBatch)
pebble/store.go additions:  ~60 lines  (ImportBatch)
import_handler.go:          ~150 lines (HTTP handler)
export.go:                  ~120 lines (CLI command)
import.go:                  ~100 lines (CLI command)

Total new code:             ~545 lines of implementation
Tests:                      ~600 lines (39 test scenarios)
```

## Key Implementation Details

**ImportBatch Pattern (SQLite):**
```go
func (s *SQLiteStore) ImportBatch(ctx context.Context, namespace string, messages []*Message) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO messages (id, stream_name, type, position, global_position, data, metadata, time)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, msg := range messages {
        _, err := stmt.ExecContext(ctx,
            msg.ID, msg.StreamName, msg.Type, msg.Position,
            msg.GlobalPosition, msg.Data, msg.Metadata, msg.Time)
        if err != nil {
            if isUniqueViolation(err) {
                return fmt.Errorf("%w: global position %d", ErrPositionExists, msg.GlobalPosition)
            }
            return err
        }
    }

    return tx.Commit()
}
```

**Import Handler Pattern:**
```go
func (h *ImportHandler) Handle(ctx *fasthttp.RequestCtx) {
    namespace := getNamespace(ctx)

    ctx.SetContentType("text/event-stream")
    ctx.Response.Header.Set("Cache-Control", "no-cache")

    ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
        scanner := bufio.NewScanner(bytes.NewReader(ctx.PostBody()))
        batch := make([]*store.Message, 0, batchSize)
        imported := int64(0)
        start := time.Now()

        for scanner.Scan() {
            var record ExportRecord
            if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
                sendError(w, "INVALID_JSON", err.Error(), lineNum)
                return
            }

            batch = append(batch, recordToMessage(&record))

            if len(batch) >= batchSize {
                if err := h.store.ImportBatch(ctx, namespace, batch); err != nil {
                    sendError(w, "IMPORT_FAILED", err.Error(), lineNum)
                    return
                }
                imported += int64(len(batch))
                sendProgress(w, imported, batch[len(batch)-1].GlobalPosition)
                batch = batch[:0]
            }
        }

        // Flush remaining
        if len(batch) > 0 {
            if err := h.store.ImportBatch(ctx, namespace, batch); err != nil {
                sendError(w, "IMPORT_FAILED", err.Error(), lineNum)
                return
            }
            imported += int64(len(batch))
        }

        sendDone(w, imported, time.Since(start))
    })
}
```

**Export Pattern:**
```go
func runExport(cfg *ExportConfig) error {
    client := eventodb.NewClient(cfg.URL, cfg.Token)

    var writer io.Writer = cfg.Output
    if cfg.Gzip {
        gzWriter := gzip.NewWriter(cfg.Output)
        defer gzWriter.Close()
        writer = gzWriter
    }

    encoder := json.NewEncoder(writer)

    categories := cfg.Categories
    if len(categories) == 0 {
        categories = []string{""} // Empty = all via global position scan
    }

    for _, category := range categories {
        position := int64(0)
        for {
            messages, err := client.CategoryGet(category, &eventodb.CategoryOpts{
                Position:  position,
                BatchSize: 1000,
            })
            if err != nil {
                return err
            }
            if len(messages) == 0 {
                break
            }

            for _, msg := range messages {
                if !inTimeRange(msg.Time, cfg.Since, cfg.Until) {
                    continue
                }

                record := messageToRecord(msg)
                if err := encoder.Encode(record); err != nil {
                    return err
                }
            }

            position = messages[len(messages)-1].GlobalPosition + 1
        }
    }

    return nil
}
```

## Test Distribution Summary

- **Phase 1 Tests:** 8 scenarios (ImportBatch for all backends)
- **Phase 2 Tests:** 8 scenarios (Import HTTP handler)
- **Phase 3 Tests:** 9 scenarios (Export CLI command)
- **Phase 4 Tests:** 6 scenarios (Import CLI command)
- **Phase 5 Tests:** 7 scenarios (Integration and roundtrip)

**Total: 38 test scenarios covering all Epic MDB004 acceptance criteria**

## Dependencies

- **Go SDK client** for export (category.get RPC calls)
- **fasthttp** for streaming HTTP handler
- **compress/gzip** standard library

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Network error during export | Return error, partial file may exist |
| Invalid JSON line on import | Return error with line number, abort |
| Duplicate global position | Return error with position, abort |
| Server unreachable | Return connection error |

## Performance Targets

| Operation | Target |
|-----------|--------|
| Export throughput | >10,000 events/sec |
| Import throughput | >5,000 events/sec |
| Batch size | 1,000 messages |
| Memory usage | <100MB constant |

---

## Implementation Status

### EPIC MDB004: SPARSE EXPORT/IMPORT - PENDING
### Current Status: READY FOR IMPLEMENTATION

### Progress Tracking
- [ ] Phase MDB004_1A: Store ImportBatch Interface
- [ ] Phase MDB004_2A: Import HTTP Handler
- [ ] Phase MDB004_3A: Export CLI Command
- [ ] Phase MDB004_4A: Import CLI Command
- [ ] Phase MDB004_5A: Integration & Documentation

### Definition of Done
- [ ] ImportBatch implemented for SQLite
- [ ] ImportBatch implemented for PostgreSQL
- [ ] ImportBatch implemented for Pebble
- [ ] POST /import endpoint with streaming progress
- [ ] Export CLI with category/time filters
- [ ] Import CLI with progress display
- [ ] Gzip compression/decompression
- [ ] All 38 test scenarios passing
- [ ] Performance targets met
- [ ] Code passes go vet and gofmt
- [ ] Documentation updated

### Important Rules
- ✅ Code compiles and tests pass before next phase
- ✅ Epic ID + test ID in test names (MDB004_XA_TN)
- ✅ Preserve original global positions on import
- ✅ Reject duplicate positions (clean namespace expected)
- ✅ Stream data - constant memory usage
- ✅ Batch inserts for performance
- ✅ Progress feedback during import
