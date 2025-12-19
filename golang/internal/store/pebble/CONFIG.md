# Pebble Store Configuration

## Overview

The Pebble store supports different configuration modes optimized for different use cases:

1. **Production Mode** (default)
2. **Test Mode** 
3. **In-Memory Mode**

## Configuration Options

```go
type Config struct {
    TestMode bool // Use reduced memory settings optimized for tests
    InMemory bool // Use in-memory storage (faster, no disk persistence)
}
```

## Usage

### Production Mode (Default)

```go
store, err := pebble.New("/path/to/data")
// Or explicitly:
store, err := pebble.NewWithConfig("/path/to/data", &pebble.Config{
    TestMode: false,
    InMemory: false,
})
```

**Settings:**
- Metadata DB: 256MB cache, 128MB memtable
- Namespace DB: 1GB cache, 256MB memtable
- WAL enabled for durability
- Optimized for high throughput and durability

### Test Mode

```go
store, err := pebble.NewWithConfig("/path/to/data", &pebble.Config{
    TestMode: true,
    InMemory: false,
})
```

**Settings:**
- Metadata DB: 32MB cache, 16MB memtable
- Namespace DB: 64MB cache, 32MB memtable
- WAL disabled for speed
- NoSync writes for namespace operations
- Optimized for test performance with reduced memory footprint

### In-Memory Mode

```go
store, err := pebble.NewWithConfig("/path/to/data", &pebble.Config{
    TestMode: true,  // Recommended with InMemory
    InMemory: true,
})
```

**Settings:**
- Metadata DB: 8MB cache, 4MB memtable
- Namespace DB: 16MB cache, 8MB memtable
- WAL disabled
- No disk persistence (uses vfs.NewMem())
- NoSync writes
- Fastest mode for testing (comparable to SQLite in-memory)

## Performance Comparison

Test suite execution times:

| Backend   | Time     | Notes                          |
|-----------|----------|--------------------------------|
| SQLite    | 10.654s  | In-memory mode                 |
| Pebble    | 10.596s  | InMemory=true, TestMode=true   |
| Postgres  | 14.775s  | Real database                  |

### Before Optimization

| Backend   | Time     | Notes                          |
|-----------|----------|--------------------------------|
| SQLite    | 10.659s  | In-memory mode                 |
| Pebble    | 20.790s  | Production settings            |
| Postgres  | 15.240s  | Real database                  |

**Improvement: ~50% faster tests** with in-memory mode!

## When to Use Each Mode

### Production Mode
- Production deployments
- When you need durability guarantees
- When you have sufficient memory resources

### Test Mode
- Integration tests that need disk persistence
- Development environments
- CI/CD pipelines with disk-based tests

### In-Memory Mode
- Unit tests
- Fast integration tests
- Temporary/ephemeral workloads
- Development with fast iteration cycles
- CI/CD pipelines (fastest option)

## Memory Usage

| Mode       | Metadata DB | Namespace DB | Total (1 NS) |
|------------|-------------|--------------|--------------|
| Production | 256MB       | 1GB          | ~1.25GB      |
| Test       | 32MB        | 64MB         | ~96MB        |
| In-Memory  | 8MB         | 16MB         | ~24MB        |

## Implementation Details

### Sync Operations

- **Production**: Uses `pebble.Sync` for namespace metadata operations (durability)
- **Test/InMemory**: Uses `pebble.NoSync` for all operations (performance)

### Write Performance

- **Production**: WAL enabled, periodic sync to disk
- **Test**: WAL disabled, faster writes
- **InMemory**: No disk I/O, fastest writes

### Directory Creation

- **Production/Test**: Creates physical directories for namespaces
- **InMemory**: Skips directory operations (uses in-memory filesystem)
