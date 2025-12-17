-- Migration: Create namespace registry for SQLite
-- Version: 001
-- Description: Creates the namespaces table for tracking namespace metadata

CREATE TABLE IF NOT EXISTS namespaces (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    db_path TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at INTEGER NOT NULL,
    metadata TEXT  -- JSON as TEXT
);

CREATE INDEX IF NOT EXISTS idx_namespaces_token_hash ON namespaces(token_hash);
