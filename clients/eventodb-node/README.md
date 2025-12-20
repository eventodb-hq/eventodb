# @eventodb/client

Node.js/TypeScript client for EventoDB - a simple, fast message store.

## Installation

```bash
npm install @eventodb/client
```

## Requirements

- Node.js 18+ (uses native fetch API)
- TypeScript 5+ (for TypeScript usage)

## Usage

### Basic Usage

```typescript
import { EventoDBClient } from '@eventodb/client';

// Create client
const client = new EventoDBClient('http://localhost:8080', {
  token: 'ns_...'
});

// Write message
const result = await client.streamWrite('account-123', {
  type: 'Deposited',
  data: { amount: 100 }
});

console.log(`Written at position ${result.position}`);

// Read stream
const messages = await client.streamGet('account-123');
for (const [id, type, pos, gpos, data, metadata, time] of messages) {
  console.log(`${type} at ${pos}:`, data);
}
```

### With Optimistic Locking

```typescript
// Read current version
const version = await client.streamVersion('account-123');

// Write with expected version
await client.streamWrite('account-123', {
  type: 'Withdrawn',
  data: { amount: 50 }
}, { expectedVersion: version });
```

### Category Reading

```typescript
// Read all streams in category
const messages = await client.categoryGet('account');

// With consumer group (for load balancing)
const messages = await client.categoryGet('account', {
  consumerGroup: {
    member: 0,  // This consumer's ID
    size: 4     // Total consumers
  }
});
```

### Namespace Management

```typescript
// Create namespace
const ns = await client.namespaceCreate('my-app', {
  description: 'My application namespace'
});

console.log(`Token: ${ns.token}`);

// Use namespace token
const nsClient = new EventoDBClient('http://localhost:8080', {
  token: ns.token
});
```

## API Reference

### Stream Operations

- `streamWrite(streamName, message, options?)` - Write a message
  - Returns: `{ position: number, globalPosition: number }`
  - Options: `{ id?: string, expectedVersion?: number }`

- `streamGet(streamName, options?)` - Read messages
  - Returns: Array of `[id, type, position, globalPosition, data, metadata, time]`
  - Options: `{ position?: number, globalPosition?: number, batchSize?: number }`

- `streamLast(streamName, options?)` - Get last message
  - Returns: Message array or `null`
  - Options: `{ type?: string }`

- `streamVersion(streamName)` - Get stream version
  - Returns: Last position (number) or `null` if stream doesn't exist

### Category Operations

- `categoryGet(categoryName, options?)` - Read from category
  - Returns: Array of `[id, streamName, type, position, globalPosition, data, metadata, time]`
  - Options: 
    ```typescript
    {
      position?: number;
      globalPosition?: number;
      batchSize?: number;
      correlation?: string;
      consumerGroup?: { member: number; size: number };
    }
    ```

### Namespace Operations

- `namespaceCreate(id, options?)` - Create namespace
  - Returns: `{ namespace: string, token: string, createdAt: string }`
  - Options: `{ token?: string, description?: string, metadata?: object }`

- `namespaceDelete(id)` - Delete namespace
  - Returns: `{ namespace: string, deletedAt: string, messagesDeleted: number }`

- `namespaceList()` - List namespaces
  - Returns: Array of namespace info objects

- `namespaceInfo(id)` - Get namespace info
  - Returns: `{ namespace, description, createdAt, messageCount, streamCount, lastActivity }`

### System Operations

- `systemVersion()` - Get server version
  - Returns: Version string (e.g., "1.3.0")

- `systemHealth()` - Get server health
  - Returns: `{ status: string }`

### Utility

- `getToken()` - Get current authentication token
  - Returns: Token string or `undefined`

## Error Handling

```typescript
import { EventoDBError, NetworkError } from '@eventodb/client';

try {
  await client.streamWrite(stream, message, { expectedVersion: 5 });
} catch (error) {
  if (error instanceof EventoDBError) {
    console.log(`Error code: ${error.code}`);
    console.log(`Message: ${error.message}`);
    console.log(`Details:`, error.details);
  } else if (error instanceof NetworkError) {
    console.log('Network error:', error.message);
  }
}
```

## TypeScript Support

Full TypeScript definitions are included. Import types as needed:

```typescript
import type { 
  Message, 
  WriteResult, 
  StreamMessage,
  CategoryMessage 
} from '@eventodb/client';

const message: Message = {
  type: 'UserRegistered',
  data: { userId: '123', email: 'user@example.com' },
  metadata: { correlationId: 'abc' }
};
```

## Server-Sent Events (SSE)

The SDK provides SSE subscription methods but requires an EventSource polyfill for Node.js:

```typescript
// Install EventSource polyfill
// npm install eventsource @types/eventsource

// Note: SSE methods are available but will emit an error in Node.js
// indicating EventSource polyfill is needed
const subscription = client.streamSubscribe('account-123');

subscription.on('poke', (poke) => {
  console.log(`New message at position ${poke.position}`);
});

subscription.on('error', (error) => {
  console.error('Subscription error:', error);
});

// Clean up
subscription.close();
```

For full SSE support, install the `eventsource` package and implement custom connection logic. See `TEST_COVERAGE.md` for details.

## Testing

**Complete Test Coverage**: 79 tests across all SDK-TEST-SPEC categories

- ✅ WRITE (9 tests)
- ✅ READ (10 tests)
- ✅ LAST (4 tests)
- ✅ VERSION (3 tests)
- ✅ CATEGORY (8 tests)
- ✅ NAMESPACE (8 tests)
- ✅ SYSTEM (2 tests)
- ✅ AUTH (4 tests)
- ✅ ERROR (3 tests)
- ✅ ENCODING (10 tests)
- ✅ EDGE (8 tests)
- ⏭️ SSE (10 tests - 9 require EventSource polyfill)

**Results**: 67 passing, 12 skipped (with documented reasons)

See `TEST_COVERAGE.md` for detailed breakdown.

### Running Tests

```bash
# Run all tests
npm test

# With custom URL
EVENTODB_URL=http://localhost:8080 npm test

# With admin token for namespace tests
EVENTODB_URL=http://localhost:8080 EVENTODB_ADMIN_TOKEN=admin_token npm test

# Via official test runner
cd ../..
./bin/run_sdk_tests.sh js
```

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Run tests
npm test

# Watch mode
npm run test:watch
```

## License

MIT
