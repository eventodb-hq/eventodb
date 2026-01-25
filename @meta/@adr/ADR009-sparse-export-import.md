# ADR-009: Sparse Export/Import for Local Development

**Date:** 2025-01-25  
**Status:** Proposed  
**Context:** Enable local debugging with production data subsets

---

## Problem

Developers need to reproduce production issues locally. This requires:

1. Syncing a service's local database (projections) from production
2. Having matching events in local EventoDB to continue processing

**Challenge:** EventoDB streams are shared collaboration spaces. Multiple services write to the same streams. There's no single-service ownership.

```
stream: workflow-123
  ├── ServiceA writes: TaskRequested     (gpos: 47)
  ├── ServiceB writes: TaskCompleted     (gpos: 52)
  ├── ServiceA writes: NextStepStarted   (gpos: 89)
  └── ServiceC writes: FinalResult       (gpos: 134)
```

If ServiceA's local DB says "processed up to globalPosition 89", the local EventoDB **must** have that exact event at globalPosition 89.

**Constraints:**
- Global positions must match production (no remapping)
- Only relevant categories needed (not entire namespace)
- Gaps in global positions are acceptable

---

## Decision

**Add CLI commands `eventodb export` and `eventodb import`** for sparse data transfer.

- Export filters by categories and time range
- Import preserves original global positions
- Gaps in global positions are expected and acceptable

---

## Design

### Export Command

```bash
eventodb export \
  --url http://prod:8080 \
  --token $PROD_TOKEN \
  --categories workflow,order,inventory \
  --since 2025-07-01 \
  --until 2025-07-25 \
  --output export.ndjson
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--url` | Yes | EventoDB server URL |
| `--token` | Yes | Namespace token |
| `--categories` | Yes | Comma-separated category list |
| `--since` | No | Start date (inclusive), ISO 8601 or YYYY-MM-DD |
| `--until` | No | End date (exclusive), ISO 8601 or YYYY-MM-DD |
| `--output` | No | Output file (default: stdout) |

**Output Format (NDJSON):**

```json
{"id":"550e8400-e29b-41d4-a716-446655440000","stream":"workflow-123","type":"TaskRequested","pos":0,"gpos":47,"data":{"task":"process"},"meta":null,"time":"2025-07-15T10:00:00Z"}
{"id":"550e8400-e29b-41d4-a716-446655440001","stream":"order-456","type":"Created","pos":0,"gpos":52,"data":{"amount":100},"meta":{"correlationStreamName":"workflow-123"},"time":"2025-07-15T10:01:00Z"}
```

**Field mapping:**

| Field | Source |
|-------|--------|
| `id` | Message UUID |
| `stream` | Full stream name |
| `type` | Event type |
| `pos` | Stream position |
| `gpos` | Global position |
| `data` | Event payload |
| `meta` | Metadata (null if empty) |
| `time` | ISO 8601 timestamp |

### Import Command

```bash
eventodb import \
  --url http://localhost:8080 \
  --token $LOCAL_TOKEN \
  --input export.ndjson
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--url` | Yes | EventoDB server URL |
| `--token` | Yes | Namespace token (target namespace) |
| `--input` | No | Input file (default: stdin) |

**Behavior:**
- Target namespace derived from token
- Preserves original `gpos` and `pos` values
- Errors if any `gpos` already exists (expects clean namespace)
- Namespace-agnostic: export from namespace A, import to namespace B

### Typical Workflow

```bash
# 1. Export relevant categories from production
eventodb export \
  --url http://prod:8080 \
  --token $PROD_TOKEN \
  --categories workflow,order \
  --since 2025-07-01 \
  --output prod-data.ndjson

# 2. Create fresh local namespace (or delete existing)
curl -X POST http://localhost:8080/rpc \
  -d '["ns.delete", "local-debug"]'
curl -X POST http://localhost:8080/rpc \
  -d '["ns.create", "local-debug"]'

# 3. Import to local EventoDB
eventodb import \
  --url http://localhost:8080 \
  --token $LOCAL_TOKEN \
  --input prod-data.ndjson

# 4. Sync service database from production
pg_dump ... | psql local_service_db

# 5. Run service locally - positions match!
```

---

## Implementation

### Architecture

```
┌─────────────┐     HTTP POST      ┌─────────────┐
│   CLI       │  ──────────────>   │   Server    │
│             │  chunked NDJSON    │             │
│ read file   │                    │ buffer N    │
│ stream lines│                    │ lines       │
│             │                    │ batch INSERT│
│ show progress│ <──────────────── │ stream progress
└─────────────┘  SSE-style events  └─────────────┘
```

**Key properties:**
- Constant memory on both sides
- File size unlimited
- Live progress feedback
- Batch inserts for performance

### Export Logic

```go
func runExport(url, token string, categories []string, since, until *time.Time, output io.Writer) error {
    client := eventodb.NewClient(url, token)
    encoder := json.NewEncoder(output)
    
    for _, category := range categories {
        position := int64(0)
        for {
            // Fetch batch from category
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
                // Apply time filter
                if since != nil && msg.Time.Before(*since) {
                    continue
                }
                if until != nil && !msg.Time.Before(*until) {
                    continue
                }
                
                // Write NDJSON line
                record := ExportRecord{
                    ID:     msg.ID,
                    Stream: msg.StreamName,
                    Type:   msg.Type,
                    Pos:    msg.Position,
                    GPos:   msg.GlobalPosition,
                    Data:   msg.Data,
                    Meta:   msg.Metadata,
                    Time:   msg.Time.Format(time.RFC3339),
                }
                if err := encoder.Encode(record); err != nil {
                    return err
                }
            }
            
            // Next batch
            position = messages[len(messages)-1].GlobalPosition + 1
        }
    }
    return nil
}
```

### Import Endpoint

```
POST /import
Authorization: Bearer <token>
Content-Type: application/x-ndjson
Transfer-Encoding: chunked

{"id":"...","stream":"workflow-123","type":"Started","pos":0,"gpos":47,"data":{...},"meta":null,"time":"..."}
{"id":"...","stream":"order-456","type":"Created","pos":0,"gpos":52,"data":{...},"meta":null,"time":"..."}
...
```

**Response** (streaming `text/event-stream`):
```
data: {"imported":1000,"gpos":1523}

data: {"imported":2000,"gpos":3042}

data: {"imported":3000,"gpos":4521}

data: {"done":true,"imported":3456,"elapsed":"2.3s"}
```

**Error mid-stream:**
```
data: {"imported":2000,"gpos":3042}

data: {"error":"POSITION_EXISTS","message":"global position 3050 already exists","line":2047}
```

### Import Server Handler

```go
func (h *ImportHandler) Handle(ctx *fasthttp.RequestCtx) {
    namespace := ctx.UserValue("namespace").(string)
    
    // Set up streaming response
    ctx.SetContentType("text/event-stream")
    ctx.Response.Header.Set("Cache-Control", "no-cache")
    ctx.Response.Header.Set("Transfer-Encoding", "chunked")
    
    ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
        reader := bufio.NewReader(bytes.NewReader(ctx.PostBody()))
        // For true streaming with fasthttp, use ctx.RequestBodyStream()
        
        batch := make([]*store.Message, 0, batchSize)
        imported := int64(0)
        lastGPos := int64(0)
        start := time.Now()
        lineNum := 0
        
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            lineNum++
            
            var record ExportRecord
            if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
                sendError(w, "INVALID_JSON", err.Error(), lineNum)
                return
            }
            
            msg := recordToMessage(&record)
            batch = append(batch, msg)
            lastGPos = msg.GlobalPosition
            
            // Batch full - flush to DB
            if len(batch) >= batchSize {
                if err := h.store.ImportBatch(ctx, namespace, batch); err != nil {
                    sendError(w, "IMPORT_FAILED", err.Error(), lineNum)
                    return
                }
                imported += int64(len(batch))
                batch = batch[:0]
                
                // Send progress
                sendProgress(w, imported, lastGPos)
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
        
        // Send done
        elapsed := time.Since(start)
        fmt.Fprintf(w, "data: {\"done\":true,\"imported\":%d,\"elapsed\":\"%s\"}\n\n", imported, elapsed)
        w.Flush()
    })
}

func sendProgress(w *bufio.Writer, imported, gpos int64) {
    fmt.Fprintf(w, "data: {\"imported\":%d,\"gpos\":%d}\n\n", imported, gpos)
    w.Flush()
}

func sendError(w *bufio.Writer, code, message string, line int) {
    fmt.Fprintf(w, "data: {\"error\":\"%s\",\"message\":\"%s\",\"line\":%d}\n\n", code, message, line)
    w.Flush()
}
```

### Import CLI Client

```go
func runImport(url, token string, inputPath string) error {
    file, err := os.Open(inputPath)
    if err != nil {
        return err
    }
    defer file.Close()
    
    // Get file size for progress bar (optional)
    stat, _ := file.Stat()
    fileSize := stat.Size()
    
    // Create request with streaming body
    req, err := http.NewRequest("POST", url+"/import", file)
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/x-ndjson")
    req.Header.Set("Transfer-Encoding", "chunked")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // Read SSE progress events
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Text()
        if !strings.HasPrefix(line, "data: ") {
            continue
        }
        
        data := line[6:]
        var progress ImportProgress
        json.Unmarshal([]byte(data), &progress)
        
        if progress.Error != "" {
            return fmt.Errorf("import failed at line %d: %s - %s", 
                progress.Line, progress.Error, progress.Message)
        }
        
        if progress.Done {
            fmt.Printf("\nDone. %d events imported in %s\n", progress.Imported, progress.Elapsed)
            return nil
        }
        
        // Update progress display
        fmt.Printf("\r  %d events imported (gpos: %d)", progress.Imported, progress.GPos)
    }
    
    return scanner.Err()
}
```

**CLI output:**
```
$ eventodb import --url http://localhost:8080 --token $TOKEN --input export.ndjson
Importing export.ndjson...
  3,456 events imported (gpos: 4521)
Done. 3456 events imported in 2.3s
```

### New Store Method

```go
// ImportBatch writes messages with explicit positions (for import/restore)
// Returns error if any globalPosition already exists in the namespace
// All messages in batch are inserted in a single transaction
ImportBatch(ctx context.Context, namespace string, messages []*Message) error
```

### Backend Implementation

**SQLite:**
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
            // Check for unique constraint violation
            if isUniqueViolation(err) {
                return fmt.Errorf("global position %d already exists", msg.GlobalPosition)
            }
            return err
        }
    }
    
    return tx.Commit()
}
```

**PostgreSQL:**
```go
func (s *PostgresStore) ImportBatch(ctx context.Context, namespace string, messages []*Message) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // Use COPY for best performance
    // Fallback to batch INSERT if COPY not available
    for _, msg := range messages {
        _, err := tx.ExecContext(ctx, `
            INSERT INTO messages (id, stream_name, type, position, global_position, data, metadata, time)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        `, msg.ID, msg.StreamName, msg.Type, msg.Position, 
           msg.GlobalPosition, msg.Data, msg.Metadata, msg.Time)
        if err != nil {
            if isUniqueViolation(err) {
                return fmt.Errorf("global position %d already exists", msg.GlobalPosition)
            }
            return err
        }
    }
    
    // Note: Do NOT update sequence - gaps are intentional
    return tx.Commit()
}
```

**Pebble:**
```go
func (s *PebbleStore) ImportBatch(ctx context.Context, namespace string, messages []*Message) error {
    batch := s.db.NewBatch()
    defer batch.Close()
    
    for _, msg := range messages {
        key := fmt.Sprintf("ns:%s:gpos:%019d", namespace, msg.GlobalPosition)
        
        // Check if exists
        _, closer, err := s.db.Get([]byte(key))
        if err == nil {
            closer.Close()
            return fmt.Errorf("global position %d already exists", msg.GlobalPosition)
        }
        
        encoded := encodeMessage(msg)
        batch.Set([]byte(key), encoded, nil)
        
        // Also set stream index key
        streamKey := fmt.Sprintf("ns:%s:stream:%s:pos:%019d", namespace, msg.StreamName, msg.Position)
        batch.Set([]byte(streamKey), []byte(key), nil) // pointer to main record
    }
    
    return batch.Commit(pebble.Sync)
}

---

## CLI Integration

Update `main.go` command handling:

```go
switch os.Args[1] {
case "serve":
    // ... existing
case "export":
    runExportCommand(os.Args[2:])
case "import":
    runImportCommand(os.Args[2:])
case "version", "--version", "-v":
    // ... existing
case "help", "--help", "-h":
    // ... existing
}
```

Update help text:

```
COMMANDS:
    serve                     Start the server
    export                    Export messages to NDJSON file
    import                    Import messages from NDJSON file
    version, -v, --version    Show version information
    help, -h, --help          Show this help message

EXPORT OPTIONS:
    --url <url>               EventoDB server URL
    --token <token>           Namespace token
    --categories <list>       Comma-separated category names
    --since <date>            Start date (YYYY-MM-DD or ISO 8601)
    --until <date>            End date (YYYY-MM-DD or ISO 8601)
    --output <file>           Output file (default: stdout)

IMPORT OPTIONS:
    --url <url>               EventoDB server URL
    --token <token>           Namespace token
    --input <file>            Input file (default: stdin)
```

## HTTP Endpoint

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/import` | POST | Streaming import with progress |

**Request:**
- `Content-Type: application/x-ndjson`
- `Authorization: Bearer <token>`
- Body: NDJSON stream (chunked transfer)

**Response:**
- `Content-Type: text/event-stream`
- SSE-style progress events

---

## Files to Modify

```
golang/
├── cmd/eventodb/
│   ├── main.go               # Add export/import command routing
│   ├── export.go             # New: export command implementation
│   └── import.go             # New: import command implementation
├── internal/store/
│   ├── store.go              # Add ImportBatch to interface
│   ├── sqlite/store.go       # Implement ImportBatch
│   ├── postgres/store.go     # Implement ImportBatch
│   └── pebble/store.go       # Implement ImportBatch
├── internal/api/
│   ├── server.go             # Register /import endpoint
│   └── import_handler.go     # New: streaming import handler
```

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Network error during export | Retry with exponential backoff, resume from last position |
| Invalid JSON line on import | Error with line number, abort |
| Duplicate global position | Error with position value, abort |
| Time parse error | Error with line number, abort |

---

## Future Enhancements

**Not in scope for this ADR:**

1. **Streaming export endpoint** - Currently CLI fetches via RPC batches. Could add `/export` endpoint for server-side streaming.

2. **Incremental export** - Track last exported position, only export new messages.

3. **Compression** - gzip support for large exports (`--gzip` flag).

4. **Resume on failure** - Track progress, allow resuming interrupted imports.

---

## Testing

1. **Unit tests:** `ImportMessage` for each backend
2. **Integration tests:** Export → Import roundtrip preserves data
3. **CLI tests:** Flag parsing, error handling
4. **Edge cases:**
   - Empty categories
   - Time filters that exclude all messages
   - Sparse positions (large gaps)
   - Unicode in event data

---

## References

- [ADR-004: Namespaces and Authentication](./ADR004-namespaces-and-auth.md)
- [EventoDB API Reference](../docs/API.md)
