# Message Store

The `store` package provides a unified interface for message storage with support for multiple backend implementations (Postgres and SQLite).

## Overview

The message store implements a Message DB-compatible storage layer with the following key features:

- **Dual Backend Support**: Postgres and SQLite backends with identical APIs
- **Namespace Isolation**: Physical separation of data per namespace (schemas for Postgres, separate DBs for SQLite)
- **Optimistic Locking**: Version-based concurrency control for writes
- **Category Queries**: Efficient queries across multiple streams in a category
- **Consumer Groups**: Deterministic partitioning for parallel processing
- **Message DB Compatibility**: Hash functions and utility functions compatible with Message DB

## Architecture

```
┌─────────────────────────────────────────────────────┐
│ Store Interface (store.go)                          │
│ - WriteMessage, GetStreamMessages                   │
│ - GetCategoryMessages, CreateNamespace              │
│ - Utility functions (Category, ID, Hash64, etc.)    │
└─────────────────────────────────────────────────────┘
                        │
         ┌──────────────┴──────────────┐
         ▼                             ▼
┌────────────────────┐        ┌────────────────────┐
│ Postgres Backend   │        │ SQLite Backend     │
│                    │        │                    │
│ - Schema per NS    │        │ - DB file per NS   │
│ - Stored procs     │        │ - Go logic         │
│ - Advisory locks   │        │ - In-memory mode   │
└────────────────────┘        └────────────────────┘
```

## Quick Start

### Using Postgres Backend

```go
package main

import (
    "context"
    "database/sql"
    "log"
    
    _ "github.com/jackc/pgx/v5/stdlib"
    "github.com/message-db/message-db/internal/store"
    "github.com/message-db/message-db/internal/store/postgres"
)

func main() {
    // Connect to Postgres
    db, err := sql.Open("pgx", "postgres://postgres:postgres@localhost:5432/postgres")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    // Create store
    st, err := postgres.New(db)
    if err != nil {
        log.Fatal(err)
    }
    defer st.Close()
    
    ctx := context.Background()
    
    // Create a namespace
    err = st.CreateNamespace(ctx, "myapp", "secure_token_hash", "My Application")
    if err != nil {
        log.Fatal(err)
    }
    
    // Write a message
    msg := &store.Message{
        StreamName: "account-123",
        Type:       "AccountCreated",
        Data: map[string]interface{}{
            "accountId": "123",
            "name":      "John Doe",
        },
    }
    
    result, err := st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Wrote message at position %d, global position %d", 
        result.Position, result.GlobalPosition)
    
    // Read messages from stream
    opts := &store.GetOpts{
        Position:  0,
        BatchSize: 10,
    }
    
    messages, err := st.GetStreamMessages(ctx, "myapp", "account-123", opts)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, m := range messages {
        log.Printf("Message: %s at position %d", m.Type, m.Position)
    }
}
```

### Using SQLite Backend

```go
package main

import (
    "context"
    "database/sql"
    "log"
    
    _ "modernc.org/sqlite"
    "github.com/message-db/message-db/internal/store"
    "github.com/message-db/message-db/internal/store/sqlite"
)

func main() {
    // In-memory mode (for testing)
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    // Create store in test mode (in-memory namespaces)
    st, err := sqlite.New(db, true, "")
    if err != nil {
        log.Fatal(err)
    }
    defer st.Close()
    
    // For production, use file-based mode:
    // st, err := sqlite.New(db, false, "/var/lib/messagedb")
    
    // Usage is identical to Postgres backend
    ctx := context.Background()
    err = st.CreateNamespace(ctx, "myapp", "secure_token_hash", "My Application")
    // ... rest of code is the same
}
```

## Core Concepts

### Stream Names

Stream names follow the format: `category-id` or `category-cardinalId+compoundPart`

Examples:
- `account-123` - Simple stream
- `account-123+deposit` - Compound stream (multiple streams for same entity)

The package provides utility functions to parse stream names:

```go
// Extract category name
category := st.Category("account-123")  // Returns: "account"

// Extract ID portion
id := st.ID("account-123")  // Returns: "123"
id := st.ID("account-123+deposit")  // Returns: "123+deposit"

// Extract cardinal ID (for consumer groups)
cardinalID := st.CardinalID("account-123+deposit")  // Returns: "123"

// Check if name is a category (no ID)
isCategory := st.IsCategory("account")  // Returns: true
isCategory := st.IsCategory("account-123")  // Returns: false
```

### Optimistic Locking

Use the `ExpectedVersion` field to ensure concurrent safety:

```go
// Read current version
version, err := st.GetStreamVersion(ctx, "myapp", "account-123")

// Write with expected version
msg := &store.Message{
    StreamName:      "account-123",
    Type:           "AccountUpdated",
    Data:           map[string]interface{}{"balance": 100},
    ExpectedVersion: &version,  // Will fail if version changed
}

result, err := st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
if err == store.ErrVersionConflict {
    // Handle conflict - retry or merge
}
```

### Category Queries

Read messages from all streams in a category:

```go
opts := &store.CategoryOpts{
    Position:  1,       // Global position to start from
    BatchSize: 100,     // Number of messages
}

messages, err := st.GetCategoryMessages(ctx, "myapp", "account", opts)
// Returns messages from account-123, account-456, etc.
```

### Consumer Groups

Partition category processing across multiple consumers:

```go
// Consumer 0 of 4-member group
opts := &store.CategoryOpts{
    Position:       1,
    BatchSize:      100,
    ConsumerMember: ptr(int64(0)),  // This consumer's index
    ConsumerSize:   ptr(int64(4)),  // Total consumers in group
}

messages, err := st.GetCategoryMessages(ctx, "myapp", "account", opts)
// Returns only messages assigned to consumer 0
// Assignment is deterministic based on Hash64(CardinalID(streamName))
```

Consumer assignment ensures:
1. Each stream is processed by exactly one consumer
2. All messages from the same stream go to the same consumer
3. Compound streams (e.g., `account-123+deposit`, `account-123+withdraw`) are assigned to the same consumer (based on cardinal ID `123`)

### Namespaces

Namespaces provide physical isolation:

```go
// Create namespace
err := st.CreateNamespace(ctx, "tenant-a", "hash_a", "Tenant A")

// Create another namespace
err = st.CreateNamespace(ctx, "tenant-b", "hash_b", "Tenant B")

// Write to different namespaces (completely isolated)
st.WriteMessage(ctx, "tenant-a", "account-1", msg1)
st.WriteMessage(ctx, "tenant-b", "account-1", msg2)

// List all namespaces
namespaces, err := st.ListNamespaces(ctx)

// Delete namespace (removes all data)
err = st.DeleteNamespace(ctx, "tenant-a")
```

## Backend Comparison

| Feature | Postgres | SQLite File | SQLite Memory |
|---------|----------|-------------|---------------|
| **Performance (Write)** | ~10ms | ~5ms | ~1ms |
| **Performance (Read)** | ~15ms | ~8ms | ~2ms |
| **Concurrency** | Excellent | Good | Limited |
| **Isolation** | Schema per NS | DB file per NS | In-memory |
| **Production Ready** | ✅ Yes | ✅ Yes | ❌ Testing only |
| **Locking** | Advisory locks | Transaction locks | Transaction locks |
| **Use Case** | Production | Edge/embedded | Testing |

## Testing

The package includes comprehensive tests and benchmarks:

```bash
# Run all tests
go test ./internal/store/...

# Run benchmarks
go test -bench=. ./internal/store/...

# Run specific backend tests
go test ./internal/store/postgres/...
go test ./internal/store/sqlite/...

# Run integration tests
go test ./internal/store/integration/...
```

## Performance Targets

All backends meet the following performance targets:

| Operation | Target |
|-----------|--------|
| WriteMessage | <10ms (Postgres), <5ms (SQLite file), <1ms (SQLite memory) |
| GetStreamMessages (10) | <15ms (Postgres), <8ms (SQLite file), <2ms (SQLite memory) |
| GetCategoryMessages (100) | <50ms (Postgres), <30ms (SQLite file), <10ms (SQLite memory) |
| CreateNamespace | <100ms (Postgres), <50ms (SQLite file), <20ms (SQLite memory) |
| DeleteNamespace | <200ms (Postgres), <100ms (SQLite file), <50ms (SQLite memory) |

## Error Handling

The package defines specific error types:

```go
import "github.com/message-db/message-db/internal/store"

_, err := st.WriteMessage(ctx, ns, stream, msg)
switch {
case errors.Is(err, store.ErrVersionConflict):
    // Handle optimistic locking conflict
case errors.Is(err, store.ErrNamespaceNotFound):
    // Namespace doesn't exist
case errors.Is(err, store.ErrStreamNotFound):
    // Stream doesn't exist
default:
    // Other error
}
```

## Advanced Usage

### Correlation Filtering

Filter category messages by correlation:

```go
correlation := "order-789"
opts := &store.CategoryOpts{
    Position:    1,
    BatchSize:   100,
    Correlation: &correlation,  // Filter by metadata.correlationStreamName
}

messages, err := st.GetCategoryMessages(ctx, "myapp", "payment", opts)
// Returns only payments correlated with order-789
```

### Last Message with Type Filter

Get the last message of a specific type:

```go
msgType := "AccountUpdated"
msg, err := st.GetLastStreamMessage(ctx, "myapp", "account-123", &msgType)
// Returns last AccountUpdated message, or nil if none found
```

## Message DB Compatibility

This implementation maintains compatibility with [Message DB](https://github.com/message-db/message-db):

- **Hash Function**: `Hash64()` produces identical results to Message DB's `hash_64()`
- **Utility Functions**: `Category()`, `ID()`, `CardinalID()` match Message DB behavior
- **Consumer Groups**: Use the same partitioning algorithm
- **Schema**: Postgres backend uses compatible table structure and stored procedures

This allows:
- Migration from/to Message DB
- Consistent consumer group assignments
- Shared tooling and queries

## Security Considerations

1. **Token Hashing**: Always hash tokens before storing (use `tokenHash` parameter)
2. **Namespace Isolation**: Namespaces are physically isolated - one tenant cannot access another's data
3. **SQL Injection**: All queries use parameterized statements
4. **File Permissions**: SQLite files should have appropriate permissions (0600 recommended)

## Troubleshooting

### Postgres Connection Issues

```go
// Enable connection pooling
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

### SQLite Lock Errors

```go
// For concurrent writes, use WAL mode (file-based only)
db.Exec("PRAGMA journal_mode=WAL")
```

### Performance Issues

1. Check indexes are created (handled by migrations)
2. Monitor batch sizes (default 1000, adjust as needed)
3. Use appropriate backend for your use case
4. Consider consumer group size (4-8 consumers typically optimal)

## Contributing

When adding features:

1. Update the `Store` interface in `store.go`
2. Implement in both Postgres and SQLite backends
3. Add tests to ensure parity
4. Update this README with examples
5. Add benchmarks if performance-critical

## License

See project root LICENSE file.
