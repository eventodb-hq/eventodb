-- Migration: Create namespace registry for TimescaleDB
-- Version: 001
-- Description: Creates the message_store schema and namespaces table for tracking namespace metadata
-- Note: This is identical to the Postgres version - TimescaleDB is Postgres-compatible

CREATE SCHEMA IF NOT EXISTS message_store;

CREATE TABLE IF NOT EXISTS message_store.namespaces (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    schema_name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at BIGINT NOT NULL,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_namespaces_token_hash ON message_store.namespaces(token_hash);
