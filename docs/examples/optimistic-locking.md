# Optimistic Locking Example

Optimistic locking prevents concurrent write conflicts by verifying the expected stream version before writing.

## Concept

```
Client A                    Server                    Client B
   |                          |                          |
   |-- Read (version=2) ----->|                          |
   |                          |<---- Read (version=2) ---|
   |                          |                          |
   |-- Write (expect=2) ----->|                          |
   |<---- Success (v=3) ------|                          |
   |                          |<---- Write (expect=2) ---|
   |                          |---- CONFLICT ERROR ----->|
   |                          |                          |
   |                          |<---- Read (version=3) ---|
   |                          |<---- Write (expect=3) ---|
   |                          |---- Success (v=4) ------>|
```

## Basic Usage

### Write with Expected Version

```bash
# First, get current version
VERSION=$(curl -s -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '["stream.version", "account-123"]')

# If stream doesn't exist, version is null
if [ "$VERSION" = "null" ]; then
  VERSION=-1
fi

# Write with expected version
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "[
    \"stream.write\",
    \"account-123\",
    {\"type\": \"Deposited\", \"data\": {\"amount\": 100}},
    {\"expectedVersion\": $VERSION}
  ]"
```

### TypeScript Implementation

```typescript
import { EventoDBClient } from './lib/client';

const client = new EventoDBClient('http://localhost:8080', {
  token: process.env.TOKEN
});

async function depositWithLocking(accountId: string, amount: number) {
  const streamName = `account-${accountId}`;
  
  // Get current version (-1 if stream doesn't exist)
  const version = await client.getStreamVersion(streamName) ?? -1;
  
  // Write with optimistic locking
  return client.writeMessage(streamName, {
    type: 'Deposited',
    data: { amount }
  }, { expectedVersion: version });
}

// Usage
await depositWithLocking('123', 100);
```

## Handling Conflicts

### Conflict Error Response

When a conflict occurs, you receive:

```json
{
  "error": {
    "code": "STREAM_VERSION_CONFLICT",
    "message": "Expected version 2, stream is at version 3",
    "details": {
      "expected": 2,
      "actual": 3
    }
  }
}
```

### Retry with Backoff

```typescript
async function writeWithRetry(
  client: EventoDBClient,
  streamName: string,
  message: any,
  maxRetries: number = 3
) {
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      // Get current version
      const version = await client.getStreamVersion(streamName) ?? -1;
      
      // Attempt write
      return await client.writeMessage(streamName, message, {
        expectedVersion: version
      });
    } catch (error) {
      if (error.message !== 'STREAM_VERSION_CONFLICT') {
        throw error; // Rethrow non-conflict errors
      }
      
      if (attempt === maxRetries - 1) {
        throw new Error(`Failed after ${maxRetries} retries: ${error.message}`);
      }
      
      // Exponential backoff: 10ms, 20ms, 40ms
      const delay = Math.pow(2, attempt) * 10;
      console.log(`Conflict detected, retrying in ${delay}ms...`);
      await new Promise(resolve => setTimeout(resolve, delay));
    }
  }
}

// Usage
await writeWithRetry(client, 'account-123', {
  type: 'Deposited',
  data: { amount: 100 }
});
```

## Real-World Example: Bank Account

### Account Aggregate

```typescript
interface AccountState {
  accountId: string;
  balance: number;
  version: number;
}

async function loadAccount(client: EventoDBClient, accountId: string): Promise<AccountState> {
  const streamName = `account-${accountId}`;
  const messages = await client.getStream(streamName);
  
  let state: AccountState = {
    accountId,
    balance: 0,
    version: -1
  };
  
  for (const msg of messages) {
    const [id, type, position, globalPosition, data] = msg;
    state.version = position;
    
    switch (type) {
      case 'AccountOpened':
        state.balance = data.initialBalance || 0;
        break;
      case 'Deposited':
        state.balance += data.amount;
        break;
      case 'Withdrawn':
        state.balance -= data.amount;
        break;
    }
  }
  
  return state;
}

async function deposit(client: EventoDBClient, accountId: string, amount: number) {
  const streamName = `account-${accountId}`;
  
  // Load current state
  const account = await loadAccount(client, accountId);
  
  // Business logic
  if (amount <= 0) {
    throw new Error('Deposit amount must be positive');
  }
  
  // Write with optimistic locking
  return client.writeMessage(streamName, {
    type: 'Deposited',
    data: { amount }
  }, { expectedVersion: account.version });
}

async function withdraw(client: EventoDBClient, accountId: string, amount: number) {
  const streamName = `account-${accountId}`;
  
  // Load current state
  const account = await loadAccount(client, accountId);
  
  // Business logic
  if (amount <= 0) {
    throw new Error('Withdrawal amount must be positive');
  }
  if (account.balance < amount) {
    throw new Error('Insufficient funds');
  }
  
  // Write with optimistic locking
  return client.writeMessage(streamName, {
    type: 'Withdrawn',
    data: { amount }
  }, { expectedVersion: account.version });
}
```

### Using the Aggregate

```typescript
const client = new EventoDBClient('http://localhost:8080', { token });

// Open account
await client.writeMessage('account-123', {
  type: 'AccountOpened',
  data: { accountId: '123', initialBalance: 100 }
});

// Deposit
await deposit(client, '123', 50);

// Withdraw (will fail if concurrent modification)
try {
  await withdraw(client, '123', 30);
} catch (error) {
  if (error.message === 'STREAM_VERSION_CONFLICT') {
    console.log('Account was modified, please retry');
  } else {
    throw error;
  }
}

// Check balance
const account = await loadAccount(client, '123');
console.log(`Balance: $${account.balance}`); // $120
```

## Concurrent Simulation

### Test Concurrent Writes

```typescript
async function testConcurrentDeposits(client: EventoDBClient) {
  // Initialize account
  await client.writeMessage('concurrent-test', {
    type: 'AccountOpened',
    data: { balance: 0 }
  });
  
  // Simulate concurrent deposits
  const deposit1 = deposit(client, 'concurrent-test', 100);
  const deposit2 = deposit(client, 'concurrent-test', 100);
  
  // One will succeed, one will fail with STREAM_VERSION_CONFLICT
  const results = await Promise.allSettled([deposit1, deposit2]);
  
  const successes = results.filter(r => r.status === 'fulfilled');
  const failures = results.filter(r => r.status === 'rejected');
  
  console.log(`Successes: ${successes.length}`); // 1
  console.log(`Failures: ${failures.length}`);   // 1
  
  // Verify balance
  const account = await loadAccount(client, 'concurrent-test');
  console.log(`Final balance: $${account.balance}`); // $100 (only one deposit succeeded)
}
```

## When to Use Optimistic Locking

### Use It For:

- **Aggregates**: Entities with business rules (accounts, orders, etc.)
- **Sequential processing**: Where order matters
- **Conflict detection**: When you need to know about concurrent modifications

### Don't Use It For:

- **Event logs**: Append-only streams without business logic
- **Activity streams**: Where order doesn't matter
- **High-contention scenarios**: Consider partitioning instead

### Alternative Patterns

**No Locking (append-only):**
```typescript
// No expectedVersion - always appends
await client.writeMessage('events-123', {
  type: 'UserLoggedIn',
  data: { userId: '123', timestamp: Date.now() }
});
```

**Partitioned Streams:**
```typescript
// Partition by user to reduce contention
const partition = userId.charCodeAt(0) % 4;
await client.writeMessage(`events-partition-${partition}`, event);
```

## Best Practices

1. **Load-then-write pattern**: Always load current state before writing
2. **Retry with backoff**: Don't hammer the server on conflicts
3. **Limited retries**: Fail after 3-5 attempts to avoid infinite loops
4. **Include version in response**: Return new version for subsequent writes
5. **Log conflicts**: Monitor conflict rate for hotspot detection

```typescript
// Complete pattern
async function safeWrite(
  client: EventoDBClient,
  streamName: string,
  buildMessage: (state: any) => any,
  maxRetries: number = 3
) {
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    // Load current state
    const messages = await client.getStream(streamName);
    const version = messages.length > 0 ? messages[messages.length - 1][2] : -1;
    
    // Build message based on current state
    const message = buildMessage(messages);
    
    try {
      const result = await client.writeMessage(streamName, message, {
        expectedVersion: version
      });
      
      return { ...result, version: version + 1 };
    } catch (error) {
      if (error.message !== 'STREAM_VERSION_CONFLICT') {
        throw error;
      }
      
      if (attempt === maxRetries - 1) {
        throw new Error(`Max retries exceeded for ${streamName}`);
      }
      
      // Backoff
      await new Promise(r => setTimeout(r, Math.pow(2, attempt) * 10));
    }
  }
}
```

---

## Next Steps

- [Consumer Groups](./consumer-groups.md) - Scale message processing
- [Subscriptions](./subscriptions.md) - Real-time notifications
