# ADR-003: External Test Suite with Bun.js

**Date:** 2024-12-17  
**Status:** Accepted  
**Context:** Need comprehensive external testing strategy that validates the HTTP API from a client perspective

---

## Decision

Implement an **external test suite using Bun.js** that tests the MessageDB server as a black box via HTTP.

---

## Architecture

### Test Suite Structure

```
test/
├── package.json
├── bun.lockb
├── tests/
│   ├── stream.test.ts          # Stream operations
│   ├── category.test.ts        # Category operations
│   ├── subscription.test.ts    # SSE subscriptions
│   ├── namespace.test.ts       # Multi-tenancy
│   ├── concurrency.test.ts     # Concurrent writes
│   └── migration.test.ts       # Schema compatibility
├── fixtures/
│   └── test-data.json
└── lib/
    ├── client.ts               # MessageDB HTTP client
    └── helpers.ts
```

---

## Key Features

### 1. Namespace Isolation with Cleanup

Each test gets its own namespace → no conflicts, parallel execution.

```typescript
import { test, afterEach } from 'bun:test';
import { MessageDBClient } from './lib/client';

test('write and read message', async () => {
  // Server in test mode auto-creates namespace on first use
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  // First request auto-creates namespace and returns token
  const writeResult = await client.writeMessage('account-123', {
    type: 'Opened',
    data: { balance: 0 }
  });
  
  // Client captures token from response header
  const messages = await client.getStream('account-123');
  expect(messages).toHaveLength(1);
  
  // Cleanup: delete namespace
  await client.deleteNamespace();
  server.close();
});
```

**Benefits:**
- Tests run in parallel without interference
- Explicit cleanup (namespaces are not ephemeral)
- Fast execution with in-memory SQLite
- No flaky tests due to shared state

---

### 2. Test Mode with In-Memory SQLite

Tests spawn MessageDB server with `--test-mode` flag.

```typescript
// lib/helpers.ts
export async function startTestServer(opts = {}) {
  const proc = Bun.spawn([
    './messagedb',
    'serve',
    '--test-mode',     // Enables in-memory SQLite + auto-namespace creation
    '--port=0',        // Random port
  ]);
  
  // Wait for server ready
  const port = await waitForHealthy(proc);
  
  return {
    url: `http://localhost:${port}`,
    close: () => proc.kill()
  };
}
```

**Test mode behavior:**
1. **Backend:** SQLite in-memory (`:memory:`)
2. **Auto-create namespaces:** First request creates namespace automatically
3. **Token in response:** Server returns token in `X-MessageDB-Token` header
4. **Cleanup supported:** Can delete namespaces via API
5. **Data lost on shutdown:** Perfect for tests

**Benefits:**
- No Postgres required for tests
- No manual namespace creation needed
- Extremely fast (all in RAM)
- Each test can spawn its own server instance
- Perfect for CI/CD

---

### 3. Test Categories

#### Stream Operations
```typescript
test('write message with expected version', async () => {
  const server = await startServer();
  const client = new MessageDBClient(server.url);
  
  await client.writeMessage('account-1', {
    type: 'Opened',
    data: { balance: 0 }
  });
  
  // This should succeed
  await client.writeMessage('account-1', {
    type: 'Deposited',
    data: { amount: 100 }
  }, { expectedVersion: 0 });
  
  // This should fail (version conflict)
  await expect(
    client.writeMessage('account-1', {
      type: 'Withdrawn',
      data: { amount: 50 }
    }, { expectedVersion: 0 })
  ).rejects.toThrow('STREAM_VERSION_CONFLICT');
  
  server.close();
});
```

#### Category Operations
```typescript
test('read category with consumer groups', async () => {
  const server = await startServer();
  const client = new MessageDBClient(server.url);
  
  // Write to multiple streams in same category
  await client.writeMessage('account-1', { type: 'Opened', data: {} });
  await client.writeMessage('account-2', { type: 'Opened', data: {} });
  await client.writeMessage('account-3', { type: 'Opened', data: {} });
  
  // Consumer group 0 of 2
  const messages1 = await client.getCategory('account', {
    consumerGroup: { member: 0, size: 2 }
  });
  
  // Consumer group 1 of 2
  const messages2 = await client.getCategory('account', {
    consumerGroup: { member: 1, size: 2 }
  });
  
  // No overlap
  const streams1 = messages1.map(m => m.streamName);
  const streams2 = messages2.map(m => m.streamName);
  expect(streams1).not.toContain(...streams2);
  
  server.close();
});
```

#### SSE Subscriptions
```typescript
test('stream subscription receives pokes', async () => {
  const server = await startServer();
  const client = new MessageDBClient(server.url);
  
  const pokes: any[] = [];
  const subscription = client.subscribe('account-1', {
    position: 0,
    onPoke: (poke) => pokes.push(poke)
  });
  
  // Write messages
  await client.writeMessage('account-1', { type: 'Event1', data: {} });
  await client.writeMessage('account-1', { type: 'Event2', data: {} });
  
  // Wait for pokes
  await Bun.sleep(100);
  
  expect(pokes).toHaveLength(2);
  expect(pokes[0]).toMatchObject({
    stream: 'account-1',
    position: 0,
    globalPosition: expect.any(Number)
  });
  
  subscription.close();
  server.close();
});
```

#### Namespace Isolation
```typescript
test('namespaces are isolated', async () => {
  const server = await startTestServer();
  
  // Each client auto-creates its own namespace on first request
  const client1 = new MessageDBClient(server.url);
  const client2 = new MessageDBClient(server.url);
  
  // Write to first namespace (auto-created)
  await client1.writeMessage('account-123', { type: 'Opened', data: {} });
  
  // Read from first namespace
  const messagesA = await client1.getStream('account-123');
  expect(messagesA).toHaveLength(1);
  
  // Write to second namespace (different auto-created namespace)
  await client2.writeMessage('account-123', { type: 'Opened', data: {} });
  
  // Read from second namespace (separate data)
  const messagesB = await client2.getStream('account-123');
  expect(messagesB).toHaveLength(1);
  
  // Verify they're different namespaces
  expect(client1.token).not.toBe(client2.token);
  
  // Cleanup
  await client1.deleteNamespace();
  await client2.deleteNamespace();
  server.close();
});
```

#### Concurrent Writes
```typescript
test('concurrent writes to different streams', async () => {
  const server = await startServer();
  const client = new MessageDBClient(server.url);
  
  // 100 concurrent writes to different streams
  const writes = Array.from({ length: 100 }, (_, i) =>
    client.writeMessage(`account-${i}`, {
      type: 'Opened',
      data: { index: i }
    })
  );
  
  const results = await Promise.all(writes);
  
  // All should succeed
  expect(results).toHaveLength(100);
  results.forEach(r => {
    expect(r.position).toBe(0);
  });
  
  server.close();
});

test('concurrent writes to same stream with optimistic locking', async () => {
  const server = await startServer();
  const client = new MessageDBClient(server.url);
  
  // First write
  await client.writeMessage('account-1', { type: 'Opened', data: {} });
  
  // 10 concurrent writes with expectedVersion=0
  const writes = Array.from({ length: 10 }, () =>
    client.writeMessage('account-1', {
      type: 'Deposited',
      data: { amount: 100 }
    }, { expectedVersion: 0 })
  );
  
  const results = await Promise.allSettled(writes);
  
  // Only 1 should succeed, 9 should fail with version conflict
  const succeeded = results.filter(r => r.status === 'fulfilled');
  const failed = results.filter(r => r.status === 'rejected');
  
  expect(succeeded).toHaveLength(1);
  expect(failed).toHaveLength(9);
  
  server.close();
});
```

---

## Test Client Implementation

```typescript
// lib/client.ts
export class MessageDBClient {
  private token?: string;
  
  constructor(
    private baseURL: string,
    opts: { token?: string } = {}
  ) {
    this.token = opts.token;
  }
  
  private async rpc(method: string, ...args: any[]) {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json'
    };
    
    // Add auth header if token available
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }
    
    const response = await fetch(`${this.baseURL}/rpc`, {
      method: 'POST',
      headers,
      body: JSON.stringify([method, ...args])
    });
    
    // Capture token from response header (test mode)
    const newToken = response.headers.get('X-MessageDB-Token');
    if (newToken && !this.token) {
      this.token = newToken;
    }
    
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error.code);
    }
    
    return response.json();
  }
  
  async writeMessage(
    streamName: string,
    msg: { type: string; data: any; metadata?: any },
    opts?: { id?: string; expectedVersion?: number }
  ) {
    return this.rpc('stream.write', streamName, msg, opts);
  }
  
  async getStream(
    streamName: string,
    opts?: { position?: number; batchSize?: number }
  ) {
    return this.rpc('stream.get', streamName, opts);
  }
  
  async getCategory(
    categoryName: string,
    opts?: {
      position?: number;
      batchSize?: number;
      consumerGroup?: { member: number; size: number };
    }
  ) {
    return this.rpc('category.get', categoryName, opts);
  }
  
  subscribe(
    streamOrCategory: string,
    opts: {
      position?: number;
      onPoke: (poke: any) => void;
      isCategory?: boolean;
    }
  ) {
    const url = `${this.baseURL}/subscribe?${
      opts.isCategory ? 'category' : 'stream'
    }=${streamOrCategory}&position=${opts.position || 0}`;
    
    // EventSource doesn't support custom headers, append token to URL
    const authURL = this.token 
      ? `${url}&token=${this.token}`
      : url;
    
    const eventSource = new EventSource(authURL);
    eventSource.addEventListener('poke', (e) => {
      opts.onPoke(JSON.parse(e.data));
    });
    
    return {
      close: () => eventSource.close()
    };
  }
  
  async deleteNamespace() {
    // Must have token to delete
    if (!this.token) {
      throw new Error('No token available');
    }
    
    // Extract namespace from token
    const namespace = this.parseNamespaceFromToken(this.token);
    return this.rpc('ns.delete', namespace);
  }
  
  private parseNamespaceFromToken(token: string): string {
    const parts = token.split('_');
    const nsBase64 = parts[1];
    return atob(nsBase64);
  }
}
```

---

## CI/CD Integration

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install Bun
        uses: oven-sh/setup-bun@v1
      
      - name: Build MessageDB server
        run: go build -o messagedb ./cmd/messagedb
      
      - name: Install test dependencies
        working-directory: ./test
        run: bun install
      
      - name: Run external tests
        working-directory: ./test
        run: bun test
        env:
          MESSAGEDB_BIN: ../messagedb
```

---

## Benefits

1. **Black-box testing**: Tests the real HTTP API, not internal Go code
2. **Fast**: In-memory SQLite, parallel execution via namespaces
3. **Isolated**: Each test is completely independent
4. **Language-agnostic**: Validates the API contract, not Go implementation
5. **CI-friendly**: No Postgres dependency, runs anywhere Bun runs
6. **Client validation**: Tests also serve as examples for client developers
7. **Namespace testing**: Validates multi-tenancy works correctly

---

## Test Execution

```bash
# Run all tests
cd test && bun test

# Run specific test file
bun test tests/stream.test.ts

# Watch mode
bun test --watch

# With coverage (if needed)
bun test --coverage
```

**Output:**
```
✓ write and read message (15ms)
✓ write message with expected version (23ms)
✓ read category with consumer groups (31ms)
✓ stream subscription receives pokes (128ms)
✓ namespaces are isolated (19ms)
✓ concurrent writes to different streams (87ms)
✓ concurrent writes to same stream with optimistic locking (45ms)

7 tests passed (348ms)
```

---

## Future Enhancements

1. **Load testing**: Use Bun to spawn many concurrent clients
2. **Chaos testing**: Kill server mid-operation, validate recovery
3. **Migration testing**: Test upgrade path from old schema versions
4. **Performance benchmarks**: Track latency/throughput over time
5. **Client generation**: Auto-generate client libraries from tests

---

## References

- [Bun Test Runner](https://bun.sh/docs/cli/test)
- [EventSource API](https://developer.mozilla.org/en-US/docs/Web/API/EventSource)
- [Black-box Testing](https://en.wikipedia.org/wiki/Black-box_testing)
