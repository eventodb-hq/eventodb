# Epic Specification Updates Summary

**Date:** December 17, 2024  
**Purpose:** Added missing Message DB compatibility features to ensure full feature parity

## Overview

After analyzing the Message DB PostgreSQL codebase, I identified several critical features missing from the original specifications. This document summarizes all updates made to MDB001, MDB002, and MDB003.

---

## MDB001: Core Storage & Migrations - UPDATES

### ✅ Added: Utility Functions (FR-2)

**New Functions:**
- `Category(streamName)` - Extract category from stream name
- `ID(streamName)` - Extract ID portion from stream name  
- `CardinalID(streamName)` - Extract cardinal ID (handles compound IDs)
- `IsCategory(name)` - Check if name is a category (no ID part)
- `Hash64(value)` - 64-bit hash compatible with Message DB

**Implementation Details:**
```go
// Compound ID support
CardinalID("account-123+456") → "123"  // Extracts part before '+'
```

**Why Critical:** Consumer groups use `cardinal_id` for partitioning. Streams with same cardinal ID route to the same consumer, even if they have different compound parts.

---

### ✅ Added: Hash Function (Message DB Compatible)

**Algorithm:**
```go
func Hash64(value string) int64 {
    hash := md5.Sum([]byte(value))
    // Take first 8 bytes, convert to int64
    return int64(binary.BigEndian.Uint64(hash[:8]))
}
```

**Why Critical:** Consumer group assignments MUST produce identical results to Message DB for data compatibility and migration scenarios.

---

### ✅ Added: Advisory Locks (Postgres)

**Implementation:**
- **Postgres:** Uses `pg_advisory_xact_lock(hash_64(category))` at category level
- **SQLite:** Uses transaction-level locking (simpler model)

**Why Critical:** Prevents race conditions during concurrent writes to the same category.

---

### ✅ Added: Compound ID Support

**Format:** `category-cardinalId+compoundPart`

**Examples:**
- `account-123` - Simple ID
- `account-123+alice` - Compound ID (cardinal: "123", compound: "alice")
- `account-123+bob` - Compound ID (cardinal: "123", compound: "bob")

**Consumer Group Behavior:**
- `account-123+alice` and `account-123+bob` route to SAME consumer
- Both share cardinal ID "123"

---

### ✅ Updated: Message Types

Added documentation for standard metadata fields:

```go
type StandardMetadata struct {
    CorrelationStreamName string `json:"correlationStreamName,omitempty"`
}
```

---

### ✅ Updated: GetOpts and CategoryOpts

Added detailed comments about defaults and deprecated fields:

```go
type GetOpts struct {
    Position       int64   // Stream position (default: 0)
    BatchSize      int64   // Default: 1000, max: 10000, -1: unlimited
    Condition      *string // DEPRECATED: SQL injection risk, do not implement
}

type CategoryOpts struct {
    Position       int64   // Global position (default: 1)
    BatchSize      int64   // Default: 1000
    Correlation    *string // Filter by metadata.correlationStreamName category
    ConsumerMember *int64  // 0-indexed
    ConsumerSize   *int64
    Condition      *string // DEPRECATED: do not implement
}
```

---

### ✅ Added: Consumer Group Implementation (FR-6)

Detailed algorithm documentation:

```
For each message in category:
  1. Extract cardinal_id from stream_name
  2. hash = hash_64(cardinal_id)
  3. member = ABS(hash) MOD consumer_group_size
  4. Include if member == consumer_group_member
```

---

### ✅ Updated: Implementation Strategy

**New Phase 1:** Utility Functions & Hashing (2 days)
- Implement all utility functions
- Test against Message DB reference data
- Verify consumer group assignments match

**Updated Phases:**
- Phase 2: Migration System (includes utility functions in namespace migrations)
- Phase 4: Postgres Backend (includes advisory locks)
- Phase 5: SQLite Backend (includes consumer group filtering in Go)

---

### ✅ Added: New Acceptance Criteria

- **AC-11:** Utility Functions Work
- **AC-12:** Hash Function Compatible with Message DB
- **AC-13:** Compound IDs with Consumer Groups
- **AC-14:** Advisory Locks Prevent Conflicts

---

### ✅ Updated: Definition of Done

Added items:
- [ ] Utility functions implemented
- [ ] Hash64 produces identical results to Message DB
- [ ] Consumer groups use cardinal_id
- [ ] Compound ID support tested
- [ ] Compatibility with Message DB verified

---

### ✅ Updated: Non-Goals

Explicitly excluded:
- ❌ SQL condition parameter (security risk)
- ❌ Debug mode / NOTICE logging
- ❌ Reporting views (can add later)

---

## MDB002: RPC API & Authentication - UPDATES

### ✅ Added: Utility Operations (FR-4)

**New RPC Methods:**

```json
// Extract category
["util.category", "account-123"] → "account"

// Extract ID
["util.id", "account-123+456"] → "123+456"

// Extract cardinal ID
["util.cardinalId", "account-123+456"] → "123"

// Check if category
["util.isCategory", "account"] → true

// Hash value (for debugging/testing)
["util.hash64", "account-123"] → -1234567890123456789
```

---

### ✅ Updated: stream.get Documentation

Added defaults and time format:

```json
{
  "position": 0,        // default: 0
  "batchSize": 1000,    // default: 1000, max: 10000
  "condition": null     // DEPRECATED: do not implement
}

// Time format: ISO 8601 with Z suffix (UTC)
// Example: "2024-12-17T01:00:00.123Z"
```

---

### ✅ Updated: category.get Documentation

**Added Correlation Filtering:**

```json
// Standard metadata field
{
  "metadata": {
    "correlationStreamName": "workflow-456"
  }
}

// Filter by correlation category
["category.get", "account", {
  "correlation": "workflow"  // Matches "workflow-*"
}]
```

**Added Consumer Group Examples:**

```json
// Compound ID behavior documented
// account-123, account-123+alice, account-123+bob
// All route to SAME consumer (cardinal_id = "123")
```

**Added Defaults:**

```json
{
  "position": 1,         // default: 1 (global position)
  "batchSize": 1000,     // default: 1000
  "correlation": null,
  "consumerGroup": null,
  "condition": null      // DEPRECATED: do not implement
}
```

---

### ✅ Renumbered FR Sections

- FR-1: Stream Operations
- FR-2: Category Operations
- FR-3: Namespace Management
- **FR-4: Utility Operations** ← NEW
- FR-5: System Operations
- FR-6: SSE Subscriptions
- FR-7: Test Mode Support
- FR-8: Error Response Format

---

### ✅ Added: Implementation Phase 4.5

**New Phase:** Utility Operations (1-2 days)
- Implement all util.* RPC methods
- Integration with store utility functions

---

### ✅ Added: New Acceptance Criteria

- **AC-8:** Utility Functions Exposed
- **AC-9:** Correlation Filtering Works
- **AC-10:** Time Format Consistent

---

### ✅ Updated: Validation Rules

Added:
1. Stream name format documentation (compound IDs)
2. Batch size: 1-10000 (default: 1000, -1: unlimited)
3. Correlation must be category name (no `-`)
4. Position/global position validation

---

### ✅ Updated: Definition of Done

Added items:
- [ ] Consumer groups use cardinal_id for compound ID support
- [ ] Correlation filtering using metadata.correlationStreamName
- [ ] All utility operations implemented
- [ ] Time format standardized (ISO 8601 UTC)
- [ ] Default batch size documented (1000)
- [ ] Compound ID scenarios tested

---

## MDB003: Testing & Production Readiness - UPDATES

### ✅ Added: Utility Function Tests

**New Test Suite:**

```typescript
// test/tests/util.test.ts
test('category extraction', ...)
test('id extraction', ...)
test('cardinal id extraction', ...)
test('category check', ...)
test('hash64 consistency', ...)
```

Tests all utility functions with various stream name formats.

---

### ✅ Added: Compound ID Tests

**New Tests in category.test.ts:**

```typescript
test('compound IDs route to same consumer', async () => {
  // Verify account-123+alice and account-123+bob
  // route to same consumer (cardinal_id = 123)
});
```

---

### ✅ Added: Correlation Filtering Tests

**New Test:**

```typescript
test('correlation filtering works', async () => {
  // Write with metadata.correlationStreamName
  // Query with correlation filter
  // Verify correct filtering
});
```

---

### ✅ Added: Message DB Compatibility Tests

**New Test Suite:**

```typescript
// test/tests/compatibility.test.ts

test('hash64 matches Message DB', ...)
  // Test against reference hashes from Message DB

test('consumer group assignment matches Message DB', ...)
  // Verify identical consumer assignments

test('time format matches ISO 8601', ...)
  // Verify time field format
```

**Purpose:** Ensure compatibility with Message DB for migration scenarios.

---

### ✅ Added: New Acceptance Criteria

- **AC-7:** Message DB Compatibility Verified (hash & consumer groups)
- **AC-8:** Utility Functions Work
- **AC-9:** Compound IDs Tested
- **AC-10:** Time Format Standardized

---

### ✅ Updated: Definition of Done

Added items:
- [ ] Consumer group tests with compound IDs passing
- [ ] Correlation filtering tests passing
- [ ] All utility function tests passing
- [ ] Message DB compatibility tests passing
- [ ] Hash function compatibility verified
- [ ] Consumer group assignment compatibility verified
- [ ] Time format standardization verified
- [ ] API documentation includes utility functions
- [ ] Migration guide from Message DB complete
- [ ] Usage examples include compound IDs

---

## Summary of Key Changes

### Critical Features Added

1. **Utility Functions** - Stream name parsing (category, id, cardinal_id, is_category, hash64)
2. **Compound ID Support** - Format: `category-cardinalId+compoundPart`
3. **Hash Function** - MD5-based, compatible with Message DB
4. **Advisory Locks** - Category-level locking in Postgres
5. **Correlation Metadata** - Standard `metadata.correlationStreamName` field
6. **Default Values** - Documented defaults for batch sizes, positions

### Documentation Improvements

1. **Time Format** - ISO 8601 UTC with Z suffix
2. **Consumer Group Behavior** - Detailed algorithm with compound ID handling
3. **Validation Rules** - Stream name formats, batch size limits
4. **Error Handling** - SQL condition parameter explicitly deprecated

### Testing Enhancements

1. **Utility Function Tests** - Complete test coverage
2. **Compound ID Tests** - Consumer group partitioning
3. **Compatibility Tests** - Verification against Message DB reference data
4. **Correlation Tests** - Metadata filtering

---

## Migration Impact

These updates ensure:

✅ **Full Message DB Compatibility** - Can migrate data from Message DB  
✅ **Identical Consumer Groups** - Same stream assignments  
✅ **Hash Compatibility** - Same hash values for same inputs  
✅ **API Parity** - All Message DB utility functions available  
✅ **Compound ID Support** - Advanced stream partitioning scenarios  

---

## Next Steps

1. **Review Updates** - Verify all changes align with project goals
2. **Implementation** - Follow updated phase plans in each epic
3. **Testing** - Add Message DB reference data for compatibility tests
4. **Documentation** - Create migration guide from Message DB to this system

---

## Files Modified

- `@meta/@pm/epics/MDB001_spec.md` - Core Storage & Migrations
- `@meta/@pm/epics/MDB002_spec.md` - RPC API & Authentication
- `@meta/@pm/epics/MDB003_spec.md` - Testing & Production Readiness

All changes maintain backward compatibility and add new functionality without removing existing features.
