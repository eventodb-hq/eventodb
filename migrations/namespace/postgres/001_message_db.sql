-- Migration: Create Message DB schema for namespace
-- Version: 001
-- Description: Creates the messages table and all utility functions compatible with Message DB
-- Template: Replace {{SCHEMA_NAME}} with actual schema name

CREATE SCHEMA IF NOT EXISTS "{{SCHEMA_NAME}}";

-- Messages table
CREATE TABLE IF NOT EXISTS "{{SCHEMA_NAME}}".messages (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    stream_name VARCHAR NOT NULL,
    type VARCHAR NOT NULL,
    "position" BIGINT NOT NULL,
    global_position BIGSERIAL NOT NULL,
    data JSONB,
    metadata JSONB,
    "time" TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT (now() AT TIME ZONE 'utc'),
    PRIMARY KEY (global_position)
);

CREATE UNIQUE INDEX IF NOT EXISTS messages_id ON "{{SCHEMA_NAME}}".messages (id);
CREATE UNIQUE INDEX IF NOT EXISTS messages_stream ON "{{SCHEMA_NAME}}".messages (stream_name, position);
CREATE INDEX IF NOT EXISTS messages_category ON "{{SCHEMA_NAME}}".messages (
    (SPLIT_PART(stream_name, '-', 1)),
    global_position,
    ((metadata->>'correlationStreamName'))
);

-- Utility Functions (Message DB compatible)

-- hash_64: Computes a 64-bit hash compatible with Message DB
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".hash_64(value VARCHAR)
RETURNS BIGINT AS $$
DECLARE
    _hash BIGINT;
BEGIN
    -- Uses MD5, takes left 64 bits (8 bytes), converts to bigint
    -- Compatible with Message DB hash implementation
    SELECT ('x' || left(md5(value), 16))::bit(64)::bigint INTO _hash;
    RETURN _hash;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- category: Extracts category from stream name
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".category(stream_name VARCHAR)
RETURNS VARCHAR AS $$
BEGIN
    RETURN SPLIT_PART(stream_name, '-', 1);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- id: Extracts ID portion from stream name
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".id(stream_name VARCHAR)
RETURNS VARCHAR AS $$
DECLARE
    _id_separator_position INTEGER;
BEGIN
    _id_separator_position := STRPOS(stream_name, '-');
    IF _id_separator_position = 0 THEN
        RETURN NULL;
    END IF;
    RETURN SUBSTRING(stream_name, _id_separator_position + 1);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- cardinal_id: Extracts cardinal ID (before '+') for compound IDs
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".cardinal_id(stream_name VARCHAR)
RETURNS VARCHAR AS $$
DECLARE
    _id VARCHAR;
BEGIN
    _id := "{{SCHEMA_NAME}}".id(stream_name);
    IF _id IS NULL THEN
        RETURN NULL;
    END IF;
    -- Extract part before '+' for compound IDs (e.g., '123+456' -> '123')
    RETURN SPLIT_PART(_id, '+', 1);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- is_category: Determines if name represents a category (no ID part)
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".is_category(stream_name VARCHAR)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN STRPOS(stream_name, '-') = 0;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- acquire_lock: Acquires category-level advisory lock
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".acquire_lock(stream_name VARCHAR)
RETURNS BIGINT AS $$
DECLARE
    _category VARCHAR;
    _category_name_hash BIGINT;
BEGIN
    _category := "{{SCHEMA_NAME}}".category(stream_name);
    _category_name_hash := "{{SCHEMA_NAME}}".hash_64(_category);
    -- Advisory lock at CATEGORY level (not stream level)
    PERFORM pg_advisory_xact_lock(_category_name_hash);
    RETURN _category_name_hash;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- write_message: Writes a message to a stream with optimistic locking
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".write_message(
    _id VARCHAR,
    _stream_name VARCHAR,
    _type VARCHAR,
    _data JSONB,
    _metadata JSONB DEFAULT NULL,
    _expected_version BIGINT DEFAULT NULL
)
RETURNS BIGINT AS $$
DECLARE
    _position BIGINT;
    _current_version BIGINT;
    _lock_hash BIGINT;
BEGIN
    -- Acquire category-level lock
    _lock_hash := "{{SCHEMA_NAME}}".acquire_lock(_stream_name);
    
    -- Get current stream version
    SELECT COALESCE(MAX(position), -1)
    INTO _current_version
    FROM "{{SCHEMA_NAME}}".messages
    WHERE stream_name = _stream_name;
    
    -- Check expected version if provided (optimistic locking)
    IF _expected_version IS NOT NULL AND _expected_version != _current_version THEN
        RAISE EXCEPTION 'Wrong expected version: % (Stream: %, Stream Version: %)',
            _expected_version, _stream_name, _current_version
            USING ERRCODE = 'P0003'; -- raise_exception error code
    END IF;
    
    -- Calculate next position
    _position := _current_version + 1;
    
    -- Insert message
    INSERT INTO "{{SCHEMA_NAME}}".messages
        (id, stream_name, type, position, data, metadata)
    VALUES
        (_id::uuid, _stream_name, _type, _position, _data, _metadata);
    
    RETURN _position;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- get_stream_messages: Retrieves messages from a stream
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_stream_messages(
    _stream_name VARCHAR,
    _position BIGINT DEFAULT 0,
    _batch_size BIGINT DEFAULT 1000,
    _condition VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    id UUID,
    stream_name VARCHAR,
    type VARCHAR,
    "position" BIGINT,
    global_position BIGINT,
    data JSONB,
    metadata JSONB,
    "time" TIMESTAMP
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        m.id,
        m.stream_name,
        m.type,
        m.position,
        m.global_position,
        m.data,
        m.metadata,
        m.time
    FROM "{{SCHEMA_NAME}}".messages m
    WHERE m.stream_name = _stream_name
      AND m.position >= _position
    ORDER BY m.position ASC
    LIMIT CASE WHEN _batch_size = -1 THEN NULL ELSE _batch_size END;
END;
$$ LANGUAGE plpgsql STABLE;

-- get_category_messages: Retrieves messages from a category with consumer group support
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_category_messages(
    _category_name VARCHAR,
    _position BIGINT DEFAULT 1,
    _batch_size BIGINT DEFAULT 1000,
    _correlation VARCHAR DEFAULT NULL,
    _consumer_group_member BIGINT DEFAULT NULL,
    _consumer_group_size BIGINT DEFAULT NULL,
    _condition VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    id UUID,
    stream_name VARCHAR,
    type VARCHAR,
    "position" BIGINT,
    global_position BIGINT,
    data JSONB,
    metadata JSONB,
    "time" TIMESTAMP
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        m.id,
        m.stream_name,
        m.type,
        m.position,
        m.global_position,
        m.data,
        m.metadata,
        m.time
    FROM "{{SCHEMA_NAME}}".messages m
    WHERE "{{SCHEMA_NAME}}".category(m.stream_name) = _category_name
      AND m.global_position >= _position
      AND (_correlation IS NULL OR "{{SCHEMA_NAME}}".category(m.metadata->>'correlationStreamName') = _correlation)
      AND (
          _consumer_group_member IS NULL OR
          _consumer_group_size IS NULL OR
          MOD(ABS("{{SCHEMA_NAME}}".hash_64("{{SCHEMA_NAME}}".cardinal_id(m.stream_name))), _consumer_group_size) = _consumer_group_member
      )
    ORDER BY m.global_position ASC
    LIMIT CASE WHEN _batch_size = -1 THEN NULL ELSE _batch_size END;
END;
$$ LANGUAGE plpgsql STABLE;

-- get_last_stream_message: Retrieves the last message from a stream
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".get_last_stream_message(
    _stream_name VARCHAR,
    _type VARCHAR DEFAULT NULL
)
RETURNS TABLE (
    id UUID,
    stream_name VARCHAR,
    type VARCHAR,
    "position" BIGINT,
    global_position BIGINT,
    data JSONB,
    metadata JSONB,
    "time" TIMESTAMP
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        m.id,
        m.stream_name,
        m.type,
        m.position,
        m.global_position,
        m.data,
        m.metadata,
        m.time
    FROM "{{SCHEMA_NAME}}".messages m
    WHERE m.stream_name = _stream_name
      AND (_type IS NULL OR m.type = _type)
    ORDER BY m.position DESC
    LIMIT 1;
END;
$$ LANGUAGE plpgsql STABLE;

-- stream_version: Gets the current version of a stream
CREATE OR REPLACE FUNCTION "{{SCHEMA_NAME}}".stream_version(
    _stream_name VARCHAR
)
RETURNS BIGINT AS $$
DECLARE
    _version BIGINT;
BEGIN
    SELECT COALESCE(MAX(position), -1)
    INTO _version
    FROM "{{SCHEMA_NAME}}".messages
    WHERE stream_name = _stream_name;
    
    RETURN _version;
END;
$$ LANGUAGE plpgsql STABLE;
