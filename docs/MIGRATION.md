# Migration Guide: PostgreSQL Message DB to MessageDB Go

This guide helps you migrate from the original PostgreSQL-based Message DB to the MessageDB Go server.

## Overview

The MessageDB Go server provides HTTP API access to message store functionality. It can work with:
- **In-memory SQLite** for testing and development
- **PostgreSQL** (planned) for production with existing Message DB databases

## Key Differences

### API Access

| Feature | Original Message DB | MessageDB Go |
|---------|---------------------|--------------|
| Access Method | Direct PostgreSQL functions | HTTP RPC API |
| Authentication | PostgreSQL roles | Token-based (Bearer) |
| Multi-tenancy | Database per tenant | Namespace per tenant |
| Real-time | PostgreSQL NOTIFY | Server-Sent Events (SSE) |

### Function Mapping

| PostgreSQL Function | RPC Method | Notes |
|---------------------|------------|-------|
| `write_message(...)` | `stream.write` | Same semantics |
| `get_stream_messages(...)` | `stream.get` | Array response format |
| `get_last_stream_message(...)` | `stream.last` | Single message response |
| `stream_version(...)` | `stream.version` | Returns number or null |
| `get_category_messages(...)` | `category.get` | Includes stream name |
| `hash_64(...)` | (internal) | Same algorithm |
| `category(...)` | (internal) | Same extraction logic |
| `id(...)` | (internal) | Same extraction logic |
| `cardinal_id(...)` | (internal) | Same extraction logic |

### Message Format

**Original (PostgreSQL row):**
```sql
SELECT * FROM messages WHERE stream_name = 'account-123';
-- id, stream_name, type, position, global_position, data, metadata, time
```

**MessageDB Go (RPC response):**
```json
[
  ["uuid", "EventType", 0, 1001, {"field": "value"}, null, "2024-01-15T10:30:00Z"]
]
```

**Category query includes stream name:**
```json
[
  ["uuid", "account-123", "EventType", 0, 1001, {"field": "value"}, null, "2024-01-15T10:30:00Z"]
]
```

## Migration Steps

### Step 1: Set Up MessageDB Go Server

```bash
# Build the server
cd golang
go build -o messagedb ./cmd/messagedb

# Start in test mode for validation
./messagedb serve --test-mode --port=8080
```

### Step 2: Update Client Code

#### Ruby (Eventide)

**Before:**
```ruby
# Direct PostgreSQL access
MessageStore::Postgres::Write.(message, stream_name)

messages = MessageStore::Postgres::Get.(stream_name)
```

**After:**
```ruby
# HTTP client
require 'net/http'
require 'json'

class MessageDBClient
  def initialize(url, token)
    @url = URI(url)
    @token = token
  end
  
  def write(stream_name, message)
    rpc('stream.write', stream_name, {
      type: message.type,
      data: message.data,
      metadata: message.metadata
    })
  end
  
  def get_stream(stream_name, opts = {})
    rpc('stream.get', stream_name, opts)
  end
  
  private
  
  def rpc(method, *args)
    http = Net::HTTP.new(@url.host, @url.port)
    request = Net::HTTP::Post.new('/rpc')
    request['Content-Type'] = 'application/json'
    request['Authorization'] = "Bearer #{@token}"
    request.body = JSON.generate([method, *args])
    
    response = http.request(request)
    JSON.parse(response.body)
  end
end

client = MessageDBClient.new('http://localhost:8080', 'ns_...')
client.write('account-123', message)
```

#### Node.js

**Before:**
```javascript
// message-db npm package
const { createWriter, createReader } = require('@eventide/message-db');

const writer = createWriter({ connectionString });
await writer.write('account-123', message);

const reader = createReader({ connectionString });
const messages = await reader.read('account-123');
```

**After:**
```typescript
// MessageDB Go client
import { MessageDBClient } from './client';

const client = new MessageDBClient('http://localhost:8080', { token: 'ns_...' });

await client.writeMessage('account-123', {
  type: 'Deposited',
  data: { amount: 100 }
});

const messages = await client.getStream('account-123');
```

### Step 3: Migrate Data (Optional)

If you have existing data in PostgreSQL Message DB:

```bash
# Export messages from PostgreSQL
psql -d message_store -c "
  COPY (
    SELECT id, stream_name, type, position, global_position, 
           data::text, metadata::text, time
    FROM messages 
    ORDER BY global_position
  ) TO '/tmp/messages.csv' WITH CSV HEADER;
"

# Import script (example)
import csv
import requests

with open('/tmp/messages.csv') as f:
    reader = csv.DictReader(f)
    for row in reader:
        requests.post('http://localhost:8080/rpc',
            headers={
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ns_...'
            },
            json=['stream.write', row['stream_name'], {
                'type': row['type'],
                'data': json.loads(row['data']),
                'metadata': json.loads(row['metadata']) if row['metadata'] else None
            }, {
                'id': row['id']
            }]
        )
```

**Note:** Global positions will be different after import. If you need exact global position matching, contact the MessageDB team.

### Step 4: Update Subscriptions

**Before (PostgreSQL LISTEN/NOTIFY):**
```ruby
# PostgreSQL notification
connection.exec("LISTEN messages")
connection.wait_for_notify do |channel, pid, payload|
  # Handle notification
end
```

**After (SSE):**
```javascript
const eventSource = new EventSource(
  `http://localhost:8080/subscribe?category=account&token=${token}`
);

eventSource.addEventListener('poke', (event) => {
  const { stream, position, globalPosition } = JSON.parse(event.data);
  // Fetch and process new messages
});
```

### Step 5: Update Consumer Groups

Consumer group semantics are preserved:

```javascript
// Consumer 0 of 4
const messages = await client.getCategory('account', {
  consumerGroup: { member: 0, size: 4 }
});

// Consumer 1 of 4
const messages = await client.getCategory('account', {
  consumerGroup: { member: 1, size: 4 }
});
```

The same hash function ensures identical stream assignment.

## Compatibility Notes

### Hash Function

MessageDB Go uses the same `hash_64` algorithm as PostgreSQL Message DB:
- FNV-1a 64-bit hash
- Same cardinal ID extraction
- Deterministic consumer group assignment

**Verification:**
```sql
-- PostgreSQL
SELECT hash_64('account-123');
-- Returns: 1234567890

-- MessageDB Go (via test)
// Same result: 1234567890
```

### Stream Names

Same naming conventions:
- `category-id` format
- Compound IDs: `category-cardinal+qualifier`
- Categories extracted by splitting on first `-`

### Optimistic Locking

Same semantics with `expectedVersion`:

```javascript
// Write at expected version
await client.writeMessage('account-123', message, { expectedVersion: 5 });

// STREAM_VERSION_CONFLICT error if version doesn't match
```

### Time Format

Both use ISO 8601 UTC format:
```
2024-01-15T10:30:00.123456789Z
```

## Breaking Changes

### 1. Response Format

Messages are returned as arrays, not objects:

```javascript
// Response
[
  ["id", "type", position, globalPosition, data, metadata, "time"]
]

// Access fields by index
const type = message[1];
const data = message[4];
```

### 2. Category Response

Category queries include stream name at index 1:

```javascript
// Response
[
  ["id", "streamName", "type", position, globalPosition, data, metadata, "time"]
]
```

### 3. Authentication

Token-based instead of PostgreSQL roles:

```http
Authorization: Bearer ns_ZGVmYXVsdA_a1b2c3d4...
```

### 4. Multi-tenancy

Namespaces instead of separate databases:

```javascript
// Create tenant namespace
await client.rpc('ns.create', 'tenant-a');

// Use tenant token for all operations
const tenantClient = new MessageDBClient(url, { token: tenantToken });
```

## Rollback Plan

If you need to rollback to PostgreSQL Message DB:

1. Keep PostgreSQL running during migration
2. Write to both systems during transition
3. Verify data consistency
4. Switch clients back to PostgreSQL if needed

## Support

For migration assistance:
- Open an issue on GitHub
- Check the [API documentation](./API.md)
- Review [examples](./examples/)

## Checklist

- [ ] MessageDB Go server deployed and accessible
- [ ] Authentication tokens generated for all namespaces
- [ ] Client code updated to use HTTP API
- [ ] Subscriptions migrated to SSE
- [ ] Consumer groups verified with same assignments
- [ ] Data migrated (if needed)
- [ ] Integration tests passing
- [ ] Performance validated
- [ ] Rollback plan documented
