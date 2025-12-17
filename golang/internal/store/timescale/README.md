# TimescaleDB Driver for Message-DB

This driver implements the Message-DB store interface using TimescaleDB's hypertable features for:
- **Time-based partitioning** (chunks)
- **Native compression** (10-20x storage savings)
- **Efficient data retention** (drop chunks, not rows)
- **S3 tiering preparation** (export chunks to object storage)

## Schema Design

### Key Design Decisions

#### 1. Partition by TIME, not Position

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        MESSAGES HYPERTABLE                               │
├──────────────┬──────────────┬──────────────┬──────────────┬─────────────┤
│   Chunk 1    │   Chunk 2    │   Chunk 3    │   Chunk 4    │  Chunk 5    │
│  Dec 1-7     │  Dec 8-14    │  Dec 15-21   │  Dec 22-28   │  Dec 29-... │
│  COMPRESSED  │  COMPRESSED  │  COMPRESSED  │     HOT      │    HOT      │
│  (S3 backed) │  (S3 backed) │              │              │             │
└──────────────┴──────────────┴──────────────┴──────────────┴─────────────┘
```

**Why time-based?**
- Aligns with data lifecycle (old data → compress → S3 → delete)
- Native chunk operations (`drop_chunks` vs expensive `DELETE`)
- Compression works best on time-ordered data
- S3 export is natural per time-range chunk

**Trade-off:** Category queries spanning large time ranges scan multiple chunks.
But in practice, consumers read from a position forward, typically hitting recent data.

#### 2. Unique Constraints Include Time

TimescaleDB requires the partitioning column (`time`) in all unique indexes.

```sql
-- Can't do this:
CREATE UNIQUE INDEX ON messages (stream_name, position);  -- ERROR!

-- Must do this:
CREATE UNIQUE INDEX ON messages (stream_name, position, time);
```

**Solution:** The `write_message` function uses advisory locks to enforce actual uniqueness:

```sql
PERFORM pg_advisory_xact_lock(hash_64(category(stream_name)));
-- Now safe to check version and insert
```

#### 3. Compression Settings

```sql
ALTER TABLE messages SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'stream_name',  -- Efficient stream queries
    timescaledb.compress_orderby = 'position ASC'    -- Sequential reads
);
```

Segmenting by `stream_name` means compressed chunks can still efficiently serve:
- `get_stream_messages('user-123')` → only decompresses user-123 segment

#### 4. Chunk Interval

Default: **7 days** per chunk

Adjust based on write volume:
- High volume (>10M msgs/day): Use 1 day chunks
- Low volume (<100K msgs/day): Use 30 day chunks

```sql
SELECT set_chunk_time_interval('schema.messages', INTERVAL '1 day');
```

## Data Lifecycle

### Hot → Warm → Cold → Archive Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│  HOT (0-7 days)      │  WARM (7-30 days)  │  COLD (30d-1y)  │  ARCHIVE  │
│  - Uncompressed      │  - Compressed      │  - Compressed   │  - S3     │
│  - Fast random I/O   │  - Good read perf  │  - Export to S3 │  - Drop   │
│  - Full indexes      │  - Indexes work    │  - Query via S3 │  - chunks │
└─────────────────────────────────────────────────────────────────────────┘
```

### Automatic Policies

```sql
-- Compress chunks older than 7 days (runs every 12 hours)
SELECT add_compression_policy('messages', INTERVAL '7 days');

-- Optional: Auto-delete chunks older than 1 year
SELECT add_retention_policy('messages', INTERVAL '1 year');
```

### Manual Operations

```sql
-- List all chunks with status
SELECT * FROM schema.get_chunks();

-- Manually compress old chunks
SELECT * FROM schema.compress_chunks_older_than(INTERVAL '3 days');

-- Drop chunks before backing up (call AFTER S3 export!)
SELECT * FROM schema.drop_chunks_older_than(INTERVAL '1 year');
```

## S3 Tiering Workflow

TimescaleDB Cloud has native S3 tiering. For self-hosted, use this workflow:

### 1. Export Chunk to CSV/Parquet

```sql
-- Export a specific chunk
COPY (
    SELECT * FROM _timescaledb_internal._hyper_1_5_chunk 
    ORDER BY global_position
) TO '/tmp/chunk_2024_w01.csv' WITH CSV HEADER;
```

Or use `pg_dump` for the chunk:
```bash
pg_dump -t '_timescaledb_internal._hyper_1_5_chunk' > chunk.sql
```

### 2. Upload to S3

```bash
aws s3 cp /tmp/chunk_2024_w01.csv s3://my-bucket/messages/2024/w01/
```

### 3. Drop the Chunk

```sql
SELECT drop_chunks('schema.messages', INTERVAL '1 year');
```

### 4. Query Historical Data

For querying S3 data, consider:
- **Athena/Presto** - Query CSV/Parquet directly
- **Foreign Data Wrapper** - `parquet_s3_fdw` for Postgres
- **Rehydrate on demand** - Load specific chunks back

## Performance Considerations

### Index Strategy

| Index | Purpose | Compressed Chunks |
|-------|---------|-------------------|
| `messages_stream_position` | Stream reads | ✅ Works (segmented by stream) |
| `messages_global_position` | Category reads | ⚠️ Slower (scans segments) |
| `messages_category_global` | Category reads | ⚠️ Slower (scans segments) |
| `messages_id_time` | Idempotency | ✅ Works |

### Query Patterns

**Fast on compressed data:**
```sql
-- Stream queries (leverages segment)
SELECT * FROM get_stream_messages('user-123', 0, 100);
```

**Slower on compressed data:**
```sql
-- Full category scan (decompresses all segments)
SELECT * FROM get_category_messages('user', 1, 10000);
```

**Optimize category queries with time bounds:**
```sql
-- Add time filter for better performance
SELECT * FROM messages 
WHERE category(stream_name) = 'user'
  AND global_position >= 1000
  AND time > NOW() - INTERVAL '7 days'  -- Limits chunks scanned
ORDER BY global_position
LIMIT 1000;
```

## Configuration

### Recommended postgresql.conf Settings

```conf
# TimescaleDB tuning
timescaledb.max_background_workers = 8
timescaledb.compress_orderby_default = 'time DESC'

# Memory for compression jobs
maintenance_work_mem = 1GB

# Parallel query for decompression  
max_parallel_workers_per_gather = 4
```

### Chunk Size Guidelines

| Write Rate | Chunk Interval | Approx Chunk Size |
|------------|----------------|-------------------|
| 100K/day   | 30 days        | ~500MB uncompressed |
| 1M/day     | 7 days         | ~1GB uncompressed |
| 10M/day    | 1 day          | ~1.5GB uncompressed |

After compression, expect 10-20x size reduction for event data.

## Comparison with Standard Postgres Driver

| Feature | Postgres | TimescaleDB |
|---------|----------|-------------|
| Write Performance | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ (slight overhead) |
| Stream Read | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Category Read | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ (compressed: ⭐⭐⭐) |
| Storage Cost | ⭐⭐ | ⭐⭐⭐⭐⭐ (10-20x less) |
| Data Retention | ⭐⭐ (slow DELETE) | ⭐⭐⭐⭐⭐ (drop chunks) |
| S3 Tiering | Manual | Native (Cloud) / Easy (Self-hosted) |
| Operational Complexity | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |

**Recommendation:** 
- Use standard Postgres for <10GB data or simple deployments
- Use TimescaleDB for >10GB data, long retention, or cost-sensitive workloads
