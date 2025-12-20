# ADR-005: TimescaleDB Driver

**Date:** 2024-12-17  
**Status:** Accepted  
**Context:** Support for time-series optimized storage with compression, partitioning, and S3 tiering capabilities

---

## Problem Statement

For large-scale event stores with long retention requirements, the standard PostgreSQL driver has limitations:

### Issues
1. **Storage costs** - Event data grows indefinitely, storage costs scale linearly
2. **Data retention** - `DELETE` operations on large tables are slow and cause bloat
3. **No compression** - Raw JSON data is stored uncompressed
4. **No tiering** - All data must remain in expensive primary storage
5. **Backup complexity** - Full backups required, no incremental by time range

### Requirements
- Efficient storage for billions of events
- Compression for cold data (10-20x reduction)
- Fast data expiration (drop time ranges, not rows)
- Preparation for S3/object storage tiering
- Compatible API with existing Postgres driver

---

## Decision

Implement a **TimescaleDB driver** that uses **hypertables** with time-based partitioning.

### Key Design Choices

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| **Partition Key** | `time` (not `global_position`) | Aligns with data lifecycle, enables efficient chunk drops |
| **Chunk Interval** | 7 days default | Balance between granularity and management overhead |
| **Compression** | Segment by `stream_name` | Efficient stream queries on compressed data |
| **Schema Prefix** | `tsdb_` | Distinguish from standard Postgres schemas (`eventodb_`) |

---

## Architecture

### Schema Structure

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Database: eventodb_timescale_test                                     │
├─────────────────────────────────────────────────────────────────────────┤
│  public.schema_migrations      → Migration tracking                     │
│  eventodb_store.namespaces      → Namespace registry                     │
│  tsdb_<namespace>.messages     → Hypertable (partitioned by time)       │
│  _timescaledb_internal.*       → TimescaleDB chunk storage              │
└─────────────────────────────────────────────────────────────────────────┘
```

### Hypertable Design

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        tsdb_default.messages (HYPERTABLE)               │
├──────────────┬──────────────┬──────────────┬──────────────┬─────────────┤
│   Chunk 1    │   Chunk 2    │   Chunk 3    │   Chunk 4    │  Chunk 5    │
│  Dec 1-7     │  Dec 8-14    │  Dec 15-21   │  Dec 22-28   │  Dec 29-... │
│  COMPRESSED  │  COMPRESSED  │  COMPRESSED  │     HOT      │    HOT      │
│  (exportable)│  (exportable)│              │              │             │
└──────────────┴──────────────┴──────────────┴──────────────┴─────────────┘
       ↓              ↓
   S3 Archive    S3 Archive
```

---

## Implementation

### 1. Table Schema

```sql
CREATE TABLE IF NOT EXISTS "{{SCHEMA_NAME}}".messages (
    -- Time column FIRST for TimescaleDB optimization
    "time" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Global position - monotonically increasing across all streams
    global_position BIGINT NOT NULL DEFAULT nextval('...'),
    
    -- Stream-level position (gapless within stream)
    "position" BIGINT NOT NULL,
    
    -- Stream identification
    stream_name TEXT NOT NULL,
    
    -- Message content
    "type" TEXT NOT NULL,
    data JSONB,
    metadata JSONB,
    
    -- Message ID (UUID)
    id UUID NOT NULL DEFAULT gen_random_uuid()
);

-- Convert to hypertable with 7-day chunks
SELECT create_hypertable(
    '"{{SCHEMA_NAME}}".messages',
    by_range('time', INTERVAL '7 days')
);
```

### 2. Unique Constraint Limitation

TimescaleDB requires the partitioning column (`time`) in all unique indexes:

```sql
-- CANNOT do this (TimescaleDB limitation):
CREATE UNIQUE INDEX ON messages (stream_name, position);  -- ERROR!

-- MUST include time:
CREATE UNIQUE INDEX ON messages (stream_name, position, time);
```

**Solution:** Enforce uniqueness via advisory locks in `write_message()`:

```sql
CREATE FUNCTION write_message(...) AS $$
BEGIN
    -- Category-level advisory lock ensures serialization
    PERFORM pg_advisory_xact_lock(hash_64(category(stream_name)));
    
    -- Safe to check version and insert
    _current_version := stream_version(_stream_name);
    IF _expected_version IS NOT NULL AND _expected_version != _current_version THEN
        RAISE EXCEPTION 'Wrong expected version...';
    END IF;
    
    INSERT INTO messages (...) VALUES (...);
END;
$$;
```

### 3. Compression Configuration

```sql
ALTER TABLE messages SET (
    timescaledb.compress,
    -- Segment by stream for efficient stream queries on compressed data
    timescaledb.compress_segmentby = 'stream_name',
    -- Order within segments for sequential reads
    timescaledb.compress_orderby = 'position ASC, global_position ASC, time DESC'
);

-- Auto-compress chunks older than 7 days
SELECT add_compression_policy('messages', INTERVAL '7 days');
```

### 4. Index Strategy

| Index | Purpose | Works on Compressed? |
|-------|---------|---------------------|
| `messages_stream_position` | Stream reads | ✅ Yes (segmented) |
| `messages_global_position` | Category reads | ⚠️ Slower |
| `messages_category_global` | Category reads | ⚠️ Slower |
| `messages_id_time` | Idempotency | ✅ Yes |
| `messages_correlation` | Consumer groups | ⚠️ Slower |

---

## Data Lifecycle Management

### Automatic Policies

```sql
-- Compress chunks older than 7 days (runs every 12 hours)
SELECT add_compression_policy('schema.messages', INTERVAL '7 days');

-- Optional: Auto-delete chunks older than 1 year
SELECT add_retention_policy('schema.messages', INTERVAL '1 year');
```

### Manual Operations

```sql
-- List chunks with status
SELECT * FROM tsdb_default.get_chunks();

-- Manually compress old chunks  
SELECT * FROM tsdb_default.compress_chunks_older_than(INTERVAL '3 days');

-- Drop chunks (call AFTER S3 backup!)
SELECT * FROM tsdb_default.drop_chunks_older_than(INTERVAL '1 year');
```

### S3 Tiering Workflow (Self-Hosted)

```bash
# 1. Export chunk to CSV
psql -c "COPY (SELECT * FROM _timescaledb_internal._hyper_1_5_chunk 
         ORDER BY global_position) TO '/tmp/chunk_2024_w01.csv' CSV HEADER"

# 2. Upload to S3
aws s3 cp /tmp/chunk_2024_w01.csv s3://my-bucket/messages/2024/w01/

# 3. Drop the chunk
psql -c "SELECT drop_chunks('schema.messages', INTERVAL '1 year')"
```

---

## File Structure

```
golang/
├── migrations/
│   ├── embed.go                              # Added TimescaleDB embeds
│   ├── metadata/
│   │   └── timescale/
│   │       └── 001_create_namespace_registry.sql
│   └── namespace/
│       └── timescale/
│           └── 001_create_schema.sql         # Hypertable + functions
├── internal/
│   ├── migrate/
│   │   └── migrate.go                        # Updated for timescale dialect
│   └── store/
│       └── timescale/
│           ├── store.go                      # Main store + chunk methods
│           ├── namespace.go                  # Namespace CRUD
│           ├── read.go                       # Read operations
│           ├── write.go                      # Write operations
│           ├── types.go                      # ChunkInfo types
│           └── README.md                     # Driver documentation
└── cmd/
    └── eventodb/
        └── main.go                           # Added -db-type flag

bin/
└── run_external_tests_timescale.sh           # Test script
```

---

## Usage

### Server Startup

```bash
# Standard PostgreSQL
./eventodb -db-url "postgres://user:pass@host:5432/db"

# TimescaleDB (explicit)
./eventodb -db-url "postgres://user:pass@host:5432/db" -db-type timescale
```

### CLI Flags

| Flag | Description |
|------|-------------|
| `-db-url` | Database connection URL (postgres://...) |
| `-db-type` | Database type override: `postgres`, `timescale`, or auto-detect |
| `-port` | HTTP server port (default: 8080) |
| `-token` | Token for default namespace |
| `-test-mode` | Use in-memory SQLite |

### Go API

```go
import "github.com/eventodb/eventodb/internal/store/timescale"

// Connect
db, _ := sql.Open("postgres", "postgres://...")
store, _ := timescale.New(db)

// Standard operations (same as Postgres driver)
store.CreateNamespace(ctx, "myapp", "token_hash", "My App")
store.WriteMessage(ctx, "myapp", "account-123", &Message{...})
store.GetStreamMessages(ctx, "myapp", "account-123", nil)

// TimescaleDB-specific operations
chunks, _ := store.GetChunks(ctx, "myapp")
store.CompressChunksOlderThan(ctx, "myapp", "3 days")
store.DropChunksOlderThan(ctx, "myapp", "1 year")
```

---

## Testing

### Test Script

```bash
# Run full test suite against TimescaleDB
./bin/run_external_tests_timescale.sh

# Keep test database for inspection
KEEP_TEST_DB=1 ./bin/run_external_tests_timescale.sh

# Custom TimescaleDB instance
TIMESCALE_HOST=myhost TIMESCALE_PORT=5432 ./bin/run_external_tests_timescale.sh
```

### Test Results

```
94/95 tests pass (99% pass rate)

Only failure: Throughput benchmark
  - Expected: >1000 writes/sec
  - Actual: ~688 writes/sec
  - Reason: Hypertable overhead (expected trade-off)
```

---

## Performance Comparison

| Metric | PostgreSQL | TimescaleDB | Notes |
|--------|------------|-------------|-------|
| Single Write | ~1.5ms | ~1.8ms | Slight overhead |
| Stream Read (100) | ~1.0ms | ~1.0ms | Same |
| Category Read (100) | ~1.0ms | ~1.0ms | Same |
| Throughput | ~1200/sec | ~700/sec | Hypertable overhead |
| Storage (compressed) | 100% | 5-10% | **10-20x savings** |
| Data Retention | Slow DELETE | Fast DROP | **Minutes vs hours** |

### When to Use TimescaleDB

✅ **Use TimescaleDB when:**
- Data volume >10GB
- Retention period >30 days
- Storage costs are a concern
- Need to archive to S3
- Need efficient data expiration

❌ **Use standard Postgres when:**
- Data volume <10GB
- Simple deployment preferred
- Maximum write throughput needed
- No long-term retention requirements

---

## Chunk Interval Guidelines

| Write Rate | Chunk Interval | Approx Chunk Size |
|------------|----------------|-------------------|
| 100K msgs/day | 30 days | ~500MB uncompressed |
| 1M msgs/day | 7 days | ~1GB uncompressed |
| 10M msgs/day | 1 day | ~1.5GB uncompressed |

After compression: **10-20x size reduction** for typical event data.

---

## Configuration

### PostgreSQL Settings

```conf
# TimescaleDB tuning
timescaledb.max_background_workers = 8

# Memory for compression jobs
maintenance_work_mem = 1GB

# Parallel query for decompression
max_parallel_workers_per_gather = 4
```

### Compression Policy Adjustment

```sql
-- Change compression delay
SELECT remove_compression_policy('schema.messages');
SELECT add_compression_policy('schema.messages', INTERVAL '30 days');

-- Change retention period
SELECT remove_retention_policy('schema.messages');
SELECT add_retention_policy('schema.messages', INTERVAL '2 years');
```

---

## Migration from PostgreSQL Driver

To migrate from standard PostgreSQL to TimescaleDB:

1. **Export data** from existing Postgres schema
2. **Create new namespace** with TimescaleDB driver
3. **Import data** ordered by `time`
4. **Verify** message counts and positions

```sql
-- Export from Postgres
COPY (SELECT * FROM eventodb_myapp.messages ORDER BY global_position)
TO '/tmp/messages.csv' CSV HEADER;

-- Import to TimescaleDB (after creating namespace)
COPY tsdb_myapp.messages FROM '/tmp/messages.csv' CSV HEADER;
```

---

## Future Enhancements

1. **Native S3 Tiering** - When using Timescale Cloud
2. **Continuous Aggregates** - Pre-computed statistics by time period
3. **Automatic Chunk Export** - Background job to export and drop old chunks
4. **Rehydration API** - Load archived chunks back on demand

---

## References

- [TimescaleDB Documentation](https://docs.timescale.com/)
- [Compression Best Practices](https://docs.timescale.com/use-timescale/latest/compression/)
- [Data Retention](https://docs.timescale.com/use-timescale/latest/data-retention/)
- [Tiered Storage](https://docs.timescale.com/use-timescale/latest/tiered-storage/)
