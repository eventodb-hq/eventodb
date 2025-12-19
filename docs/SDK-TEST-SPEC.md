# MessageDB SDK Test Specification

This document defines a unified test suite that all MessageDB SDK implementations (Node.js, Go, Elixir, etc.) must pass to ensure consistent behavior across languages.

## Test Format

Each test case has:
- **ID**: `<CATEGORY>-<NUMBER>` (e.g., `WRITE-001`)
- **Description**: What the test validates
- **Setup**: Prerequisites or test data needed
- **Action**: SDK method to call with specific parameters
- **Expected**: Expected result or behavior
- **Notes**: Additional context or edge cases

## Test Categories

- **WRITE**: Stream writing operations
- **READ**: Stream reading operations
- **CATEGORY**: Category operations
- **NS**: Namespace operations
- **SYS**: System operations
- **AUTH**: Authentication and authorization
- **ERROR**: Error handling
- **ENCODING**: Data encoding and special characters
- **EDGE**: Edge cases and boundary conditions
- **SSE**: Server-Sent Events subscriptions

---

## WRITE - Stream Writing

### WRITE-001: Write minimal message
- **Setup**: Clean test stream (e.g., `test-${uuid}`)
- **Action**: `streamWrite(streamName, {type: "TestEvent", data: {foo: "bar"}})`
- **Expected**: Returns object with `position` (number >= 0) and `globalPosition` (number >= 0)
- **Notes**: First message in stream should have position 0

### WRITE-002: Write message with metadata
- **Setup**: Clean test stream
- **Action**: `streamWrite(streamName, {type: "TestEvent", data: {foo: "bar"}, metadata: {correlationId: "123"}})`
- **Expected**: Returns position info; subsequent read should return metadata exactly as written

### WRITE-003: Write with custom message ID
- **Setup**: Clean test stream, generate UUID (e.g., `550e8400-e29b-41d4-a716-446655440000`)
- **Action**: `streamWrite(streamName, message, {id: customUuid})`
- **Expected**: Returns position; subsequent read should show message ID matches custom UUID

### WRITE-004: Write with expected version (success)
- **Setup**: Stream with 2 messages (positions 0, 1)
- **Action**: `streamWrite(streamName, message, {expectedVersion: 1})`
- **Expected**: Returns position 2; write succeeds

### WRITE-005: Write with expected version (conflict)
- **Setup**: Stream with 2 messages (positions 0, 1)
- **Action**: `streamWrite(streamName, message, {expectedVersion: 5})`
- **Expected**: Throws/returns error with code `STREAM_VERSION_CONFLICT` or similar semantic error

### WRITE-006: Write multiple messages sequentially
- **Setup**: Clean test stream
- **Action**: Write 5 messages sequentially
- **Expected**: Positions are sequential (0, 1, 2, 3, 4); global positions are monotonically increasing

### WRITE-007: Write to stream with ID
- **Setup**: None
- **Action**: `streamWrite("account-123", message)`
- **Expected**: Success; stream name includes ID part

### WRITE-008: Write with empty data object
- **Setup**: Clean test stream
- **Action**: `streamWrite(streamName, {type: "TestEvent", data: {}})`
- **Expected**: Success; data is stored as empty object

### WRITE-009: Write with null metadata
- **Setup**: Clean test stream
- **Action**: `streamWrite(streamName, {type: "TestEvent", data: {x: 1}, metadata: null})`
- **Expected**: Success; metadata is stored as null

### WRITE-010: Write without authentication
- **Setup**: SDK client without auth token
- **Action**: `streamWrite(streamName, message)`
- **Expected**: Throws/returns error with code `AUTH_REQUIRED`

---

## READ - Stream Reading

### READ-001: Read from empty stream
- **Setup**: Non-existent stream name
- **Action**: `streamGet(streamName)`
- **Expected**: Returns empty array/list `[]`

### READ-002: Read single message
- **Setup**: Stream with 1 message at position 0
- **Action**: `streamGet(streamName)`
- **Expected**: Returns array with 1 message containing all fields: `[id, type, position, globalPosition, data, metadata, time]`

### READ-003: Read multiple messages
- **Setup**: Stream with 5 messages (positions 0-4)
- **Action**: `streamGet(streamName)`
- **Expected**: Returns 5 messages in ascending position order (0, 1, 2, 3, 4)

### READ-004: Read with position filter
- **Setup**: Stream with 10 messages (positions 0-9)
- **Action**: `streamGet(streamName, {position: 5})`
- **Expected**: Returns messages at positions 5, 6, 7, 8, 9 (5 messages total)

### READ-005: Read with global position filter
- **Setup**: Stream with messages at global positions 100, 101, 102, 103
- **Action**: `streamGet(streamName, {globalPosition: 102})`
- **Expected**: Returns messages at global positions 102, 103

### READ-006: Read with batch size limit
- **Setup**: Stream with 100 messages
- **Action**: `streamGet(streamName, {batchSize: 10})`
- **Expected**: Returns exactly 10 messages (positions 0-9)

### READ-007: Read with batch size unlimited
- **Setup**: Stream with 50 messages
- **Action**: `streamGet(streamName, {batchSize: -1})`
- **Expected**: Returns all 50 messages

### READ-008: Read message data integrity
- **Setup**: Write message with complex data: `{nested: {array: [1, 2, 3], bool: true, null: null}}`
- **Action**: Read message back
- **Expected**: Data field exactly matches written structure (deep equality)

### READ-009: Read message metadata integrity
- **Setup**: Write message with metadata: `{correlationId: "123", userId: "user-456"}`
- **Action**: Read message back
- **Expected**: Metadata field exactly matches written structure

### READ-010: Read message timestamp format
- **Setup**: Write a message
- **Action**: Read message back
- **Expected**: `time` field is valid ISO 8601 timestamp in UTC (matches pattern: `YYYY-MM-DDTHH:MM:SS.nnnnnnnnnZ`)

---

## LAST - Stream Last Message

### LAST-001: Last message from non-empty stream
- **Setup**: Stream with 5 messages
- **Action**: `streamLast(streamName)`
- **Expected**: Returns message at position 4 (last message)

### LAST-002: Last message from empty stream
- **Setup**: Non-existent stream
- **Action**: `streamLast(streamName)`
- **Expected**: Returns `null` or equivalent empty value

### LAST-003: Last message filtered by type
- **Setup**: Stream with messages: `[TypeA, TypeB, TypeA, TypeB, TypeA]` at positions 0-4
- **Action**: `streamLast(streamName, {type: "TypeB"})`
- **Expected**: Returns message at position 3 (last TypeB)

### LAST-004: Last message type filter no match
- **Setup**: Stream with only TypeA messages
- **Action**: `streamLast(streamName, {type: "TypeB"})`
- **Expected**: Returns `null`

---

## VERSION - Stream Version

### VERSION-001: Version of non-existent stream
- **Setup**: Non-existent stream
- **Action**: `streamVersion(streamName)`
- **Expected**: Returns `null` or equivalent empty value

### VERSION-002: Version of stream with messages
- **Setup**: Stream with 3 messages (positions 0, 1, 2)
- **Action**: `streamVersion(streamName)`
- **Expected**: Returns `2` (last position, 0-indexed)

### VERSION-003: Version after write
- **Setup**: Stream with 1 message (position 0)
- **Action**: Write another message, then get version
- **Expected**: Returns `1`

---

## CATEGORY - Category Reading

### CATEGORY-001: Read from category
- **Setup**: Write messages to `test-1`, `test-2`, `test-3` streams
- **Action**: `categoryGet("test")`
- **Expected**: Returns messages from all three streams

### CATEGORY-002: Read category with position filter
- **Setup**: Category with messages at global positions 100, 105, 110, 115
- **Action**: `categoryGet("test", {position: 110})`
- **Expected**: Returns messages at global positions 110, 115

### CATEGORY-003: Read category with batch size
- **Setup**: Category with 50 messages across multiple streams
- **Action**: `categoryGet("test", {batchSize: 10})`
- **Expected**: Returns exactly 10 messages

### CATEGORY-004: Category message format
- **Setup**: Write message to `test-123`
- **Action**: `categoryGet("test")`
- **Expected**: Message array includes `streamName` field (8 elements total): `[id, streamName, type, position, globalPosition, data, metadata, time]`

### CATEGORY-005: Category with consumer group
- **Setup**: Category with messages in streams: `test-1`, `test-2`, `test-3`, `test-4`
- **Action**: `categoryGet("test", {consumerGroup: {member: 0, size: 2}})`
- **Expected**: Returns messages from a deterministic subset of streams based on hash of cardinal ID

### CATEGORY-006: Category with correlation filter
- **Setup**: 
  - Write message to `test-1` with metadata: `{correlationStreamName: "workflow-123"}`
  - Write message to `test-2` with metadata: `{correlationStreamName: "other-456"}`
- **Action**: `categoryGet("test", {correlation: "workflow"})`
- **Expected**: Returns only message from `test-1`

### CATEGORY-007: Read from empty category
- **Setup**: No streams matching category
- **Action**: `categoryGet("nonexistent")`
- **Expected**: Returns empty array

### CATEGORY-008: Category global position ordering
- **Setup**: Category with messages across multiple streams
- **Action**: `categoryGet("test")`
- **Expected**: Messages returned in ascending global position order

---

## NS - Namespace Operations

### NS-001: Create namespace
- **Setup**: None
- **Action**: `namespaceCreate("test-ns-" + randomId, {description: "Test namespace"})`
- **Expected**: Returns object with `namespace`, `token`, `createdAt` fields; token starts with `ns_`

### NS-002: Create namespace with custom token
- **Setup**: Generate valid token for namespace "custom-ns"
- **Action**: `namespaceCreate("custom-ns", {token: customToken})`
- **Expected**: Returns info with specified token

### NS-003: Create duplicate namespace
- **Setup**: Existing namespace "duplicate-test"
- **Action**: `namespaceCreate("duplicate-test")`
- **Expected**: Throws/returns error with code `NAMESPACE_EXISTS`

### NS-004: Delete namespace
- **Setup**: Create namespace "delete-test"
- **Action**: `namespaceDelete("delete-test")`
- **Expected**: Returns object with `namespace`, `deletedAt`, `messagesDeleted` fields

### NS-005: Delete non-existent namespace
- **Setup**: None
- **Action**: `namespaceDelete("does-not-exist")`
- **Expected**: Throws/returns error with code `NAMESPACE_NOT_FOUND`

### NS-006: List namespaces
- **Setup**: At least one namespace exists
- **Action**: `namespaceList()`
- **Expected**: Returns array of namespace objects with `namespace`, `description`, `createdAt`, `messageCount` fields

### NS-007: Get namespace info
- **Setup**: Namespace "info-test" with 5 messages
- **Action**: `namespaceInfo("info-test")`
- **Expected**: Returns object with correct `messageCount: 5`

### NS-008: Get info for non-existent namespace
- **Setup**: None
- **Action**: `namespaceInfo("does-not-exist")`
- **Expected**: Throws/returns error with code `NAMESPACE_NOT_FOUND`

---

## SYS - System Operations

### SYS-001: Get server version
- **Setup**: None
- **Action**: `systemVersion()`
- **Expected**: Returns string matching semver pattern (e.g., "1.3.0")

### SYS-002: Get server health
- **Setup**: None
- **Action**: `systemHealth()`
- **Expected**: Returns object with `status` field (value should be "ok" or similar)

---

## AUTH - Authentication

### AUTH-001: Valid token authentication
- **Setup**: SDK configured with valid token
- **Action**: `streamWrite(streamName, message)`
- **Expected**: Success

### AUTH-002: Missing token
- **Setup**: SDK configured without token
- **Action**: `streamWrite(streamName, message)`
- **Expected**: Throws/returns error with code `AUTH_REQUIRED`

### AUTH-003: Invalid token format
- **Setup**: SDK configured with malformed token (e.g., "invalid-token")
- **Action**: `streamWrite(streamName, message)`
- **Expected**: Throws/returns error with code `AUTH_INVALID` or `AUTH_REQUIRED`

### AUTH-004: Token namespace isolation
- **Setup**: Two namespaces (ns1, ns2) with different tokens
- **Action**: 
  - Write message to stream in ns1 using token1
  - Try to read same stream name using token2
- **Expected**: Token2 cannot see messages from token1 (returns empty or error)

---

## ERROR - Error Handling

### ERROR-001: Invalid RPC method
- **Setup**: None
- **Action**: Call non-existent method (implementation-specific, e.g., `client.rawCall(["invalid.method"])`)
- **Expected**: Throws/returns error with code `INVALID_REQUEST` or method not found error

### ERROR-002: Missing required argument
- **Setup**: None
- **Action**: `streamWrite(streamName)` (missing message argument)
- **Expected**: Throws error (SDK should validate required arguments)

### ERROR-003: Invalid stream name type
- **Setup**: None
- **Action**: `streamWrite(123, message)` (number instead of string)
- **Expected**: Throws error (SDK should validate argument types)

### ERROR-004: Connection refused
- **Setup**: SDK configured to connect to non-existent server (e.g., `http://localhost:99999`)
- **Action**: `streamWrite(streamName, message)`
- **Expected**: Throws connection error with clear message

### ERROR-005: Server returns malformed JSON
- **Setup**: Mock/test server returning invalid JSON
- **Action**: Any RPC call
- **Expected**: Throws parse error with helpful message

### ERROR-006: Network timeout
- **Setup**: SDK with short timeout (e.g., 100ms), slow/hanging server
- **Action**: Any RPC call
- **Expected**: Throws timeout error after configured duration

### ERROR-007: HTTP error status
- **Setup**: Server returning 500 error
- **Action**: Any RPC call
- **Expected**: Throws error indicating server error

---

## ENCODING - Data Encoding

### ENCODING-001: UTF-8 text in data
- **Setup**: None
- **Action**: Write message with data: `{text: "Hello 世界 🌍 émojis"}`
- **Expected**: Read back with exact same UTF-8 content

### ENCODING-002: Unicode in metadata
- **Setup**: None
- **Action**: Write message with metadata: `{description: "Test 测试 🎉"}`
- **Expected**: Read back with exact same content

### ENCODING-003: Special characters in stream name
- **Setup**: None
- **Action**: `streamWrite("test-stream_123.abc", message)`
- **Expected**: Success (if server allows) or clear error

### ENCODING-004: Empty string values
- **Setup**: None
- **Action**: Write message with data: `{emptyString: ""}`
- **Expected**: Read back with empty string, not null

### ENCODING-005: Boolean values
- **Setup**: None
- **Action**: Write message with data: `{isTrue: true, isFalse: false}`
- **Expected**: Read back with correct boolean types (not strings)

### ENCODING-006: Null values
- **Setup**: None
- **Action**: Write message with data: `{nullValue: null}`
- **Expected**: Read back as null (not undefined or missing)

### ENCODING-007: Numeric values
- **Setup**: None
- **Action**: Write message with data: `{integer: 42, float: 3.14159, negative: -100, zero: 0}`
- **Expected**: Read back with correct numeric values and types

### ENCODING-008: Nested objects
- **Setup**: None
- **Action**: Write message with deeply nested data: `{level1: {level2: {level3: {value: "deep"}}}}`
- **Expected**: Read back with exact structure preserved

### ENCODING-009: Arrays in data
- **Setup**: None
- **Action**: Write message with data: `{items: [1, "two", {three: 3}, null, true]}`
- **Expected**: Read back with exact array structure and mixed types

### ENCODING-010: Large message payload
- **Setup**: None
- **Action**: Write message with large data object (e.g., 100KB of JSON)
- **Expected**: Success (if under server limit) or clear size limit error

---

## EDGE - Edge Cases

### EDGE-001: Empty batch size behavior
- **Setup**: Stream with 10 messages
- **Action**: `streamGet(streamName, {batchSize: 0})`
- **Expected**: Returns empty array or error (SDK should document behavior)

### EDGE-002: Negative position
- **Setup**: Stream with messages
- **Action**: `streamGet(streamName, {position: -1})`
- **Expected**: Error or returns empty array (SDK should validate or document)

### EDGE-003: Very large batch size
- **Setup**: Stream with 5 messages
- **Action**: `streamGet(streamName, {batchSize: 1000000})`
- **Expected**: Returns 5 messages (server may cap at max, e.g., 10000)

### EDGE-004: Stream name edge cases
- **Setup**: None
- **Action**: Test stream names: `"a"`, `"stream-with-many-dashes"`, `"stream123"`, `"UPPERCASE"`
- **Expected**: All should work or fail consistently with clear errors

### EDGE-005: Concurrent writes to same stream
- **Setup**: Stream exists
- **Action**: Write 10 messages concurrently (parallel requests)
- **Expected**: All succeed; positions are unique and sequential (may be in any order)

### EDGE-006: Read from position beyond stream end
- **Setup**: Stream with 5 messages (positions 0-4)
- **Action**: `streamGet(streamName, {position: 100})`
- **Expected**: Returns empty array

### EDGE-007: Expected version -1 (no stream)
- **Setup**: Non-existent stream
- **Action**: `streamWrite(streamName, message, {expectedVersion: -1})`
- **Expected**: Success if stream doesn't exist; conflict if it does

### EDGE-008: Expected version 0 (first message)
- **Setup**: Non-existent stream
- **Action**: `streamWrite(streamName, message, {expectedVersion: 0})`
- **Expected**: Conflict (stream version should be -1 for non-existent stream)

---

## SSE - Server-Sent Events

### SSE-001: Subscribe to stream
- **Setup**: Stream "sse-test-stream"
- **Action**: Subscribe to stream, then write a message to it
- **Expected**: Receive poke event with `{stream, position, globalPosition}`

### SSE-002: Subscribe to category
- **Setup**: None
- **Action**: Subscribe to category "sse-test", then write message to "sse-test-123"
- **Expected**: Receive poke event for new message

### SSE-003: Subscribe with position
- **Setup**: Stream with 5 messages (positions 0-4)
- **Action**: Subscribe from position 3
- **Expected**: Receive pokes for positions 3, 4, and any new messages

### SSE-004: Subscribe without authentication
- **Setup**: None
- **Action**: Subscribe without token parameter
- **Expected**: Connection error or authentication error

### SSE-005: Subscribe with consumer group
- **Setup**: Category "sse-test"
- **Action**: Subscribe with `consumerGroup: {member: 0, size: 2}`
- **Expected**: Receive pokes only for streams in member 0's partition

### SSE-006: Multiple subscriptions
- **Setup**: None
- **Action**: Create 2 separate subscriptions to different streams
- **Expected**: Both subscriptions receive their respective pokes independently

### SSE-007: Reconnection handling
- **Setup**: Active subscription
- **Action**: Simulate connection drop, reconnect
- **Expected**: SDK handles reconnection gracefully, resuming from last known position

### SSE-008: Poke event parsing
- **Setup**: Active subscription
- **Action**: Write message and receive poke
- **Expected**: Poke data is valid JSON with required fields: `stream` (or category), `position`, `globalPosition`

---

## Implementation Notes

### For SDK Developers

1. **Test Naming**: Use test IDs in test names for traceability
   ```javascript
   // JavaScript/Node.js
   test('WRITE-001: Write minimal message', async () => { ... })
   ```
   ```go
   // Go
   func TestWRITE001_WriteMinimalMessage(t *testing.T) { ... }
   ```
   ```elixir
   # Elixir
   test "WRITE-001: Write minimal message" do ... end
   ```

2. **Test Fixtures**: Use unique stream names per test run to avoid conflicts
   ```javascript
   const streamName = `test-${Date.now()}-${Math.random().toString(36).substring(7)}`;
   ```

3. **Cleanup**: Clean up test namespaces after test runs when possible

4. **Flaky Tests**: Network/SSE tests may be flaky; implement retries where appropriate

5. **Skip vs Fail**: Some tests may be skipped if features aren't implemented yet; document this clearly

### Compliance Tracking

Use a compliance matrix to track SDK test coverage:

| Test ID | Node.js | Go | Elixir | Notes |
|---------|---------|-----|--------|-------|
| WRITE-001 | ✅ | ✅ | ✅ | |
| WRITE-002 | ✅ | ✅ | ⏳ | Elixir WIP |
| AUTH-001 | ✅ | ❌ | ✅ | Go needs impl |
| SSE-007 | ⏳ | ⏳ | ⏳ | Complex to test |

**Legend:**
- ✅ Implemented and passing
- ⏳ Work in progress
- ❌ Not yet implemented
- 🚫 Not applicable for this SDK

### Test Data Standards

Use consistent test data across SDKs for easier comparison:

```json
{
  "type": "TestEvent",
  "data": {
    "testId": "WRITE-001",
    "timestamp": "2024-01-15T10:30:00Z",
    "value": 42
  },
  "metadata": {
    "correlationId": "test-correlation-123"
  }
}
```

### Coverage Goals

- **Tier 1 (Must Have)**: WRITE-001 to WRITE-009, READ-001 to READ-010, AUTH-001 to AUTH-004, ERROR-001 to ERROR-004
- **Tier 2 (Should Have)**: All CATEGORY, NS, SYS, LAST, VERSION tests
- **Tier 3 (Nice to Have)**: All ENCODING, EDGE, SSE tests

New SDKs should aim for 100% Tier 1 coverage before initial release.

---

## Version History

- **1.0.0** (2024-12-19): Initial test specification with 80+ test cases covering all MessageDB API endpoints
