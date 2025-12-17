# ADR-001: RPC-Style API Format

**Date:** 2024-12-17  
**Status:** Accepted  
**Context:** Designing a REST API wrapper for Message DB to abstract Postgres implementation

---

## Decision

We will implement an RPC-style API using compact JSON format with the following constraints:

### Core Principles

1. **Single RPC endpoint**: `POST /rpc`
2. **Maximum 4 arguments per method**: `["method", arg1, arg2, arg3]`
3. **Method naming**: `noun.verb` pattern (e.g., `stream.write`, `category.get`)
4. **Compact JSON**: Arrays for efficiency over objects
5. **Lightweight streaming**: SSE streams "pokes" (notifications) only, not full messages

---

## API Methods

### 1. Write Message

```json
["stream.write", "streamName", {msg}, {opts}]
```

**msg object:**
```json
{
  "type": "Withdrawn",
  "data": {"amount": 50},
  "metadata": {"userId": "u1"}  // optional
}
```

**opts object (optional):**
```json
{
  "id": "a5eb2a97-...",  // optional, server generates if not provided
  "expectedVersion": 5   // optional
}
```

**Response:**
```json
{"position": 6, "globalPosition": 1234}
```

---

### 2. Get Stream Messages

```json
["stream.get", "streamName", {opts}]
```

**opts object (optional):**
```json
{
  "position": 0,          // stream position
  "globalPosition": 1235, // OR global position (mutually exclusive)
  "batchSize": 100,
  "condition": null       // SQL condition if needed
}
```

**Response:**
```json
[
  ["id1", "Opened", 0, 1000, {...data}, {...meta}, "2024-12-17T01:00:00Z"],
  ["id2", "Deposited", 1, 1001, {...data}, {...meta}, "2024-12-17T01:01:00Z"]
]
```

**Format:** `[messageId, type, position, globalPosition, data, metadata, time]`

---

### 3. Get Category Messages

```json
["category.get", "categoryName", {opts}]
```

**opts object:**
```json
{
  "position": 0,           // global position for category
  "globalPosition": 1235,  // alternative
  "batchSize": 100,
  "correlation": null,
  "consumerGroup": {"member": 0, "size": 2},  // optional
  "condition": null
}
```

**Response:**
```json
[
  ["id1", "account-123", "Opened", 0, 1000, {...}, {...}, "2024-12-17T01:00:00Z"],
  ["id2", "account-456", "Deposited", 1, 1001, {...}, {...}, "2024-12-17T01:01:00Z"]
]
```

**Format:** `[messageId, streamName, type, position, globalPosition, data, metadata, time]`

Note: Stream name is included since messages come from multiple streams.

---

### 4. Get Last Message

```json
["stream.last", "streamName", {opts}]
```

**opts (optional):**
```json
{
  "type": "Withdrawn"  // filter by message type
}
```

**Response:**
```json
["id", "Withdrawn", 5, 1234, {...data}, {...meta}, "2024-12-17T01:00:00Z"]
```

Returns `null` if not found.

---

### 5. Get Stream Version

```json
["stream.version", "streamName"]
```

**Response:**
```json
5  // or null if stream doesn't exist
```

---

## Subscription (Server-Sent Events)

### Philosophy

Based on [HN discussion](https://news.ycombinator.com/item?id=21810272): Stream only lightweight "pokes" (notifications), not full message data. Clients fetch actual data separately.

**Benefits:**
- Lightweight streaming connection
- No message duplication in memory
- Client controls polling/fetching strategy
- Server doesn't buffer messages for slow clients

---

### Subscribe to Stream

```
GET /subscribe?stream=account-123&position=5
```

**SSE Event Format:**
```
event: poke
data: {"stream": "account-123", "position": 6, "globalPosition": 1235}

event: poke
data: {"stream": "account-123", "position": 7, "globalPosition": 1236}
```

**Client Flow:**
1. Receive poke with position info
2. Fetch actual data: `["stream.get", "account-123", {"position": 6, "batchSize": 10}]`

---

### Subscribe to Category

```
GET /subscribe?category=account&position=1000&consumer=0&size=2
```

**SSE Event Format:**
```
event: poke
data: {"stream": "account-123", "position": 6, "globalPosition": 1235}

event: poke
data: {"stream": "account-456", "position": 2, "globalPosition": 1236}
```

**Client Flow:**
1. Receive poke with global position
2. Fetch actual data: `["category.get", "account", {"globalPosition": 1235, "batchSize": 10}]`

---

## System Methods

### Get Server Version
```json
["sys.version"]
```

**Response:** `"1.3.0"`

---

### Health Check
```json
["sys.health"]
```

**Response:**
```json
{"status": "ok", "backend": "postgres", "connections": 5}
```

---

### Namespace Management (Optional Multi-tenancy)

```json
["ns.create", "tenant-a", {opts}]
```

**Response:**
```json
{"namespace": "tenant-a", "created": true}
```

```json
["ns.list"]
```

**Response:**
```json
["tenant-a", "tenant-b"]
```

---

## Namespace Support

**Approach:** Namespace prefix in stream name

```json
["stream.write", "tenant-a:account-123", {...}]
```

**Rationale:**
- Explicit and visible
- Works with any HTTP client
- No routing complexity
- Easy to parse and validate

**Alternatives considered:**
- Separate endpoint per namespace: `/rpc/tenant-a` (adds routing complexity)
- Header-based: `X-MessageDB-Namespace: tenant-a` (less explicit)

---

## Error Response Format

```json
{
  "error": {
    "code": "STREAM_VERSION_CONFLICT", 
    "message": "Expected version 5, stream is at version 6",
    "details": {"expected": 5, "actual": 6}
  }
}
```

**Common Error Codes:**
- `STREAM_VERSION_CONFLICT` - Optimistic concurrency violation
- `INVALID_REQUEST` - Malformed request
- `STREAM_NOT_FOUND` - Stream doesn't exist
- `NAMESPACE_NOT_FOUND` - Invalid namespace
- `BACKEND_ERROR` - Database/storage error

---

## Complete Example

### 1. Write an event
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -d '[
    "stream.write",
    "account-123",
    {"type": "Withdrawn", "data": {"amount": 50}},
    {"expectedVersion": 5}
  ]'
```

**Response:**
```json
{"position": 6, "globalPosition": 1234}
```

---

### 2. Subscribe to notifications
```bash
curl -N http://localhost:8080/subscribe?stream=account-123&position=5
```

**Server streams pokes:**
```
event: poke
data: {"stream": "account-123", "position": 6, "globalPosition": 1235}
```

---

### 3. Fetch the actual data
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -d '["stream.get", "account-123", {"position": 6, "batchSize": 10}]'
```

**Response:**
```json
[
  ["id1", "Deposited", 6, 1235, {"amount": 25}, {}, "2024-12-17T01:10:00Z"],
  ["id2", "Withdrawn", 7, 1236, {"amount": 10}, {}, "2024-12-17T01:11:00Z"]
]
```

---

## Rationale

### Why RPC over REST?

1. **Simplicity**: Single endpoint, method-based routing
2. **Efficiency**: Compact JSON reduces payload size
3. **Flexibility**: Easy to add new methods without URL design
4. **Event Sourcing fit**: Aligns with command/query pattern

### Why Compact JSON?

1. **Smaller payloads**: Arrays are more compact than objects
2. **Faster parsing**: Positional arguments are predictable
3. **Type safety**: Easy to validate with schemas
4. **Batch-friendly**: Natural array nesting for batch operations

### Why Poke-Only Streaming?

1. **Resource efficiency**: No message buffering on server
2. **Backpressure control**: Client decides when to fetch
3. **Reliability**: Fetch failures don't lose messages
4. **Scalability**: Server doesn't track per-client state beyond position

---

## Future Considerations

- **Batch operations**: `[["stream.write", ...], ["stream.write", ...]]`
- **Binary encoding**: MessagePack/CBOR for even more compactness
- **GraphQL-style field selection**: `{"fields": ["id", "type", "data"]}`
- **Compression**: gzip/brotli for responses

---

## References

- [HN Discussion on Postgres-based messaging](https://news.ycombinator.com/item?id=21810272)
- [Compact JSON Spec](https://jsonjoy.com/specs/compact-json/examples)
- [Message DB Documentation](http://docs.eventide-project.org/user-guide/message-db/)
