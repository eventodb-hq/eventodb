-- Migration: 002
-- Description: SQLite schema update for empty category support
-- Required for: ADR-009 sparse export without category filter
-- Note: The actual fix is in Go code (sqlite/read.go), this just tracks version

-- Record migration version
INSERT OR IGNORE INTO _schema_version (version) VALUES (2);
