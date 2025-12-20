# Basic Usage Examples

This guide demonstrates basic EventoDB operations with curl and TypeScript examples.

## Setup

### Start the Server

```bash
# Build and run
cd golang
go build -o eventodb ./cmd/eventodb
./eventodb serve --test-mode --port=8080
```

Save the token printed at startup:

```
═══════════════════════════════════════════════════════════════
DEFAULT NAMESPACE TOKEN:
ns_ZGVmYXVsdA_a1b2c3d4e5f6g7h8i9j0k1l2
═══════════════════════════════════════════════════════════════
```

Set it as an environment variable:

```bash
export TOKEN="ns_ZGVmYXVsdA_a1b2c3d4e5f6g7h8i9j0k1l2"
```

---

## Writing Messages

### Simple Write

Write a message to a stream:

**curl:**
```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '[
    "stream.write",
    "account-123",
    {
      "type": "AccountOpened",
      "data": {
        "accountId": "123",
        "ownerName": "John Doe",
        "initialBalance": 0
      }
    }
  ]'
```

**Response:**
```json
{"position": 0, "globalPosition": 1}
```

**TypeScript:**
```typescript
import { EventoDBClient } from './lib/client';

const client = new EventoDBClient('http://localhost:8080', {
  token: process.env.TOKEN
});

const result = await client.writeMessage('account-123', {
  type: 'AccountOpened',
  data: {
    accountId: '123',
    ownerName: 'John Doe',
    initialBalance: 0
  }
});

console.log(`Written at position ${result.position}`);
```

### Write with Custom ID

Provide your own message UUID:

```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '[
    "stream.write",
    "account-123",
    {
      "type": "Deposited",
      "data": { "amount": 100 }
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440000"
    }
  ]'
```

### Write with Metadata

Add metadata for correlation and tracing:

```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '[
    "stream.write",
    "account-123",
    {
      "type": "Deposited",
      "data": { "amount": 100 },
      "metadata": {
        "correlationStreamName": "deposit-workflow-456",
        "causationMessageId": "previous-message-uuid",
        "userId": "user-789"
      }
    }
  ]'
```

**TypeScript:**
```typescript
await client.writeMessage('account-123', {
  type: 'Deposited',
  data: { amount: 100 },
  metadata: {
    correlationStreamName: 'deposit-workflow-456',
    causationMessageId: 'previous-message-uuid',
    userId: 'user-789'
  }
});
```

---

## Reading Messages

### Read All Messages from a Stream

```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.get", "account-123"]'
```

**Response:**
```json
[
  ["uuid-1", "AccountOpened", 0, 1, {"accountId": "123", "ownerName": "John Doe", "initialBalance": 0}, null, "2024-01-15T10:30:00Z"],
  ["uuid-2", "Deposited", 1, 2, {"amount": 100}, {"correlationStreamName": "deposit-workflow-456"}, "2024-01-15T10:31:00Z"]
]
```

**TypeScript:**
```typescript
const messages = await client.getStream('account-123');

for (const msg of messages) {
  const [id, type, position, globalPosition, data, metadata, time] = msg;
  console.log(`${type} at position ${position}: ${JSON.stringify(data)}`);
}
```

### Read with Options

```bash
# Start from position 5
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.get", "account-123", {"position": 5}]'

# Limit to 10 messages
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.get", "account-123", {"batchSize": 10}]'

# Both options
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.get", "account-123", {"position": 5, "batchSize": 10}]'
```

**TypeScript:**
```typescript
const messages = await client.getStream('account-123', {
  position: 5,
  batchSize: 10
});
```

### Get Last Message

```bash
# Get last message of any type
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.last", "account-123"]'

# Get last message of specific type
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.last", "account-123", {"type": "Deposited"}]'
```

**TypeScript:**
```typescript
const last = await client.getLastMessage('account-123');
const lastDeposit = await client.getLastMessage('account-123', { type: 'Deposited' });
```

### Get Stream Version

```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.version", "account-123"]'
```

**Response:**
```json
1
```

**TypeScript:**
```typescript
const version = await client.getStreamVersion('account-123');
console.log(`Stream has ${version + 1} messages`); // version is 0-based
```

---

## Reading from Categories

### Get All Messages in a Category

```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["category.get", "account"]'
```

**Response (includes stream name):**
```json
[
  ["uuid-1", "account-123", "AccountOpened", 0, 1, {"accountId": "123"}, null, "2024-01-15T10:30:00Z"],
  ["uuid-2", "account-456", "AccountOpened", 0, 2, {"accountId": "456"}, null, "2024-01-15T10:30:01Z"],
  ["uuid-3", "account-123", "Deposited", 1, 3, {"amount": 100}, null, "2024-01-15T10:31:00Z"]
]
```

**TypeScript:**
```typescript
const messages = await client.getCategory('account');

for (const msg of messages) {
  const [id, streamName, type, position, globalPosition, data, metadata, time] = msg;
  console.log(`${streamName}: ${type} at global position ${globalPosition}`);
}
```

### Category with Options

```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '[
    "category.get",
    "account",
    {
      "globalPosition": 100,
      "batchSize": 50
    }
  ]'
```

**TypeScript:**
```typescript
const messages = await client.getCategory('account', {
  globalPosition: 100,
  batchSize: 50
});
```

---

## Error Handling

### Handle Errors in TypeScript

```typescript
try {
  await client.writeMessage('account-123', {
    type: 'Deposited',
    data: { amount: 100 }
  }, { expectedVersion: 99 }); // Wrong version
} catch (error) {
  if (error.message === 'STREAM_VERSION_CONFLICT') {
    console.log('Version conflict - retry with correct version');
  } else if (error.message === 'AUTH_REQUIRED') {
    console.log('Authentication required');
  } else {
    console.log('Unexpected error:', error.message);
  }
}
```

### Handle Errors with curl

```bash
# Check HTTP status code
response=$(curl -s -w "\n%{http_code}" -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.write", "account-123", {"type": "Test", "data": {}}, {"expectedVersion": 99}]')

http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$http_code" -ne 200 ]; then
  echo "Error ($http_code): $body"
fi
```

---

## System Operations

### Check Server Health

```bash
curl http://localhost:8080/health
```

**Response:**
```json
{"status":"ok"}
```

### Get Server Version

```bash
curl http://localhost:8080/version
```

**Response:**
```json
{"version":"1.3.0"}
```

### Get Version via RPC

```bash
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["sys.version"]'
```

---

## Complete Example: Bank Account

```typescript
import { EventoDBClient } from './lib/client';

const client = new EventoDBClient('http://localhost:8080', {
  token: process.env.TOKEN
});

async function openAccount(accountId: string, ownerName: string) {
  return client.writeMessage(`account-${accountId}`, {
    type: 'AccountOpened',
    data: { accountId, ownerName, initialBalance: 0 }
  });
}

async function deposit(accountId: string, amount: number) {
  const streamName = `account-${accountId}`;
  const version = await client.getStreamVersion(streamName) ?? -1;
  
  return client.writeMessage(streamName, {
    type: 'Deposited',
    data: { amount }
  }, { expectedVersion: version });
}

async function getBalance(accountId: string): Promise<number> {
  const messages = await client.getStream(`account-${accountId}`);
  
  let balance = 0;
  for (const msg of messages) {
    const [id, type, position, globalPosition, data] = msg;
    if (type === 'AccountOpened') {
      balance = data.initialBalance;
    } else if (type === 'Deposited') {
      balance += data.amount;
    } else if (type === 'Withdrawn') {
      balance -= data.amount;
    }
  }
  
  return balance;
}

// Usage
await openAccount('123', 'John Doe');
await deposit('123', 100);
await deposit('123', 50);

const balance = await getBalance('123');
console.log(`Balance: $${balance}`); // Balance: $150
```

---

## Next Steps

- [Optimistic Locking](./optimistic-locking.md) - Prevent concurrent write conflicts
- [Consumer Groups](./consumer-groups.md) - Scale message processing
- [Subscriptions](./subscriptions.md) - Real-time notifications
