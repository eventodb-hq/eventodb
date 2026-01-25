-- Migration: Create Message DB schema for namespace (SQLite)
-- Version: 001
-- Description: Creates the messages table for SQLite
-- Note: Utility functions are implemented in Go, not SQL

CREATE TABLE IF NOT EXISTS messages (
    id TEXT NOT NULL,
    stream_name TEXT NOT NULL,
    type TEXT NOT NULL,
    position INTEGER NOT NULL,
    global_position INTEGER PRIMARY KEY AUTOINCREMENT,
    data TEXT,  -- JSON as TEXT
    metadata TEXT,  -- JSON as TEXT
    time INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS messages_id ON messages (id);
CREATE UNIQUE INDEX IF NOT EXISTS messages_stream ON messages (stream_name, position);
CREATE INDEX IF NOT EXISTS messages_category ON messages (
    substr(stream_name, 1, CASE WHEN instr(stream_name, '-') > 0 THEN instr(stream_name, '-') - 1 ELSE length(stream_name) END),
    global_position
);

-- Schema version tracking table
CREATE TABLE IF NOT EXISTS _schema_version (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER DEFAULT (strftime('%s', 'now'))
);

-- Record initial schema version
INSERT OR IGNORE INTO _schema_version (version) VALUES (1);
