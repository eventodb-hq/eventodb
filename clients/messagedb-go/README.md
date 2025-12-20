# messagedb-go

Go client for MessageDB - a simple, fast message store.

## Installation

```bash
go get github.com/messagedb/messagedb-go
```

## Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    messagedb "github.com/messagedb/messagedb-go"
)

func main() {
    // Create client
    client := messagedb.NewClient("http://localhost:8080", 
        messagedb.WithToken("ns_..."))
    
    ctx := context.Background()
    
    // Write message
    result, err := client.StreamWrite(ctx, "account-123", messagedb.Message{
        Type: "Deposited",
        Data: map[string]interface{}{"amount": 100},
    }, nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Written at position %d\n", result.Position)
    
    // Read stream
    messages, err := client.StreamGet(ctx, "account-123", nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Read %d messages\n", len(messages))
    
    // Get last message
    last, err := client.StreamLast(ctx, "account-123", nil)
    if err != nil {
        log.Fatal(err)
    }
    if last != nil {
        fmt.Printf("Last message: %+v\n", last)
    }
    
    // Read from category
    catMessages, err := client.CategoryGet(ctx, "account", nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Category has %d messages\n", len(catMessages))
    
    // Subscribe to real-time events
    sub, err := client.SubscribeStream(ctx, "account-123", nil)
    if err != nil {
        log.Fatal(err)
    }
    defer sub.Close()
    
    // Listen for poke events in background
    go func() {
        for poke := range sub.Events {
            fmt.Printf("New message at position %d\n", poke.Position)
        }
    }()
}
```

## Features

- ✅ **Stream Operations**: Write, read, get last message, check version
- ✅ **Category Operations**: Read with filtering, consumer groups, correlation
- ✅ **Namespace Operations**: Create, delete, list, get info
- ✅ **System Operations**: Version, health checks
- ✅ **Server-Sent Events**: Real-time subscriptions to streams and categories
- ✅ **Zero Dependencies**: Uses only Go standard library
- ✅ **Type-Safe**: Strongly typed message structures
- ✅ **Context-Aware**: All operations support context cancellation
- ✅ **Well-Tested**: 71 tests covering all functionality

## API

### Client Creation

```go
// Basic client
client := messagedb.NewClient("http://localhost:8080")

// With authentication token
client := messagedb.NewClient("http://localhost:8080",
    messagedb.WithToken("ns_..."))

// With custom HTTP client
httpClient := &http.Client{Timeout: 10 * time.Second}
client := messagedb.NewClient("http://localhost:8080",
    messagedb.WithHTTPClient(httpClient))
```

### Stream Operations

```go
// Write message
result, err := client.StreamWrite(ctx, streamName, messagedb.Message{
    Type: "EventType",
    Data: map[string]interface{}{"key": "value"},
    Metadata: map[string]interface{}{"correlationId": "123"},
}, nil)

// Write with options
result, err := client.StreamWrite(ctx, streamName, message, &messagedb.WriteOptions{
    ID: messagedb.StrPtr("custom-uuid"),
    ExpectedVersion: messagedb.Int64Ptr(5),
})

// Read stream
messages, err := client.StreamGet(ctx, streamName, nil)

// Read with options
messages, err := client.StreamGet(ctx, streamName, &messagedb.GetStreamOptions{
    Position: messagedb.Int64Ptr(10),
    BatchSize: messagedb.IntPtr(100),
})

// Get last message
last, err := client.StreamLast(ctx, streamName, nil)

// Get last message of specific type
last, err := client.StreamLast(ctx, streamName, &messagedb.GetLastOptions{
    Type: messagedb.StrPtr("EventType"),
})

// Get stream version
version, err := client.StreamVersion(ctx, streamName)
if version != nil {
    fmt.Printf("Stream version: %d\n", *version)
}
```

### Category Operations

```go
// Read category
messages, err := client.CategoryGet(ctx, "account", nil)

// Read with consumer group
messages, err := client.CategoryGet(ctx, "account", &messagedb.GetCategoryOptions{
    ConsumerGroup: &messagedb.ConsumerGroup{
        Member: 0,
        Size: 4,
    },
})

// Read with correlation filter
messages, err := client.CategoryGet(ctx, "account", &messagedb.GetCategoryOptions{
    Correlation: messagedb.StrPtr("workflow"),
})
```

### Namespace Operations

```go
// Create namespace
result, err := client.NamespaceCreate(ctx, "my-namespace", &messagedb.CreateNamespaceOptions{
    Description: messagedb.StrPtr("My test namespace"),
})

// Delete namespace
result, err := client.NamespaceDelete(ctx, "my-namespace")

// List namespaces
namespaces, err := client.NamespaceList(ctx)

// Get namespace info
info, err := client.NamespaceInfo(ctx, "my-namespace")
fmt.Printf("Message count: %d\n", info.MessageCount)
```

### System Operations

```go
// Get server version
version, err := client.SystemVersion(ctx)
fmt.Printf("Server version: %s\n", version)

// Get server health
health, err := client.SystemHealth(ctx)
fmt.Printf("Health status: %s\n", health.Status)
```

### Server-Sent Events (SSE)

```go
// Subscribe to stream
sub, err := client.SubscribeStream(ctx, "account-123", nil)
if err != nil {
    log.Fatal(err)
}
defer sub.Close()

// Listen for poke events
go func() {
    for {
        select {
        case poke := <-sub.Events:
            fmt.Printf("New message: stream=%s, position=%d, gpos=%d\n",
                poke.Stream, poke.Position, poke.GlobalPosition)
            
            // Fetch the new message
            messages, _ := client.StreamGet(ctx, poke.Stream, &messagedb.GetStreamOptions{
                Position: &poke.Position,
                BatchSize: messagedb.IntPtr(1),
            })
            
        case err := <-sub.Errors:
            log.Printf("Subscription error: %v", err)
            return
        }
    }
}()

// Subscribe to category
sub, err := client.SubscribeCategory(ctx, "account", nil)

// Subscribe to category with consumer group
sub, err := client.SubscribeCategory(ctx, "account", &messagedb.SubscribeCategoryOptions{
    ConsumerGroup: &messagedb.ConsumerGroup{
        Member: 0,
        Size: 4,
    },
})

// Subscribe from specific position
sub, err := client.SubscribeStream(ctx, "account-123", &messagedb.SubscribeStreamOptions{
    Position: messagedb.Int64Ptr(10),
})
```

### Error Handling

```go
result, err := client.StreamWrite(ctx, stream, message, &messagedb.WriteOptions{
    ExpectedVersion: messagedb.Int64Ptr(5),
})
if err != nil {
    var dbErr *messagedb.Error
    if errors.As(err, &dbErr) {
        switch dbErr.Code {
        case "STREAM_VERSION_CONFLICT":
            // Handle version conflict
        case "AUTH_REQUIRED":
            // Handle authentication error
        default:
            // Handle other errors
        }
    }
    return err
}
```

## Testing

Tests run against a live MessageDB server:

```bash
# Start server
docker-compose up -d

# Run tests
go test -v

# With custom URL and admin token
MESSAGEDB_URL=http://localhost:8080 MESSAGEDB_ADMIN_TOKEN=ns_... go test -v

# Run specific test
go test -v -run TestWRITE001

# Run with race detector
go test -v -race
```

## Message Format

### StreamMessage
Returned by `StreamGet` and `StreamLast`:
```go
type StreamMessage struct {
    ID             string                    // Message UUID
    Type           string                    // Event type
    Position       int64                     // Position in stream (0-indexed)
    GlobalPosition int64                     // Global position across all streams
    Data           map[string]interface{}    // Message data
    Metadata       map[string]interface{}    // Message metadata
    Time           time.Time                 // Timestamp (UTC)
}
```

### CategoryMessage
Returned by `CategoryGet`:
```go
type CategoryMessage struct {
    ID             string                    // Message UUID
    StreamName     string                    // Full stream name
    Type           string                    // Event type
    Position       int64                     // Position in stream
    GlobalPosition int64                     // Global position
    Data           map[string]interface{}    // Message data
    Metadata       map[string]interface{}    // Message metadata
    Time           time.Time                 // Timestamp (UTC)
}
```

## Helper Functions

The package provides helper functions for creating option pointers:

```go
// String pointer
description := messagedb.StrPtr("my description")

// Int64 pointer
position := messagedb.Int64Ptr(10)

// Int pointer
batchSize := messagedb.IntPtr(100)
```

## License

MIT
