# MessageDB API Reference

Complete API reference for the MessageDB Go server.

## Overview

MessageDB uses a JSON-RPC style API over HTTP POST. All requests are sent to the `/rpc` endpoint.

### Request Format

```json
["method", arg1, arg2, ...]
```

### Response Format

**Success:**
```json
<result>
```

**Error:**
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": { ... }
  }
}
```

### Authentication

Include your namespace token in the `Authorization` header:

```http
Authorization: Bearer ns_ZGVmYXVsdA_a1b2c3d4e5f6...
```

In test mode, the server auto-creates namespaces and returns tokens in the `X-MessageDB-Token` header.

---

## Stream Operations

### stream.write

Write a message to a stream.

**Request:**
```json
["stream.write", "streamName", {
  "type": "EventType",
  "data": { ... },
  "metadata": { ... }
}, {
  "id": "custom-uuid",
  "expectedVersion": 5
}]
```

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `streamName` | string | Yes | Target stream (e.g., `account-123`) |
| `message` | object | Yes | Message to write |
| `message.type` | string | Yes | Event type name |
| `message.data` | object | Yes | Event payload |
| `message.metadata` | object | No | Optional metadata |
| `options` | object | No | Write options |
| `options.id` | string | No | Custom message UUID (auto-generated if omitted) |
| `options.expectedVersion` | number | No | Expected stream version for optimistic locking |

**Response:**
```json
{
  "position": 0,
  "globalPosition": 1234
}
```

**Error Codes:**
- `INVALID_REQUEST` - Invalid arguments
- `STREAM_VERSION_CONFLICT` - Expected version doesn't match actual version
- `AUTH_REQUIRED` - No authentication token provided
- `BACKEND_ERROR` - Database error

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.write", "account-123", {"type": "Deposited", "data": {"amount": 100}}, {"expectedVersion": 0}]'
```

---

### stream.get

Read messages from a stream.

**Request:**
```json
["stream.get", "streamName", {
  "position": 0,
  "batchSize": 100
}]
```

**Arguments:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `streamName` | string | Yes | - | Stream to read from |
| `options.position` | number | No | 0 | Starting position (inclusive) |
| `options.globalPosition` | number | No | - | Alternative: filter by global position |
| `options.batchSize` | number | No | 1000 | Max messages to return (-1 for unlimited, max 10000) |

**Response:**
```json
[
  ["msg-uuid-1", "EventType", 0, 1001, {"field": "value"}, {"correlationStreamName": "workflow-1"}, "2024-01-15T10:30:00.123456789Z"],
  ["msg-uuid-2", "EventType", 1, 1002, {"field": "value"}, null, "2024-01-15T10:30:01.123456789Z"]
]
```

**Message Array Format:**
| Index | Field | Description |
|-------|-------|-------------|
| 0 | `id` | Message UUID |
| 1 | `type` | Event type |
| 2 | `position` | Stream position |
| 3 | `globalPosition` | Global sequence number |
| 4 | `data` | Event payload |
| 5 | `metadata` | Message metadata |
| 6 | `time` | ISO 8601 timestamp (UTC) |

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.get", "account-123", {"position": 0, "batchSize": 10}]'
```

---

### stream.last

Get the last message from a stream.

**Request:**
```json
["stream.last", "streamName", {"type": "SpecificType"}]
```

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `streamName` | string | Yes | Stream to read from |
| `options.type` | string | No | Filter by event type |

**Response:**
```json
["msg-uuid", "EventType", 5, 1234, {"field": "value"}, null, "2024-01-15T10:30:00Z"]
```

Returns `null` if stream is empty or doesn't exist.

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.last", "account-123", {"type": "Deposited"}]'
```

---

### stream.version

Get the current version (latest position) of a stream.

**Request:**
```json
["stream.version", "streamName"]
```

**Response:**
```json
5
```

Returns `null` if stream doesn't exist.

**Note:** Version is 0-based, so version 5 means 6 messages (positions 0-5).

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.version", "account-123"]'
```

---

## Category Operations

### category.get

Read messages from all streams in a category.

**Request:**
```json
["category.get", "categoryName", {
  "position": 0,
  "batchSize": 100,
  "correlation": "workflow",
  "consumerGroup": {
    "member": 0,
    "size": 4
  }
}]
```

**Arguments:**
| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `categoryName` | string | Yes | - | Category to query (e.g., `account`) |
| `options.position` | number | No | 0 | Starting global position |
| `options.globalPosition` | number | No | - | Alternative to position |
| `options.batchSize` | number | No | 1000 | Max messages to return |
| `options.correlation` | string | No | - | Filter by correlationStreamName category |
| `options.consumerGroup.member` | number | No | - | Consumer group member index (0-based) |
| `options.consumerGroup.size` | number | No | - | Total number of consumers |

**Response:**
```json
[
  ["msg-uuid-1", "account-123", "Deposited", 0, 1001, {"amount": 100}, null, "2024-01-15T10:30:00Z"],
  ["msg-uuid-2", "account-456", "Withdrawn", 0, 1002, {"amount": 50}, null, "2024-01-15T10:30:01Z"]
]
```

**Category Message Array Format:**
| Index | Field | Description |
|-------|-------|-------------|
| 0 | `id` | Message UUID |
| 1 | `streamName` | Full stream name (includes ID) |
| 2 | `type` | Event type |
| 3 | `position` | Stream position |
| 4 | `globalPosition` | Global sequence number |
| 5 | `data` | Event payload |
| 6 | `metadata` | Message metadata |
| 7 | `time` | ISO 8601 timestamp (UTC) |

**Consumer Groups:**

Consumer groups allow multiple consumers to process category messages without overlap. Each consumer receives a deterministic subset of streams based on a hash of the stream's cardinal ID.

```json
{
  "consumerGroup": {
    "member": 0,  // This consumer's index (0, 1, 2, or 3)
    "size": 4     // Total number of consumers
  }
}
```

Cardinal ID is extracted from compound IDs:
- `account-123` → cardinal ID: `123`
- `account-123+retry` → cardinal ID: `123` (same consumer)

**Correlation Filtering:**

Filter messages by the category of their `correlationStreamName` metadata:

```json
{
  "correlation": "workflow"  // Match correlationStreamName starting with "workflow"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["category.get", "account", {"batchSize": 100, "consumerGroup": {"member": 0, "size": 4}}]'
```

---

## Namespace Operations

### ns.create

Create a new namespace.

**Request:**
```json
["ns.create", "my-namespace", {
  "description": "My application namespace",
  "token": "ns_bXktbmFtZXNwYWNl_custom..."
}]
```

**Arguments:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `namespaceId` | string | Yes | Unique namespace identifier |
| `options.description` | string | No | Human-readable description |
| `options.token` | string | No | Custom token (must be valid format for namespace) |

**Response:**
```json
{
  "namespace": "my-namespace",
  "token": "ns_bXktbmFtZXNwYWNl_a1b2c3d4...",
  "createdAt": "2024-01-15T10:30:00.123456789Z"
}
```

**Error Codes:**
- `NAMESPACE_EXISTS` - Namespace already exists
- `INVALID_REQUEST` - Invalid namespace ID or token format

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -d '["ns.create", "tenant-a", {"description": "Tenant A production"}]'
```

---

### ns.delete

Delete a namespace and all its data.

**Request:**
```json
["ns.delete", "namespace-id"]
```

**Response:**
```json
{
  "namespace": "namespace-id",
  "deletedAt": "2024-01-15T10:30:00.123456789Z",
  "messagesDeleted": 1543
}
```

**Error Codes:**
- `NAMESPACE_NOT_FOUND` - Namespace doesn't exist

**⚠️ Warning:** This operation is irreversible and deletes all messages in the namespace.

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["ns.delete", "tenant-a"]'
```

---

### ns.list

List all namespaces.

**Request:**
```json
["ns.list"]
```

**Response:**
```json
[
  {
    "namespace": "default",
    "description": "Default namespace",
    "createdAt": "2024-01-15T10:30:00Z",
    "messageCount": 1234
  },
  {
    "namespace": "tenant-a",
    "description": "Tenant A",
    "createdAt": "2024-01-16T10:30:00Z",
    "messageCount": 567
  }
]
```

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["ns.list"]'
```

---

### ns.info

Get detailed information about a namespace.

**Request:**
```json
["ns.info", "namespace-id"]
```

**Response:**
```json
{
  "namespace": "tenant-a",
  "description": "Tenant A production",
  "createdAt": "2024-01-15T10:30:00Z",
  "messageCount": 1543,
  "streamCount": 42,
  "lastActivity": "2024-01-17T15:45:30Z"
}
```

**Error Codes:**
- `NAMESPACE_NOT_FOUND` - Namespace doesn't exist

**Example:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["ns.info", "tenant-a"]'
```

---

## System Operations

### sys.version

Get the server version.

**Request:**
```json
["sys.version"]
```

**Response:**
```json
"1.3.0"
```

---

### sys.health

Get server health status.

**Request:**
```json
["sys.health"]
```

**Response:**
```json
{
  "status": "ok",
  "backend": "sqlite",
  "connections": 5
}
```

---

## Server-Sent Events (SSE)

### GET /subscribe

Subscribe to real-time notifications when messages are written.

**Query Parameters:**
| Name | Type | Required | Description |
|------|------|----------|-------------|
| `stream` | string | * | Stream to subscribe to |
| `category` | string | * | Category to subscribe to |
| `position` | number | No | Starting position (default: 0) |
| `consumerGroupMember` | number | No | Consumer group member index |
| `consumerGroupSize` | number | No | Consumer group size |
| `token` | string | Yes | Authentication token |

*Either `stream` or `category` is required (not both).

**Poke Event Format:**
```
event: poke
data: {"stream":"account-123","position":5,"globalPosition":1234}
```

**Example - Stream Subscription:**
```bash
curl -N "http://localhost:8080/subscribe?stream=account-123&position=0&token=$TOKEN"
```

**Example - Category Subscription with Consumer Group:**
```bash
curl -N "http://localhost:8080/subscribe?category=account&consumerGroupMember=0&consumerGroupSize=4&token=$TOKEN"
```

**JavaScript Example:**
```javascript
const eventSource = new EventSource(
  `http://localhost:8080/subscribe?stream=account-123&token=${token}`
);

eventSource.addEventListener('poke', (event) => {
  const poke = JSON.parse(event.data);
  console.log(`New message at position ${poke.position}`);
});
```

---

## HTTP Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/rpc` | POST | RPC API endpoint |
| `/subscribe` | GET | SSE subscription endpoint |
| `/health` | GET | Health check (returns `{"status":"ok"}`) |
| `/version` | GET | Version info (returns `{"version":"1.3.0"}`) |

---

## Error Codes Reference

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVALID_REQUEST` | 400 | Malformed request or invalid arguments |
| `AUTH_REQUIRED` | 401 | No authentication token provided |
| `AUTH_INVALID` | 401 | Invalid or expired token |
| `NAMESPACE_NOT_FOUND` | 404 | Namespace doesn't exist |
| `NAMESPACE_EXISTS` | 409 | Namespace already exists |
| `STREAM_VERSION_CONFLICT` | 409 | Optimistic locking conflict |
| `BACKEND_ERROR` | 500 | Database or internal error |

---

## Token Format

Namespace tokens follow the format:

```
ns_<base64url-encoded-namespace>_<random-suffix>
```

Example for namespace `default`:
```
ns_ZGVmYXVsdA_a1b2c3d4e5f6g7h8
```

Tokens can be parsed to extract the namespace:
```javascript
const parts = token.split('_');
const namespace = atob(parts[1].replace(/-/g, '+').replace(/_/g, '/'));
```

---

## Rate Limits

Default limits (configurable):

| Resource | Limit |
|----------|-------|
| Requests per second | 1000 |
| Max batch size | 10000 |
| Max message size | 1 MB |
| Max header size | 1 MB |
| SSE connections per token | 100 |

---

## Best Practices

1. **Use optimistic locking** for aggregate streams to prevent concurrent write conflicts
2. **Use consumer groups** for scaling category processors
3. **Set appropriate batch sizes** - smaller batches for real-time, larger for catch-up
4. **Use correlation** to track message relationships across streams
5. **Subscribe from last known position** to avoid reprocessing messages
