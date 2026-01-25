# ADR-010: Namespace Schema Versioning

**Date:** 2026-01-25  
**Status:** Proposed  
**Context:** Enable incremental migrations for per-namespace schemas

---

## Problem

EventoDB creates a separate schema (Postgres) or database file (SQLite) for each namespace. These schemas contain stored procedures and tables that may need updates over time.

**Current behavior:**
- `ApplyNamespaceMigration()` runs the full `001_create_schema.sql` on namespace creation
- No version tracking per namespace
- No way to apply incremental updates to existing namespaces
- Bug fixes to stored procedures require manual intervention

**Example issue:**
```sql
-- Original get_category_messages (in production namespaces)
WHERE category(stream_name) = _category_name  -- Doesn't handle empty category

-- Fixed version (ADR-009 export all)
WHERE (_category_name IS NULL OR _category_name = '' OR category(stream_name) = _category_name)
```

Existing namespaces have the old stored procedure. New namespaces get the fix. No automatic way to update.

---

## Decision

**Add per-namespace schema versioning with automatic migration on startup.**

Each namespace tracks its schema version independently. On server startup, all existing namespaces are checked and upgraded if needed.

---

## Design

### 1. Schema Version Table

**Postgres** (per namespace schema):
```sql
CREATE TABLE IF NOT EXISTS "{{SCHEMA_NAME}}"._schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT NOW()
);
```

**SQLite** (per namespace database file):
```sql
CREATE TABLE IF NOT EXISTS _schema_version (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER DEFAULT (strftime('%s', 'now'))
);
```

### 2. Migration File Structure

```
golang/migrations/namespace/
├── postgres/
│   ├── 001_create_schema.sql      # Initial full schema (existing)
│   ├── 002_category_get_all.sql   # Fix: empty category = all messages
│   └── 003_xxx.sql                # Future migrations
└── sqlite/
    ├── 001_create_schema.sql      # Initial full schema (existing)
    ├── 002_category_get_all.sql   # Fix: empty category = all messages
    └── 003_xxx.sql
```

### 3. Migration File Format

Each migration file contains:
1. Header comment with version and description
2. Idempotent SQL (CREATE OR REPLACE, IF NOT EXISTS)
3. Version tracking insert

**Example: `002_category_get_all.sql` (Postgres)**
```sql
-- Migration: 002
-- Description: Allow empty category to return all messages

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
    SELECT m.id, m.stream_name, m.type, m.position, m.global_position,
           m.data, m.metadata, m.time
    FROM "{{SCHEMA_NAME}}".messages m
    WHERE (_category_name IS NULL OR _category_name = '' 
           OR "{{SCHEMA_NAME}}".category(m.stream_name) = _category_name)
      AND m.global_position >= _position
      AND (_correlation IS NULL 
           OR "{{SCHEMA_NAME}}".category(m.metadata->>'correlationStreamName') = _correlation)
      AND (_consumer_group_member IS NULL OR _consumer_group_size IS NULL
           OR MOD(ABS("{{SCHEMA_NAME}}".hash_64("{{SCHEMA_NAME}}".cardinal_id(m.stream_name))), 
                  _consumer_group_size) = _consumer_group_member)
    ORDER BY m.global_position ASC
    LIMIT CASE WHEN _batch_size = -1 THEN NULL ELSE _batch_size END;
END;
$$ LANGUAGE plpgsql STABLE;

-- Record migration version
INSERT INTO "{{SCHEMA_NAME}}"._schema_version (version) 
VALUES (2) ON CONFLICT DO NOTHING;
```

### 4. Migration Logic

**On namespace creation:**
1. Apply all migrations in order (001, 002, 003...)
2. Each migration records its version

**On server startup:**
1. List all existing namespaces
2. For each namespace:
   - Query `_schema_version` for max version
   - If table doesn't exist, assume version 0
   - Apply all migrations > current version

```go
func (s *Store) MigrateAllNamespaces(ctx context.Context) error {
    namespaces, err := s.ListNamespaces(ctx)
    if err != nil {
        return err
    }
    
    for _, ns := range namespaces {
        if err := s.MigrateNamespace(ctx, ns.ID); err != nil {
            return fmt.Errorf("migrate namespace %s: %w", ns.ID, err)
        }
    }
    return nil
}

func (s *Store) MigrateNamespace(ctx context.Context, nsID string) error {
    currentVersion := s.getNamespaceSchemaVersion(ctx, nsID)
    
    for _, migration := range s.namespaceMigrations {
        if migration.Version > currentVersion {
            if err := s.applyNamespaceMigration(ctx, nsID, migration); err != nil {
                return err
            }
            log.Info().
                Str("namespace", nsID).
                Int("version", migration.Version).
                Msg("Applied namespace migration")
        }
    }
    return nil
}
```

### 5. Handling Version 0 (Legacy Namespaces)

Namespaces created before versioning don't have `_schema_version` table.

**Bootstrap strategy:**
1. Migration 001 creates `_schema_version` table
2. Migration 001 is idempotent (CREATE IF NOT EXISTS, CREATE OR REPLACE)
3. After running 001, version is set to 1
4. Subsequent migrations run normally

```sql
-- 001_create_schema.sql additions:

-- Create version tracking table
CREATE TABLE IF NOT EXISTS "{{SCHEMA_NAME}}"._schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT NOW()
);

-- Record initial version (at end of file)
INSERT INTO "{{SCHEMA_NAME}}"._schema_version (version) 
VALUES (1) ON CONFLICT DO NOTHING;
```

### 6. Startup Sequence

```
1. Parse config, connect to database
2. Run metadata migrations (existing behavior)
3. NEW: Run namespace migrations for all existing namespaces
4. Create default namespace (if not exists)
5. Start HTTP server
```

---

## Implementation

### Files to Modify

```
golang/
├── internal/
│   ├── migrate/
│   │   └── namespace.go           # NEW: Namespace migration logic
│   └── store/
│       ├── store.go               # Add MigrateNamespace to interface
│       ├── sqlite/
│       │   └── namespace.go       # Implement namespace migration
│       ├── postgres/
│       │   └── namespace.go       # Implement namespace migration
│       └── pebble/
│           └── namespace.go       # Implement namespace migration
├── migrations/namespace/
│   ├── postgres/
│   │   ├── 001_create_schema.sql  # Add _schema_version table
│   │   └── 002_category_get_all.sql # NEW: Fix empty category
│   └── sqlite/
│       ├── 001_create_schema.sql  # Add _schema_version table
│       └── 002_category_get_all.sql # NEW: Fix empty category
└── cmd/eventodb/
    └── main.go                    # Call MigrateAllNamespaces on startup
```

### Store Interface Addition

```go
type Store interface {
    // ... existing methods ...
    
    // MigrateNamespace applies pending schema migrations to a namespace
    MigrateNamespace(ctx context.Context, namespaceID string) error
    
    // GetNamespaceSchemaVersion returns current schema version for a namespace
    GetNamespaceSchemaVersion(ctx context.Context, namespaceID string) (int, error)
}
```

### Pebble Consideration

Pebble doesn't use SQL stored procedures - all logic is in Go. For Pebble:
- `_schema_version` still tracked (as a key-value entry)
- Migrations may update key formats or add indexes
- Most SQL-focused migrations are no-ops for Pebble

```go
// Pebble version key: _schema_version -> version number (int64)
func (s *PebbleStore) GetNamespaceSchemaVersion(ctx context.Context, nsID string) (int, error) {
    key := []byte("_schema_version")
    val, closer, err := s.db.Get(key)
    if err == pebble.ErrNotFound {
        return 0, nil
    }
    if err != nil {
        return 0, err
    }
    defer closer.Close()
    return decodeInt(val), nil
}
```

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Migration fails | Log error, abort startup |
| Namespace unreachable | Skip, log warning, continue with others |
| Version table missing | Treat as version 0, apply from 001 |
| Duplicate version insert | Ignore (ON CONFLICT DO NOTHING) |

---

## Testing

1. **Unit tests:** Migration parsing, version comparison
2. **Integration tests:** 
   - Fresh namespace gets all migrations
   - Existing namespace gets only new migrations
   - Idempotency: Running migrations twice is safe
3. **Backend-specific tests:**
   - Postgres stored procedure updates
   - SQLite schema updates
   - Pebble key format updates

---

## Migration Guidelines

1. **Always idempotent:** Use `CREATE OR REPLACE`, `IF NOT EXISTS`, `ON CONFLICT DO NOTHING`
2. **Never delete columns/tables:** Only add, never remove (backward compatibility)
3. **Template variables:** Use `{{SCHEMA_NAME}}` for Postgres namespace schema
4. **Version at end:** Insert version record as last statement
5. **Test both dialects:** Ensure migration works for Postgres and SQLite

---

## Rollback Strategy

**No automatic rollback.** Event sourcing philosophy: move forward only.

If a migration causes issues:
1. Create a new migration that reverts the change
2. Apply the fix-forward migration

---

## Future Considerations

1. **Migration status endpoint:** `GET /admin/migrations` showing per-namespace versions
2. **Dry-run mode:** Show what would be migrated without applying
3. **Parallel migration:** Migrate multiple namespaces concurrently
4. **Migration notifications:** Webhook/log on migration completion

---

## References

- [ADR-002: Schema Migrations](./ADR002-schema-migrations.md) - Metadata migrations
- [ADR-004: Namespaces and Authentication](./ADR004-namespaces-and-auth.md) - Namespace design
- [ADR-009: Sparse Export/Import](./ADR009-sparse-export-import.md) - Triggered this need
