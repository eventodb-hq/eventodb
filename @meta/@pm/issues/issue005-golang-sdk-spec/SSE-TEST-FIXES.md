# SSE Test Fixes - Summary

## Problem

Four SSE tests were skipped in the Go SDK due to timing issues and implementation gaps:

1. **SSE-002** - Category subscription (timing issues in tests)
2. **SSE-004** - Subscribe without auth (test mode)
3. **SSE-005** - Consumer group subscription (timing issues)
4. **SSE-007** - Reconnection handling (requires connection simulation)

## Root Causes

### 1. Race Condition in SSE Subscriptions

**Problem**: There was a race condition between:
- Establishing the SSE subscription in the pubsub system
- Writing a message to trigger a poke event
- Receiving the poke notification

The tests were using `time.Sleep()` to wait for subscription establishment, but this was unreliable.

**Solution**: Implemented a "ready signal" mechanism:
- Server now subscribes to pubsub **before** fetching existing messages
- Server sends a `: ready\n\n` comment immediately after subscription is established
- Client waits for this ready signal before proceeding with the test
- This eliminates the race condition and makes tests deterministic

### 2. Category Name Extraction Issue

**Problem**: The category extraction function uses the first dash (`-`) as a delimiter:
```go
func Category(streamName string) string {
    if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
        return streamName[:idx]
    }
    return streamName
}
```

When tests used `randomStreamName("sse-test")`, it created:
- Category: `sse-test-<uuid>` (intended category)
- Stream: `sse-test-<uuid>-123`
- Extracted category: `sse` (first part before dash)

The subscription was to `sse-test-<uuid>` but the pubsub was publishing to category `sse`.

**Solution**: Changed tests to use category names without dashes:
```go
category := fmt.Sprintf("ssetest%d", time.Now().UnixNano()%1000000)
stream := category + "-123"
```

Now:
- Category: `ssetest123456` (no dashes)
- Stream: `ssetest123456-123`
- Extracted category: `ssetest123456` (matches!)

### 3. Authentication Testing in Test Mode

**Problem**: The test server runs in "test mode" which allows missing authentication, so SSE-004 couldn't test the "subscribe without auth" failure case.

**Solution**: Created a separate test server instance in production mode (testMode=false) specifically for this test, which properly enforces authentication requirements.

### 4. Reconnection Position Tracking

**Problem**: The test was confusing global position with stream position when reconnecting.

**Solution**: Fixed the test to use stream position (not global position) when subscribing from a specific position after reconnection.

## Changes Made

### 1. Server-Side (`golang/internal/api/sse.go`)

#### `subscribeToStream` function:
- **Before**: Subscribe to pubsub after fetching existing messages
- **After**: Subscribe to pubsub FIRST, send ready signal, then fetch existing messages
- **Benefit**: Prevents race condition where messages written between fetch and subscribe are missed

#### `subscribeToCategory` function:
- Same change as `subscribeToStream`
- Ensures category subscriptions are fully established before processing

### 2. Client-Side (`golang/test_integration/sdk_spec_sse_test.go`)

#### `SSEClient` struct:
- Added `ready chan bool` channel to receive ready signal

#### `readEvents` function:
- Added logic to detect `: ready` comment and signal the ready channel
- Skip all comment lines (those starting with `:`)

#### `WaitForReady` function:
- New function to wait for subscription to be ready with timeout
- Returns error if ready signal not received within timeout

#### Test Changes:
- **SSE-001, SSE-002, SSE-003, SSE-005, SSE-006, SSE-008**: Replaced `time.Sleep()` with `WaitForReady()`
- **SSE-002, SSE-005**: Fixed category name to avoid dash conflicts
- **SSE-004**: Implemented proper test with production-mode server
- **SSE-007**: Fixed to use stream position instead of global position

## Test Results

All SSE tests now pass:
```
=== RUN   TestSSE001_SubscribeToStream
--- PASS: TestSSE001_SubscribeToStream (0.06s)
=== RUN   TestSSE002_SubscribeToCategory
--- PASS: TestSSE002_SubscribeToCategory (0.06s)
=== RUN   TestSSE003_SubscribeWithPosition
--- PASS: TestSSE003_SubscribeWithPosition (0.06s)
=== RUN   TestSSE004_SubscribeWithoutAuthentication
--- PASS: TestSSE004_SubscribeWithoutAuthentication (0.05s)
=== RUN   TestSSE005_SubscribeWithConsumerGroup
--- PASS: TestSSE005_SubscribeWithConsumerGroup (0.06s)
=== RUN   TestSSE006_MultipleSubscriptions
--- PASS: TestSSE006_MultipleSubscriptions (0.06s)
=== RUN   TestSSE007_ReconnectionHandling
--- PASS: TestSSE007_ReconnectionHandling (0.57s)
=== RUN   TestSSE008_PokeEventParsing
--- PASS: TestSSE008_PokeEventParsing (0.06s)
PASS
```

## Key Improvements

1. **Deterministic Tests**: Eliminated race conditions by using explicit ready signals
2. **Better Synchronization**: Subscribe-then-fetch pattern prevents missing messages
3. **Proper Auth Testing**: Can now test auth requirements in production mode
4. **Clearer Test Logic**: Position tracking is explicit and correct
5. **No More Skipped Tests**: All SSE tests are now enabled and passing

## Impact

- **Reliability**: Tests are now deterministic and won't fail due to timing issues
- **Coverage**: Full SSE functionality is now tested according to SDK-TEST-SPEC.md
- **Production Safety**: The ready signal mechanism also benefits production use by providing clear subscription state
- **Maintainability**: Other SDK implementations can follow the same pattern

## Future Considerations

The ready signal mechanism is now part of the SSE protocol. Other SDK implementations should:
1. Wait for the `: ready` comment before considering subscription established
2. Use category names without dashes when testing category subscriptions
3. Understand the difference between stream position and global position
4. Test authentication separately in production mode when needed
