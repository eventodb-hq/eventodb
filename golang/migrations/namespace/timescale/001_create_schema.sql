-- Migration: Create Message DB schema for namespace (TimescaleDB)
-- Version: 001
-- Description: Creates the messages hypertable with compression and time-based partitioning
-- Template: Replace {{SCHEMA_NAME}} with actual schema name
--
-- DESIGN DECISIONS:
-- 1. Hypertable partitioned by TIME (not position) for:
--    - Efficient chunk-based retention (drop_chunks vs DELETE)
--    - Native compression on time-ordered data
--    - S3 tiering per time-range chunks
--    - Natural alignment with data lifecycle
--
-- 2. Chunk interval: 7 days (adjust based on write volume)
--    - 1M msgs/day → 7 day chunks (~7M messages each)
--    - Smaller chunks = more granular retention but more overhead
--
-- 3. Compression segmented by stream_name for efficient stream queries
--
-- 4. Unique constraints include time (TimescaleDB requirement)
--    - Actual uniqueness enforced by write_message function via advisory locks

CREATE SCHEMA IF NOT EXISTS "{{SCHEMA_NAME}}";

-- =============================================================================
-- SEQUENCES
-- =============================================================================

CREATE SEQUENCE IF NOT EXISTS "{{SCHEMA_NAME}}".messages_global_position_seq;

-- =============================================================================
-- MESSAGES HYPERTABLE
-- =============================================================================

CREATE TABLE IF NOT EXISTS "{{SCHEMA_NAME}}".messages (
    -- Time column FIRST for TimescaleDB optimization
    "time" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Global position - monotonically increasing across all streams
    global_position BIGINT NOT NULL DEFAULT nextval('"{{SCHEMA_NAME}}".messages_global_position_seq'),
    
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
    by_range('time', INTERVAL '7 days'),
    if_not_exists => TRUE,
    migrate_data => TRUE
);

-- =============================================================================
-- INDEXES
-- =============================================================================

-- Stream position uniqueness (includes time for hypertable constraint)
-- Actual uniqueness enforced by write_message function
CREATE UNIQUE INDEX IF NOT EXISTS messages_stream_position_time 
    ON "{{SCHEMA_NAME}}".messages (stream_name, "position", "time");

-- Message ID uniqueness (includes time)
CREATE UNIQUE INDEX IF NOT EXISTS messages_id_time 
    ON "{{SCHEMA_NAME}}".messages (id, "time");

-- Stream position lookup (non-unique, for efficient stream queries)
CREATE INDEX IF NOT EXISTS messages_stream_position 
    ON "{{SCHEMA_NAME}}".messages (stream_name, "position");

-- Category + global position for category reads
CREATE INDEX IF NOT EXISTS messages_category_global 
    ON "{{SCHEMA_NAME}}".messages (
        (SPLIT_PART(stream_name, '-', 1)),
        global_position
    );

-- Global position ordering (for category reads without time filter)
CREATE INDEX IF NOT EXISTS messages_global_position 
    ON "{{SCHEMA_NAME}}".messages (global_position);

-- Correlation filtering (for consumer group queries)
CREATE INDEX IF NOT EXISTS messages_correlation 
    ON "{{SCHEMA_NAME}}".messages ((metadata->>'correlationStreamName'), global_position)
    WHERE metadata->>'correlationStreamName' IS NOT NULL;

-- =============================================================================
-- COMPRESSION SETTINGS
-- =============================================================================

-- Enable compression with optimal settings for event store workload
ALTER TABLE "{{SCHEMA_NAME}}".messages SET (
    timescaledb.compress,
    -- Segment by stream for efficient stream queries on compressed data
    timescaledb.compress_segmentby = 'stream_name',
    -- Order within segments for sequential reads
    timescaledb.compress_orderby = 'position ASC, global_position ASC, time DESC'
);

-- Add compression policy: compress chunks older than 7 days
-- This gives you a week of "hot" data for fast random access
SELECT add_compression_policy(
    '"{{SCHEMA_NAME}}".messages', 
    INTERVAL '7 days',
    if_not_exists => TRUE
);

-- =============================================================================
-- UTILITY FUNCTIONS (Message DB compatible)
-- =============================================================================

-- hash_64: Computes a 64-bit hash compatible with Message DB
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".hash_64(value TEXT)
RETURNS BIGINT AS $$
    SELECT ('x' || left(md5(value), 16))::bit(64)::bigint;
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;

-- category: Extracts category from stream name
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".category(stream_name TEXT)
RETURNS TEXT AS $$
    SELECT SPLIT_PART(stream_name, '-', 1);
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;

-- id: Extracts ID portion from stream name
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".id(stream_name TEXT)
RETURNS TEXT AS $$
    SELECT CASE 
        WHEN STRPOS(stream_name, '-') = 0 THEN NULL
        ELSE SUBSTRING(stream_name FROM STRPOS(stream_name, '-') + 1)
    END;
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;

-- cardinal_id: Extracts cardinal ID (before '+') for compound IDs
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".cardinal_id(stream_name TEXT)
RETURNS TEXT AS $$
    SELECT SPLIT_PART("{{SCHEMA_NAME}}".id(stream_name), '+', 1);
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;

-- is_category: Determines if name represents a category (no ID part)
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".is_category(stream_name TEXT)
RETURNS BOOLEAN AS $$
    SELECT STRPOS(stream_name, '-') = 0;
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;

-- acquire_lock: Acquires category-level advisory lock
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".acquire_lock(stream_name TEXT)
RETURNS BIGINT AS $$
DECLARE
    _category TEXT;
    _hash BIGINT;
BEGIN
    _category := "{{SCHEMA_NAME}}".category(stream_name);
    _hash := "{{SCHEMA_NAME}}".hash_64(_category);
    PERFORM pg_advisory_xact_lock(_hash);
    RETURN _hash;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- stream_version: Gets the current version of a stream
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".stream_version(_stream_name TEXT)
RETURNS BIGINT AS $$
    SELECT COALESCE(MAX("position"), -1)
    FROM "{{SCHEMA_NAME}}".messages
    WHERE stream_name = _stream_name;
$$ LANGUAGE sql STABLE;

-- =============================================================================
-- WRITE FUNCTION
-- =============================================================================

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".write_message(
    _id VARCHAR,
    _stream_name TEXT,
    _type TEXT,
    _data JSONB,
    _metadata JSONB DEFAULT NULL,
    _expected_version BIGINT DEFAULT NULL
)
RETURNS BIGINT AS $$
DECLARE
    _position BIGINT;
    _current_version BIGINT;
BEGIN
    -- Acquire category-level lock for consistency
    PERFORM "{{SCHEMA_NAME}}".acquire_lock(_stream_name);
    
    -- Get current stream version
    _current_version := "{{SCHEMA_NAME}}".stream_version(_stream_name);
    
    -- Check expected version (optimistic locking)
    IF _expected_version IS NOT NULL AND _expected_version != _current_version THEN
        RAISE EXCEPTION 'Wrong expected version: % (Stream: %, Stream Version: %)',
            _expected_version, _stream_name, _current_version
            USING ERRCODE = 'P0003';
    END IF;
    
    -- Calculate next position
    _position := _current_version + 1;
    
    -- Insert message (global_position auto-assigned by sequence default)
    INSERT INTO "{{SCHEMA_NAME}}".messages
        (id, stream_name, "type", "position", data, metadata)
    VALUES
        (_id::uuid, _stream_name, _type, _position, _data, _metadata);
    
    RETURN _position;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- =============================================================================
-- READ FUNCTIONS
-- =============================================================================

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_stream_messages(
    _stream_name TEXT,
    _position BIGINT DEFAULT 0,
    _batch_size BIGINT DEFAULT 1000,
    _condition TEXT DEFAULT NULL  -- Deprecated, ignored
)
RETURNS TABLE (
    id UUID,
    stream_name TEXT,
    "type" TEXT,
    "position" BIGINT,
    global_position BIGINT,
    data JSONB,
    metadata JSONB,
    "time" TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT m.id, m.stream_name, m.type, m.position, m.global_position, 
           m.data, m.metadata, m.time
    FROM "{{SCHEMA_NAME}}".messages m
    WHERE m.stream_name = _stream_name
      AND m.position >= _position
    ORDER BY m.position ASC
    LIMIT CASE WHEN _batch_size = -1 THEN NULL ELSE _batch_size END;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_category_messages(
    _category_name TEXT,
    _position BIGINT DEFAULT 1,
    _batch_size BIGINT DEFAULT 1000,
    _correlation TEXT DEFAULT NULL,
    _consumer_group_member BIGINT DEFAULT NULL,
    _consumer_group_size BIGINT DEFAULT NULL,
    _condition TEXT DEFAULT NULL  -- Deprecated, ignored
)
RETURNS TABLE (
    id UUID,
    stream_name TEXT,
    "type" TEXT,
    "position" BIGINT,
    global_position BIGINT,
    data JSONB,
    metadata JSONB,
    "time" TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT m.id, m.stream_name, m.type, m.position, m.global_position,
           m.data, m.metadata, m.time
    FROM "{{SCHEMA_NAME}}".messages m
    WHERE "{{SCHEMA_NAME}}".category(m.stream_name) = _category_name
      AND m.global_position >= _position
      AND (_correlation IS NULL OR 
           "{{SCHEMA_NAME}}".category(m.metadata->>'correlationStreamName') = _correlation)
      AND (_consumer_group_member IS NULL OR _consumer_group_size IS NULL OR
           MOD(ABS("{{SCHEMA_NAME}}".hash_64("{{SCHEMA_NAME}}".cardinal_id(m.stream_name))), _consumer_group_size) = _consumer_group_member)
    ORDER BY m.global_position ASC
    LIMIT CASE WHEN _batch_size = -1 THEN NULL ELSE _batch_size END;
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_last_stream_message(
    _stream_name TEXT,
    _type TEXT DEFAULT NULL
)
RETURNS TABLE (
    id UUID,
    stream_name TEXT,
    "type" TEXT,
    "position" BIGINT,
    global_position BIGINT,
    data JSONB,
    metadata JSONB,
    "time" TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT m.id, m.stream_name, m.type, m.position, m.global_position,
           m.data, m.metadata, m.time
    FROM "{{SCHEMA_NAME}}".messages m
    WHERE m.stream_name = _stream_name
      AND (_type IS NULL OR m.type = _type)
    ORDER BY m.position DESC
    LIMIT 1;
END;
$$ LANGUAGE plpgsql STABLE;

-- =============================================================================
-- DATA LIFECYCLE MANAGEMENT FUNCTIONS
-- =============================================================================

-- Get chunk information for this namespace
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_chunks()
RETURNS TABLE (
    chunk_name TEXT,
    range_start TIMESTAMPTZ,
    range_end TIMESTAMPTZ,
    is_compressed BOOLEAN,
    size_bytes BIGINT,
    message_count BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        c.chunk_name::TEXT,
        c.range_start,
        c.range_end,
        c.is_compressed,
        pg_total_relation_size(format('%I.%I', c.chunk_schema, c.chunk_name)::regclass),
        (SELECT COUNT(*) FROM "{{SCHEMA_NAME}}".messages m 
         WHERE m.time >= c.range_start AND m.time < c.range_end)
    FROM timescaledb_information.chunks c
    WHERE c.hypertable_schema = '{{SCHEMA_NAME}}'
      AND c.hypertable_name = 'messages'
    ORDER BY c.range_start DESC;
END;
$$ LANGUAGE plpgsql STABLE;

-- Drop chunks older than specified interval (for manual retention)
-- Call AFTER backing up to S3!
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".drop_chunks_older_than(_interval INTERVAL)
RETURNS SETOF TEXT AS $$
BEGIN
    RETURN QUERY
    SELECT drop_chunks('"{{SCHEMA_NAME}}".messages', NOW() - _interval)::TEXT;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- Manually compress a specific time range (for on-demand compression)
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".compress_chunks_older_than(_interval INTERVAL)
RETURNS TABLE(chunk_name TEXT, compressed BOOLEAN) AS $$
DECLARE
    _chunk RECORD;
BEGIN
    FOR _chunk IN
        SELECT c.chunk_schema, c.chunk_name
        FROM timescaledb_information.chunks c
        WHERE c.hypertable_schema = '{{SCHEMA_NAME}}'
          AND c.hypertable_name = 'messages'
          AND c.is_compressed = FALSE
          AND c.range_end < NOW() - _interval
    LOOP
        PERFORM compress_chunk(format('%I.%I', _chunk.chunk_schema, _chunk.chunk_name)::regclass);
        chunk_name := _chunk.chunk_name;
        compressed := TRUE;
        RETURN NEXT;
    END LOOP;
END;
$$ LANGUAGE plpgsql VOLATILE;
