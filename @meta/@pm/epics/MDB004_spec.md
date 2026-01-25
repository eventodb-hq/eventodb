# EPIC MDB004: Sparse Export/Import for Local Development

## Overview

**Epic ID:** MDB004
**Name:** Sparse Export/Import for Local Development
**Duration:** 3-4 days
**Status:** pending
**Priority:** high
**Depends On:** None

**Goal:** Enable developers to export filtered event data from production and import it locally with preserved global positions, supporting local debugging with production data subsets.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│ CLI Commands                                        │
│ - eventodb export                                   │
│ - eventodb import                                   │
└─────────────────────────────────────────────────────┘
                        │
            ┌───────────┴───────────┐
            ▼                       ▼
┌───────────────────────┐   ┌───────────────────────┐
│ Export (RPC batches)  │   │ Import (HTTP stream)  │
│ - category.get        │   │ - POST /import        │
│ - filter by time      │   │ - chunked NDJSON      │
│ - stream to file      │   │ - batch INSERT        │
└───────────────────────┘   └───────────────────────┘
                                    │
                                    ▼
                        ┌───────────────────────┐
                        │ SSE Progress Events   │
                        │ - imported count      │
                        │ - last global pos     │
                        │ - errors              │
                        └───────────────────────┘
```

## Technical Requirements

### NDJSON Export Format

```json
{"id":"550e8400-e29b-41d4-a716-446655440000","stream":"workflow-123","type":"TaskRequested","pos":0,"gpos":47,"data":{"task":"process"},"meta":null,"time":"2025-07-15T10:00:00Z"}
{"id":"550e8400-e29b-41d4-a716-446655440001","stream":"order-456","type":"Created","pos":0,"gpos":52,"data":{"amount":100},"meta":{"correlationStreamName":"workflow-123"},"time":"2025-07-15T10:01:00Z"}
```

**Field mapping:**

| Field | Description |
|-------|-------------|
| `id` | Message UUID |
| `stream` | Full stream name |
| `type` | Event type |
| `pos` | Stream position |
| `gpos` | Global position (preserved on import) |
| `data` | Event payload |
| `meta` | Metadata (null if empty) |
| `time` | ISO 8601 timestamp |

### Import HTTP Endpoint

```
POST /import
Authorization: Bearer <token>
Content-Type: application/x-ndjson
Transfer-Encoding: chunked
```

**Response (streaming text/event-stream):**
```
data: {"imported":1000,"gpos":1523}

data: {"imported":2000,"gpos":3042}

data: {"done":true,"imported":3456,"elapsed":"2.3s"}
```

**Error mid-stream:**
```
data: {"error":"POSITION_EXISTS","message":"global position 3050 already exists","line":2047}
```

## Functional Requirements

### FR-1: Export Command

```bash
eventodb export \
  --url http://prod:8080 \
  --token $TOKEN \
  --categories workflow,order \
  --since 2025-07-01 \
  --until 2025-07-25 \
  --gzip \
  --output export.ndjson.gz
```

**Flags:**

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--url` | Yes | - | EventoDB server URL |
| `--token` | Yes | - | Namespace token |
| `--categories` | No | all | Comma-separated category list |
| `--since` | No | - | Start date (inclusive) |
| `--until` | No | - | End date (exclusive) |
| `--gzip` | No | false | Compress output with gzip |
| `--output` | No | stdout | Output file path |

### FR-2: Import Command

```bash
eventodb import \
  --url http://localhost:8080 \
  --token $TOKEN \
  --gzip \
  --input export.ndjson.gz
```

**Flags:**

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--url` | Yes | - | EventoDB server URL |
| `--token` | Yes | - | Namespace token |
| `--gzip` | No | false | Decompress input with gzip |
| `--input` | No | stdin | Input file path |

### FR-3: Store ImportBatch Method

```go
// ImportBatch writes messages with explicit positions (for import/restore)
// Returns error if any globalPosition already exists in the namespace
// All messages in batch are inserted in a single transaction
ImportBatch(ctx context.Context, namespace string, messages []*Message) error
```

### FR-4: Streaming Import Handler

```go
func (h *ImportHandler) Handle(ctx *fasthttp.RequestCtx) {
    // Read chunked NDJSON body
    // Buffer N messages (e.g., 1000)
    // Batch INSERT with preserved positions
    // Stream progress events back
    // Handle errors mid-stream
}
```

### FR-5: Bidirectional Streaming

```
CLI                              Server
 │                                 │
 │─── chunked NDJSON body ───────>│
 │                                 │ buffer 1000
 │                                 │ batch INSERT
 │<── data: {"imported":1000} ────│
 │                                 │
 │─── more lines ────────────────>│
 │                                 │ buffer 1000
 │                                 │ batch INSERT
 │<── data: {"imported":2000} ────│
 │                                 │
 │─── EOF ───────────────────────>│
 │<── data: {"done":true,...} ────│
```

### FR-6: Gzip Compression

**Export:**
```go
var writer io.Writer = output
if useGzip {
    gzWriter := gzip.NewWriter(output)
    defer gzWriter.Close()
    writer = gzWriter
}
encoder := json.NewEncoder(writer)
```

**Import:**
```go
var reader io.Reader = file
if useGzip {
    gzReader, err := gzip.NewReader(file)
    defer gzReader.Close()
    reader = gzReader
}
```

## Implementation Strategy

### Phase 1: Store Interface (0.5 day)
- Add ImportBatch to store.Store interface
- Implement for SQLite backend
- Implement for PostgreSQL backend
- Implement for Pebble backend
- Unit tests for each backend

### Phase 2: Import HTTP Handler (1 day)
- Create import_handler.go
- Implement chunked body reading
- Implement batch buffering
- Implement SSE progress streaming
- Register /import endpoint
- Integration tests

### Phase 3: Export CLI Command (1 day)
- Add export command to main.go
- Implement category fetching via RPC
- Implement time filtering
- Implement NDJSON streaming output
- Implement gzip compression
- CLI tests

### Phase 4: Import CLI Command (0.5 day)
- Add import command to main.go
- Implement file streaming to server
- Implement progress display
- Implement gzip decompression
- CLI tests

### Phase 5: Integration Testing (0.5 day)
- End-to-end export/import roundtrip
- Large file handling
- Error scenarios
- Performance validation

## Acceptance Criteria

### AC-1: Export Full Namespace
- **GIVEN** EventoDB with events
- **WHEN** `eventodb export --url ... --token ...`
- **THEN** All events exported as NDJSON with correct format

### AC-2: Export Filtered by Categories
- **GIVEN** EventoDB with multiple categories
- **WHEN** `eventodb export --categories workflow,order`
- **THEN** Only workflow and order events exported

### AC-3: Export Filtered by Time
- **GIVEN** EventoDB with events spanning months
- **WHEN** `eventodb export --since 2025-07-01 --until 2025-07-15`
- **THEN** Only events in date range exported

### AC-4: Export with Gzip
- **GIVEN** EventoDB with events
- **WHEN** `eventodb export --gzip --output data.ndjson.gz`
- **THEN** Output is valid gzip-compressed NDJSON

### AC-5: Import Preserves Positions
- **GIVEN** Export file with gpos values 47, 52, 89
- **WHEN** `eventodb import --input export.ndjson`
- **THEN** Events stored with exact same global positions

### AC-6: Import Rejects Duplicates
- **GIVEN** Namespace with existing event at gpos 47
- **WHEN** Importing file with event at gpos 47
- **THEN** Error returned, import aborted

### AC-7: Import Shows Progress
- **GIVEN** Large export file
- **WHEN** `eventodb import --input large.ndjson`
- **THEN** Progress updates shown during import

### AC-8: Import with Gzip
- **GIVEN** Gzip-compressed export file
- **WHEN** `eventodb import --gzip --input data.ndjson.gz`
- **THEN** File decompressed and imported correctly

### AC-9: Constant Memory
- **GIVEN** 10GB export file
- **WHEN** Importing
- **THEN** Memory usage stays constant (streaming)

### AC-10: Namespace Agnostic
- **GIVEN** Export from namespace A
- **WHEN** Import to namespace B
- **THEN** Events stored in namespace B with preserved positions

## Definition of Done

- [ ] ImportBatch implemented for SQLite
- [ ] ImportBatch implemented for PostgreSQL
- [ ] ImportBatch implemented for Pebble
- [ ] POST /import endpoint with streaming
- [ ] SSE progress events working
- [ ] Export CLI command working
- [ ] Import CLI command working
- [ ] Gzip compression/decompression working
- [ ] Time filtering working
- [ ] Category filtering working
- [ ] Progress display in CLI
- [ ] Error handling for all edge cases
- [ ] Unit tests for all backends
- [ ] Integration tests for roundtrip
- [ ] Documentation updated (API.md, CLI help)
- [ ] Code passes go vet and gofmt

## Error Codes

- `POSITION_EXISTS` - Global position already exists in namespace
- `INVALID_JSON` - Malformed JSON line in import
- `IMPORT_FAILED` - Database error during import

## Performance Expectations

| Operation | Expected Performance |
|-----------|---------------------|
| Export throughput | >10,000 events/sec |
| Import throughput | >5,000 events/sec |
| Memory usage | Constant (<100MB) |
| Batch size | 1,000 messages |
| Progress interval | Every batch |

## Non-Goals

- ❌ Schema validation on import
- ❌ Incremental/resumable export
- ❌ Automatic gzip detection
- ❌ Parallel import
- ❌ Cross-version compatibility checks
