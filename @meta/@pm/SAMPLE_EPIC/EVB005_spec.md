# EPIC EVB005: Event Schema Validation

## Overview

**Epic ID:** EVB005
**Name:** Event Schema Validation
**Duration:** 3-4 days
**Status:** pending
**Priority:** high
**Depends On:** EVB001 (Core Event Storage)

**Goal:** Enable schema registration and validation for event types to ensure data quality and enable future storage optimizations.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│ Protocol Layer (eventobase-server)                 │
│ - w.register_schema                                 │
│ - w.delete_schema                                   │
│ - q.list_schemas / q.get_schema                     │
└─────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│ Domain API (eventobase-core)                        │
│ - Schema validation on append                       │
│ - LZ4 compression for all payloads                  │
└─────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│ SQLite Database (event_types table)                │
│ - schema_definition BLOB (compressed)               │
│ - accept_any INTEGER (0/1)                          │
│ - compression_algo TEXT                             │
└─────────────────────────────────────────────────────┘
```

## Technical Requirements

### Database Schema Extension

```sql
ALTER TABLE event_types ADD COLUMN schema_definition BLOB;
ALTER TABLE event_types ADD COLUMN schema_version INTEGER DEFAULT 1;
ALTER TABLE event_types ADD COLUMN accept_any INTEGER DEFAULT 0;
ALTER TABLE event_types ADD COLUMN compression_algo TEXT DEFAULT 'lz4';
ALTER TABLE event_types ADD COLUMN created_at INTEGER NOT NULL;
```

### Schema Format (JSON)

```json
{
  "version": 1,
  "fields": [
    {"name": "user_id", "type": "string", "required": true},
    {"name": "email", "type": "string", "required": false},
    {"name": "metadata", "type": "any", "required": false}
  ]
}
```

**Supported Types:** `string`, `int64`, `float64`, `boolean`, `timestamp`, `any`

## Functional Requirements

### FR-1: Schema Types and Validation

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EventSchema {
    pub version: i32,
    pub fields: Vec<SchemaField>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SchemaField {
    pub name: String,
    pub field_type: FieldType,
    pub required: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum FieldType {
    String,
    Int64,
    Float64,
    Boolean,
    Timestamp,
    Any,
}

impl EventSchema {
    pub fn validate(&self) -> Result<()> {
        if self.version != 1 {
            return Err(Error::InvalidSchemaVersion(self.version));
        }
        if self.fields.is_empty() {
            return Err(Error::InvalidSchema("fields cannot be empty".into()));
        }
        for field in &self.fields {
            if field.name.is_empty() {
                return Err(Error::InvalidSchema("field name cannot be empty".into()));
            }
        }
        Ok(())
    }
}
```

### FR-2: Schema Registration

```rust
impl Domain {
    pub fn register_schema(
        &mut self,
        event_type: &str,
        schema: Option<EventSchema>,
        accept_any: bool,
        compression_algo: &str,
    ) -> Result<()> {
        // Validate inputs
        if !accept_any && schema.is_none() {
            return Err(Error::InvalidSchema("schema required when accept_any is false".into()));
        }

        if let Some(s) = &schema {
            s.validate()?;
        }

        let tx = self.conn.transaction()?;

        // Get or create event_type
        let vocab = VocabularyManager::new(&tx);
        let event_type_id = vocab.ensure_event_type(event_type)?;

        // Compress schema if provided
        let schema_blob = if let Some(s) = schema {
            let json = serde_json::to_vec(&s)?;
            Some(compress_lz4(&json))
        } else {
            None
        };

        let now = current_timestamp_ms();

        // Insert or update schema
        tx.execute(
            "INSERT INTO event_types (id, name, schema_definition, schema_version, accept_any, compression_algo, created_at)
             VALUES (?, ?, ?, 1, ?, ?, ?)
             ON CONFLICT(id) DO UPDATE SET
                schema_definition = excluded.schema_definition,
                accept_any = excluded.accept_any,
                compression_algo = excluded.compression_algo",
            rusqlite::params![event_type_id, event_type, schema_blob, accept_any as i32, compression_algo, now]
        )?;

        tx.commit()?;
        Ok(())
    }
}
```

### FR-3: Schema Deletion (Safe)

```rust
impl Domain {
    pub fn delete_schema(&mut self, event_type: &str) -> Result<()> {
        let tx = self.conn.transaction()?;

        let event_type_id: i64 = tx.query_row(
            "SELECT id FROM event_types WHERE name = ?",
            [event_type],
            |row| row.get(0)
        ).map_err(|_| Error::SchemaNotFound(event_type.to_string()))?;

        // Fast EXISTS check
        let has_events: bool = tx.query_row(
            "SELECT EXISTS(SELECT 1 FROM events WHERE event_type_id = ? LIMIT 1)",
            [event_type_id],
            |row| row.get(0)
        )?;

        if has_events {
            return Err(Error::SchemaInUse(event_type.to_string()));
        }

        tx.execute("DELETE FROM event_types WHERE id = ?", [event_type_id])?;
        tx.commit()?;
        Ok(())
    }
}
```

### FR-4: Validation on Append

```rust
impl Domain {
    fn validate_event_data(
        &self,
        event_type_id: i64,
        data: &serde_json::Value,
    ) -> Result<()> {
        // Get schema config
        let (schema_def, accept_any): (Option<Vec<u8>>, i32) = self.conn.query_row(
            "SELECT schema_definition, accept_any FROM event_types WHERE id = ?",
            [event_type_id],
            |row| Ok((row.get(0)?, row.get(1)?))
        ).optional()?.unwrap_or((None, 0));

        // Skip validation if accept_any
        if accept_any == 1 {
            return Ok(());
        }

        // No schema registered - allow
        let Some(schema_blob) = schema_def else {
            return Ok(());
        };

        // Decompress and parse schema
        let schema_json = decompress_lz4(&schema_blob)?;
        let schema: EventSchema = serde_json::from_slice(&schema_json)?;

        // Validate data against schema
        let obj = data.as_object()
            .ok_or(Error::ValidationFailed("data must be an object".into()))?;

        for field in &schema.fields {
            if !field.required {
                continue;
            }

            let value = obj.get(&field.name)
                .ok_or(Error::ValidationFailed(format!("required field '{}' missing", field.name)))?;

            if field.field_type != FieldType::Any {
                validate_field_type(&field.field_type, value)
                    .map_err(|e| Error::ValidationFailed(format!("field '{}': {}", field.name, e)))?;
            }
        }

        Ok(())
    }
}

fn validate_field_type(field_type: &FieldType, value: &serde_json::Value) -> Result<()> {
    match field_type {
        FieldType::String => {
            if !value.is_string() {
                return Err(Error::ValidationFailed(format!("expected string, got {}", value_type_name(value))));
            }
        }
        FieldType::Int64 => {
            if !value.is_i64() && !value.is_u64() {
                return Err(Error::ValidationFailed(format!("expected int64, got {}", value_type_name(value))));
            }
        }
        FieldType::Float64 => {
            if !value.is_f64() && !value.is_i64() && !value.is_u64() {
                return Err(Error::ValidationFailed(format!("expected float64, got {}", value_type_name(value))));
            }
        }
        FieldType::Boolean => {
            if !value.is_boolean() {
                return Err(Error::ValidationFailed(format!("expected boolean, got {}", value_type_name(value))));
            }
        }
        FieldType::Timestamp => {
            if !value.is_i64() && !value.is_u64() {
                return Err(Error::ValidationFailed(format!("expected timestamp (int64), got {}", value_type_name(value))));
            }
        }
        FieldType::Any => {}
    }
    Ok(())
}
```

### FR-5: LZ4 Compression

```rust
use lz4_flex::{compress_prepend_size, decompress_size_prepended};

pub fn compress_lz4(data: &[u8]) -> Vec<u8> {
    compress_prepend_size(data)
}

pub fn decompress_lz4(compressed: &[u8]) -> Result<Vec<u8>> {
    decompress_size_prepended(compressed)
        .map_err(|e| Error::DecompressionFailed(e.to_string()))
}

// Apply to event data during append
impl Domain {
    fn compress_event_data(&self, data: &serde_json::Value) -> Result<Vec<u8>> {
        let json = serde_json::to_vec(data)?;
        Ok(compress_lz4(&json))
    }
}
```

### FR-6: Schema Copying for Sandboxes

```rust
impl Domain {
    pub fn copy_schemas_from(&mut self, source_conn: &Connection) -> Result<usize> {
        let tx = self.conn.transaction()?;

        let mut stmt = source_conn.prepare(
            "SELECT name, schema_definition, schema_version, accept_any, compression_algo, created_at
             FROM event_types
             WHERE schema_definition IS NOT NULL OR accept_any = 1"
        )?;

        let schemas: Vec<_> = stmt.query_map([], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, Option<Vec<u8>>>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, String>(4)?,
                row.get::<_, i64>(5)?
            ))
        })?.collect::<Result<Vec<_>, _>>()?;

        let mut count = 0;
        for (name, def, ver, any, algo, created) in schemas {
            tx.execute(
                "INSERT OR IGNORE INTO event_types (name, schema_definition, schema_version, accept_any, compression_algo, created_at)
                 VALUES (?, ?, ?, ?, ?, ?)",
                rusqlite::params![name, def, ver, any, algo, created]
            )?;
            count += 1;
        }

        tx.commit()?;
        Ok(count)
    }
}
```

### FR-7: Schema Query Operations

```rust
impl Domain {
    pub fn list_schemas(&self) -> Result<Vec<SchemaInfo>> {
        let mut stmt = self.conn.prepare(
            "SELECT name, schema_definition, accept_any, compression_algo, created_at
             FROM event_types
             WHERE schema_definition IS NOT NULL OR accept_any = 1"
        )?;

        let schemas = stmt.query_map([], |row| {
            let name: String = row.get(0)?;
            let schema_blob: Option<Vec<u8>> = row.get(1)?;
            let accept_any: i32 = row.get(2)?;
            let compression_algo: String = row.get(3)?;
            let created_at: i64 = row.get(4)?;

            let schema = if let Some(blob) = schema_blob {
                let json = decompress_lz4(&blob)?;
                Some(serde_json::from_slice::<EventSchema>(&json)?)
            } else {
                None
            };

            Ok(SchemaInfo {
                event_type: name,
                schema,
                accept_any: accept_any == 1,
                compression_algo,
                created_at,
            })
        })?.collect::<Result<Vec<_>, _>>()?;

        Ok(schemas)
    }

    pub fn get_schema(&self, event_type: &str) -> Result<SchemaInfo> {
        // Similar to list_schemas but with WHERE name = ?
    }
}

pub struct SchemaInfo {
    pub event_type: String,
    pub schema: Option<EventSchema>,
    pub accept_any: bool,
    pub compression_algo: String,
    pub created_at: i64,
}
```

## Implementation Strategy

### Phase 1: Core Types & Validation (0.5 day)
- Add EventSchema, SchemaField, FieldType types
- Implement schema validation logic
- Add validate_field_type function
- Unit tests for validation

### Phase 2: Database Schema & Compression (0.5 day)
- Migration to extend event_types table
- Implement LZ4 compression helpers
- Test compression/decompression

### Phase 3: Schema Registration & Deletion (1 day)
- Implement register_schema
- Implement delete_schema with EXISTS check
- Test registration/deletion flows
- Test safe deletion enforcement

### Phase 4: Validation on Append (1 day)
- Integrate validation into append flow
- Compress event data during append
- Add error handling for validation failures
- Test validation with various schemas

### Phase 5: Protocol Operations (0.5 day)
- Implement protocol handlers (w.register_schema, w.delete_schema)
- Implement query handlers (q.list_schemas, q.get_schema)
- Wire up to JSONRPC layer

### Phase 6: Sandbox Schema Copying (0.5 day)
- Implement copy_schemas_from
- Integrate with sandbox creation
- Test schema copying

### Phase 7: Testing & Documentation (0.5 day)
- Integration tests
- Performance validation
- Update API documentation

## Acceptance Criteria

### AC-1: Schema Registration
- **GIVEN** Valid schema definition
- **WHEN** register_schema is called
- **THEN** Schema is stored compressed in event_types table

### AC-2: Schema Validation on Append
- **GIVEN** Event type with registered schema
- **WHEN** Appending event with missing required field
- **THEN** Returns validation error, event not stored

### AC-3: Accept Any Mode
- **GIVEN** Event type with accept_any=true
- **WHEN** Appending event with any data structure
- **THEN** Event is stored without validation

### AC-4: Safe Schema Deletion
- **GIVEN** Schema with existing events
- **WHEN** delete_schema is called
- **THEN** Returns SCHEMA_IN_USE error, schema not deleted

### AC-5: Permissive Validation
- **GIVEN** Event with extra fields not in schema
- **WHEN** Appending event
- **THEN** Event is accepted and stored

### AC-6: Compression Applied
- **GIVEN** Any event append
- **WHEN** Event is stored
- **THEN** Data is LZ4 compressed in events.data BLOB

### AC-7: Schema Copying
- **GIVEN** Sandbox creation with copy_schemas_from
- **WHEN** Sandbox is created
- **THEN** All schemas copied, events not copied

## Definition of Done

- [ ] EventSchema types and validation implemented
- [ ] Database migration for event_types columns
- [ ] LZ4 compression/decompression working
- [ ] register_schema implemented
- [ ] delete_schema with EXISTS check implemented
- [ ] Validation integrated into append flow
- [ ] Permissive mode (extra fields accepted)
- [ ] Accept-any mode working
- [ ] Protocol operations (w.register_schema, w.delete_schema, q.list_schemas, q.get_schema)
- [ ] Schema copying for sandboxes
- [ ] Comprehensive test suite (>80% coverage)
- [ ] Error handling for all edge cases
- [ ] Documentation updated (API_PROTOCOL.md, ADR)
- [ ] Code passes clippy and rustfmt

## Error Codes

- `SCHEMA_VALIDATION_FAILED` - Event data doesn't match schema
- `SCHEMA_IN_USE` - Cannot delete schema, events exist
- `SCHEMA_NOT_FOUND` - Schema doesn't exist
- `INVALID_SCHEMA_DEFINITION` - Malformed schema JSON

## Performance Expectations

| Operation | Expected Performance |
|-----------|---------------------|
| Schema validation | <5ms per event |
| LZ4 compression | <2ms per event |
| LZ4 decompression | <1ms per event |
| Schema registration | <20ms |
| Schema deletion check | <10ms (EXISTS query) |

## Validation Rules

1. **Required fields** must be present in event data
2. **Type checking** enforced for declared types (except `any`)
3. **Permissive mode** - extra fields accepted
4. **Any type** - accepts any value without validation
5. **Validation timing** - only at ingestion (w.append)

## Non-Goals

- ❌ Schema versioning/migration
- ❌ Nested object validation (use `any` type)
- ❌ Custom validation rules (regex, ranges, etc.)
- ❌ Schema evolution/compatibility checks
- ❌ Validation on read
- ❌ Optional compression
