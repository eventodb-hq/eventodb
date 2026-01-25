-- Migration: 002
-- Description: Allow empty category in get_category_messages to return all messages
-- Required for: ADR-009 sparse export without category filter

-- Update get_category_messages to handle empty/null category
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
    WHERE (_category_name IS NULL OR _category_name = '' OR "{{SCHEMA_NAME}}".category(m.stream_name) = _category_name)
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

-- Record migration version
INSERT INTO "{{SCHEMA_NAME}}"._schema_version (version) VALUES (2) ON CONFLICT DO NOTHING;
