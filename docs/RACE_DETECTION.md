# Race Condition Detection and Prevention Guide

> **⚠️ macOS Users with CGO Issues?** See [RACE_DETECTION_MACOS_FIX.md](./RACE_DETECTION_MACOS_FIX.md) for solutions.

## Quick Start: Running Race Detection

### 🐳 Easy Mode: Use Docker (Works on all platforms)
```bash
# From project root
./bin/race_check_docker.sh
```

### 💻 Local Mode: Native race detection
```bash
cd golang
go clean -testcache
go test ./... -race -timeout 120s
```

**Note:** If you get compiler errors on macOS, use the Docker method above.

### Run specific package with race detector:
```bash
go test ./internal/api -race -v
```

### Run specific test with race detector:
```bash
go test ./internal/api -race -run TestSpecificFunction -v
```

---

## Why CGO is Required

The Go race detector relies on C runtime instrumentation (CGO). 

**Important:** On macOS with recent Xcode versions, you may encounter CGO compiler errors. In that case:
- Use Docker: `./bin/race_check_docker.sh`
- Or see [RACE_DETECTION_MACOS_FIX.md](./RACE_DETECTION_MACOS_FIX.md) for fixes

The race detector is automatically enabled when you use the `-race` flag (CGO is enabled by default in Go).

---

## Common Race Condition Patterns in Your Codebase

### 1. **Map Access Without Locks**
```go
// ❌ WRONG - Race condition
type Store struct {
    namespaces map[string]*DB
}

func (s *Store) Get(name string) *DB {
    return s.namespaces[name]  // Concurrent read/write = race!
}

func (s *Store) Set(name string, db *DB) {
    s.namespaces[name] = db  // Concurrent write = race!
}

// ✅ CORRECT - Protected with mutex
type Store struct {
    mu         sync.RWMutex
    namespaces map[string]*DB
}

func (s *Store) Get(name string) *DB {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.namespaces[name]
}

func (s *Store) Set(name string, db *DB) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.namespaces[name] = db
}
```

### 2. **Shared Variable Access in Goroutines**
```go
// ❌ WRONG - Race on counter
func BadConcurrentCounter() int {
    counter := 0
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            counter++  // Race condition!
            wg.Done()
        }()
    }
    wg.Wait()
    return counter
}

// ✅ CORRECT - Using atomic operations
func GoodConcurrentCounter() int64 {
    var counter int64
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            atomic.AddInt64(&counter, 1)
            wg.Done()
        }()
    }
    wg.Wait()
    return counter
}

// ✅ ALSO CORRECT - Using mutex
func GoodConcurrentCounterMutex() int {
    counter := 0
    var mu sync.Mutex
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            mu.Lock()
            counter++
            mu.Unlock()
            wg.Done()
        }()
    }
    wg.Wait()
    return counter
}
```

### 3. **Channel Operations Without Proper Synchronization**
```go
// ❌ WRONG - Potential race on closed channel
type PubSub struct {
    subs map[Subscriber]struct{}
}

func (ps *PubSub) Unsubscribe(sub Subscriber) {
    delete(ps.subs, sub)
    close(sub)  // Another goroutine might be writing to this!
}

// ✅ CORRECT - Check before closing
type PubSub struct {
    mu   sync.RWMutex
    subs map[Subscriber]struct{}
}

func (ps *PubSub) Unsubscribe(sub Subscriber) {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    
    if _, ok := ps.subs[sub]; ok {
        delete(ps.subs, sub)
        close(sub)
    }
}
```

### 4. **Test Cleanup Race Conditions**
```go
// ❌ WRONG - Cleanup race
func TestSomething(t *testing.T) {
    store := NewStore()
    
    go func() {
        // Some async work
        store.Write()
    }()
    
    store.Close()  // Race: goroutine still writing!
}

// ✅ CORRECT - Wait for goroutines
func TestSomething(t *testing.T) {
    store := NewStore()
    
    var wg sync.WaitGroup
    wg.Add(1)
    go func() {
        defer wg.Done()
        store.Write()
    }()
    
    wg.Wait()
    store.Close()
}

// ✅ EVEN BETTER - Use context for cancellation
func TestSomething(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    store := NewStore()
    
    var wg sync.WaitGroup
    wg.Add(1)
    go func() {
        defer wg.Done()
        select {
        case <-ctx.Done():
            return
        default:
            store.Write()
        }
    }()
    
    wg.Wait()
    store.Close()
}
```

---

## Best Practices for Your EventoDB Backend

### 1. **Always Use Mutexes for Shared State**

Your `PubSub` and `PebbleStore` already do this correctly:

```go
// From your pubsub.go (GOOD!)
type PubSub struct {
    mu sync.RWMutex
    streamSubs   map[string]map[string]map[Subscriber]struct{}
    categorySubs map[string]map[string]map[Subscriber]struct{}
}

func (ps *PubSub) Publish(event WriteEvent) {
    ps.mu.RLock()  // Read lock for concurrent reads
    defer ps.mu.RUnlock()
    // ... safe access to maps
}
```

### 2. **Use Read/Write Locks Appropriately**

- `RLock()` for reads (multiple readers can run concurrently)
- `Lock()` for writes (exclusive access)

```go
// Reading (can be done by multiple goroutines)
s.mu.RLock()
val := s.namespaces[key]
s.mu.RUnlock()

// Writing (exclusive)
s.mu.Lock()
s.namespaces[key] = value
s.mu.Unlock()
```

### 3. **Defer Unlocks to Prevent Deadlocks**

Always use `defer` to ensure unlocks happen even if function panics:

```go
func (s *Store) Get(key string) (*Value, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()  // ✅ Guaranteed to unlock
    
    if val, ok := s.data[key]; ok {
        return val, nil
    }
    return nil, ErrNotFound  // Lock still released!
}
```

### 4. **Avoid Holding Locks During Slow Operations**

```go
// ❌ WRONG - Holding lock during I/O
func (s *Store) SlowOperation(key string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    data := s.data[key]
    return data.WriteToDatabase()  // SLOW! Blocks all other operations
}

// ✅ CORRECT - Release lock before I/O
func (s *Store) SlowOperation(key string) error {
    s.mu.RLock()
    data := s.data[key]
    s.mu.RUnlock()  // Release lock
    
    return data.WriteToDatabase()  // No lock held during I/O
}
```

### 5. **Use sync.WaitGroup for Goroutine Coordination**

```go
func ProcessItems(items []Item) {
    var wg sync.WaitGroup
    
    for _, item := range items {
        wg.Add(1)
        go func(i Item) {
            defer wg.Done()
            processItem(i)
        }(item)  // ✅ Pass item as parameter!
    }
    
    wg.Wait()  // Wait for all goroutines to finish
}
```

### 6. **Use atomic Package for Simple Counters**

```go
type Stats struct {
    requestCount int64  // ✅ Use int64 for atomic ops
}

func (s *Stats) IncrementRequests() {
    atomic.AddInt64(&s.requestCount, 1)
}

func (s *Stats) GetRequests() int64 {
    return atomic.LoadInt64(&s.requestCount)
}
```

### 7. **Double-Check Pattern (Your Code Does This!)**

```go
// From your pebble/store.go (EXCELLENT!)
func (s *PebbleStore) getNamespaceDB(ctx context.Context, nsID string) (*namespaceHandle, error) {
    // Fast path: check with read lock
    s.mu.RLock()
    if handle, ok := s.namespaces[nsID]; ok {
        s.mu.RUnlock()
        return handle, nil
    }
    s.mu.RUnlock()
    
    // Slow path: acquire write lock
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Double-check: another goroutine might have created it
    if handle, ok := s.namespaces[nsID]; ok {
        return handle, nil
    }
    
    // Create new handle
    handle := &namespaceHandle{db: db}
    s.namespaces[nsID] = handle
    return handle, nil
}
```

---

## Testing for Race Conditions

### Run Tests Repeatedly to Catch Intermittent Races

```bash
# Run test 100 times
for i in {1..100}; do
    echo "Run $i"
    CGO_ENABLED=1 go test ./internal/api -race -run TestPubSub || break
done
```

### Use -count Flag

```bash
# Run each test 50 times
CGO_ENABLED=1 go test ./... -race -count=50
```

### Increase Timeout for Race Tests

Race detector makes tests slower (5-10x). Increase timeouts:

```bash
CGO_ENABLED=1 go test ./... -race -timeout 300s
```

### Add Stress Tests

```go
func TestConcurrentWrites(t *testing.T) {
    t.Parallel()  // Run in parallel with other tests
    
    store := NewStore()
    defer store.Close()
    
    const numGoroutines = 100
    const numWrites = 1000
    
    var wg sync.WaitGroup
    wg.Add(numGoroutines)
    
    for i := 0; i < numGoroutines; i++ {
        go func(id int) {
            defer wg.Done()
            for j := 0; j < numWrites; j++ {
                err := store.Write(fmt.Sprintf("key-%d-%d", id, j), "value")
                if err != nil {
                    t.Errorf("Write failed: %v", err)
                }
            }
        }(i)
    }
    
    wg.Wait()
}
```

---

## Debugging Race Conditions

### 1. **Read the Race Detector Output Carefully**

```
WARNING: DATA RACE
Write at 0x00c0001a0180 by goroutine 8:
  github.com/eventodb/eventodb/internal/api.(*PubSub).Publish()
      /path/to/pubsub.go:123 +0x1a4

Previous read at 0x00c0001a0180 by goroutine 7:
  github.com/eventodb/eventodb/internal/api.(*PubSub).SubscribeStream()
      /path/to/pubsub.go:45 +0x2c4

Goroutine 8 (running) created at:
  github.com/eventodb/eventodb/internal/api.TestPublish()
      /path/to/pubsub_test.go:67 +0x156
```

This tells you:
- **What**: Two goroutines accessing the same memory
- **Where**: Exact line numbers
- **When**: Which goroutines and how they were created

### 2. **Use GORACE Environment Variable**

```bash
# Get more detailed output
GORACE="log_path=/tmp/race.log history_size=7" \
  CGO_ENABLED=1 go test ./... -race

# Strip paths for cleaner output
GORACE="strip_path_prefix=/Users/roman/go/src/" \
  CGO_ENABLED=1 go test ./... -race

# Halt on first race detection
GORACE="halt_on_error=1" \
  CGO_ENABLED=1 go test ./... -race
```

### 3. **Add Logging Around Suspected Race Conditions**

```go
func (ps *PubSub) Publish(event WriteEvent) {
    log.Printf("Publish: acquiring lock for event %+v", event)
    ps.mu.RLock()
    defer func() {
        ps.mu.RUnlock()
        log.Printf("Publish: released lock for event %+v", event)
    }()
    
    // ... rest of code
}
```

---

## Integration with CI/CD

### Add Race Detection to Your CI Pipeline

```yaml
# .github/workflows/test.yml
- name: Run tests with race detector
  run: |
    cd golang
    go clean -testcache
    CGO_ENABLED=1 go test ./... -race -timeout 300s
```

### Create a Dedicated Race Detection Script

Create `bin/race_check.sh`:

```bash
#!/usr/bin/env bash
set -e

echo "Running race detection tests..."
cd golang

echo "Clearing test cache..."
go clean -testcache

echo "Running tests with race detector..."
CGO_ENABLED=1 go test ./... -race -timeout 300s -v

echo "✅ All race detection tests passed!"
```

---

## Common Pitfalls to Avoid

### 1. **Loop Variable Capture**
```go
// ❌ WRONG
for _, item := range items {
    go func() {
        process(item)  // All goroutines see last value!
    }()
}

// ✅ CORRECT
for _, item := range items {
    go func(i Item) {
        process(i)
    }(item)  // Pass as parameter
}
```

### 2. **Slice/Map Concurrent Modification**
```go
// ❌ WRONG - Concurrent map writes
var m = make(map[string]int)
go func() { m["a"] = 1 }()
go func() { m["b"] = 2 }()  // Race!

// ✅ CORRECT - Use sync.Map or mutex
var m sync.Map
go func() { m.Store("a", 1) }()
go func() { m.Store("b", 2) }()
```

### 3. **Closing Channels While Writing**
```go
// ❌ WRONG
ch := make(chan int)
go func() {
    for i := 0; i < 100; i++ {
        ch <- i  // Might write after close!
    }
}()
close(ch)  // Race!

// ✅ CORRECT - Signal completion
ch := make(chan int)
done := make(chan struct{})
go func() {
    defer close(done)
    for i := 0; i < 100; i++ {
        ch <- i
    }
}()
<-done
close(ch)
```

---

## Quick Reference

### Easy: Use Docker (Recommended for macOS)
```bash
./bin/race_check_docker.sh
```

### Enable Race Detector Locally
```bash
cd golang
go test ./... -race
```

### Clear Test Cache
```bash
go clean -testcache
```

### Run Race Tests with Verbose Output
```bash
go test ./... -race -v -timeout 300s
```

### Run Specific Package
```bash
go test ./internal/api -race -v
```

### Configure Race Detector
```bash
GORACE="halt_on_error=1 log_path=/tmp/race.log" go test ./... -race
```

### Stress Test
```bash
go test ./... -race -count=100 -timeout 600s
```

---

## Summary

1. **Always use `CGO_ENABLED=1` with `-race` flag**
2. **Clear test cache before race testing: `go clean -testcache`**
3. **Protect shared state with mutexes (RWMutex for read-heavy workloads)**
4. **Use `sync.WaitGroup` to coordinate goroutines**
5. **Use `atomic` package for simple counters**
6. **Always `defer` mutex unlocks**
7. **Pass loop variables as parameters to goroutines**
8. **Test with `-race -count=N` to catch intermittent races**
9. **Run race tests in CI/CD with increased timeout**
10. **Read race detector output carefully - it shows exact locations**

Your codebase already follows many best practices! The main issue was just the `CGO_ENABLED=0` flag preventing the race detector from working.
