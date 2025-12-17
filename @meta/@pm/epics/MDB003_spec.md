# EPIC MDB003: Testing & Production Readiness

## Overview

**Epic ID:** MDB003
**Name:** Testing & Production Readiness
**Duration:** 1-2 weeks
**Status:** pending
**Priority:** high
**Depends On:** MDB002 (RPC API & Authentication)

**Goal:** Establish comprehensive test coverage, performance validation, and production deployment capability for the MessageDB Go server.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ External Test Suite (Bun.js)                                 │
│ - Black-box HTTP testing                                     │
│ - TypeScript test client                                     │
│ - Test scenarios for all API methods                         │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ MessageDB Server (test-mode enabled)                         │
│ - SQLite in-memory backend                                   │
│ - Auto-namespace creation                                    │
│ - All RPC methods exposed                                    │
└─────────────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Production Deployment                                        │
│ - Docker container                                           │
│ - CI/CD pipeline                                             │
│ - Performance benchmarks                                     │
│ - Documentation & examples                                   │
└─────────────────────────────────────────────────────────────┘
```

## Technical Requirements

### External Test Suite (ADR-003)

**Test Suite Structure:**
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
    └── helpers.ts              # Test utilities
```

**Test Philosophy:**
- **Black-box testing:** Test HTTP API, not internal Go code
- **Namespace isolation:** Each test gets own namespace, parallel execution
- **Fast execution:** In-memory SQLite, no Postgres dependency
- **Language-agnostic:** Validates API contract for any client

### Performance Requirements

**Target Metrics:**
- API response time < 50ms (p95)
- Stream write throughput: 1000+ writes/sec per namespace
- Category query: 100 messages in < 30ms
- SSE poke delivery: < 5ms
- Overall performance within 20% of direct Postgres access

**Benchmarks:**
- Stream write (single message)
- Stream read (100 messages)
- Category read with consumer groups
- Concurrent writes (same stream)
- Concurrent writes (different streams)
- SSE subscription latency

## Functional Requirements

### FR-1: External Test Client

**TypeScript Client Implementation:**
```typescript
// test/lib/client.ts
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
  
  async writeMessage(streamName: string, msg: any, opts?: any) {
    return this.rpc('stream.write', streamName, msg, opts);
  }
  
  async getStream(streamName: string, opts?: any) {
    return this.rpc('stream.get', streamName, opts);
  }
  
  async getCategory(categoryName: string, opts?: any) {
    return this.rpc('category.get', categoryName, opts);
  }
  
  subscribe(streamOrCategory: string, opts: any) {
    const url = `${this.baseURL}/subscribe?${
      opts.isCategory ? 'category' : 'stream'
    }=${streamOrCategory}&position=${opts.position || 0}`;
    
    const authURL = this.token ? `${url}&token=${this.token}` : url;
    const eventSource = new EventSource(authURL);
    
    eventSource.addEventListener('poke', (e) => {
      opts.onPoke(JSON.parse(e.data));
    });
    
    return {
      close: () => eventSource.close()
    };
  }
  
  async deleteNamespace() {
    if (!this.token) {
      throw new Error('No token available');
    }
    const namespace = this.parseNamespaceFromToken(this.token);
    return this.rpc('ns.delete', namespace);
  }
}
```

### FR-2: Test Scenarios

#### Stream Operations Tests
```typescript
// test/tests/stream.test.ts
import { test, expect, afterEach } from 'bun:test';
import { startTestServer, MessageDBClient } from '../lib';

test('write and read message', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  const writeResult = await client.writeMessage('account-123', {
    type: 'Opened',
    data: { balance: 0 }
  });
  
  expect(writeResult.position).toBe(0);
  
  const messages = await client.getStream('account-123');
  expect(messages).toHaveLength(1);
  expect(messages[0][1]).toBe('Opened'); // type
  
  await client.deleteNamespace();
  server.close();
});

test('optimistic locking prevents conflicts', async () => {
  const server = await startTestServer();
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
  
  await client.deleteNamespace();
  server.close();
});
```

#### Category Operations Tests
```typescript
// test/tests/category.test.ts
test('consumer groups partition streams', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  // Write to multiple streams
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
  
  // Extract stream names
  const streams1 = messages1.map(m => m[1]); // streamName at index 1
  const streams2 = messages2.map(m => m[1]);
  
  // No overlap
  expect(streams1.some(s => streams2.includes(s))).toBe(false);
  
  await client.deleteNamespace();
  server.close();
});

test('compound IDs route to same consumer', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  // Write to streams with compound IDs (same cardinal ID)
  await client.writeMessage('account-123+alice', { type: 'Opened', data: {} });
  await client.writeMessage('account-123+bob', { type: 'Opened', data: {} });
  await client.writeMessage('account-456+charlie', { type: 'Opened', data: {} });
  
  // Consumer group 0 of 2
  const messages1 = await client.getCategory('account', {
    consumerGroup: { member: 0, size: 2 }
  });
  
  // Consumer group 1 of 2
  const messages2 = await client.getCategory('account', {
    consumerGroup: { member: 1, size: 2 }
  });
  
  const streams1 = messages1.map(m => m[1]);
  const streams2 = messages2.map(m => m[1]);
  
  // Streams with cardinal ID 123 should be in same consumer group
  const has123Alice1 = streams1.includes('account-123+alice');
  const has123Bob1 = streams1.includes('account-123+bob');
  const has123Alice2 = streams2.includes('account-123+alice');
  const has123Bob2 = streams2.includes('account-123+bob');
  
  // Both alice and bob should be in same consumer (both have cardinal_id=123)
  expect(has123Alice1).toBe(has123Bob1);
  expect(has123Alice2).toBe(has123Bob2);
  
  await client.deleteNamespace();
  server.close();
});

test('correlation filtering works', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  // Write messages with correlation metadata
  await client.writeMessage('account-1', {
    type: 'Opened',
    data: {},
    metadata: { correlationStreamName: 'workflow-123' }
  });
  
  await client.writeMessage('account-2', {
    type: 'Opened',
    data: {},
    metadata: { correlationStreamName: 'workflow-456' }
  });
  
  await client.writeMessage('account-3', {
    type: 'Opened',
    data: {},
    metadata: { correlationStreamName: 'process-789' }
  });
  
  // Query with correlation filter
  const messages = await client.getCategory('account', {
    correlation: 'workflow'
  });
  
  expect(messages).toHaveLength(2);
  const streamNames = messages.map(m => m[1]);
  expect(streamNames).toContain('account-1');
  expect(streamNames).toContain('account-2');
  expect(streamNames).not.toContain('account-3');
  
  await client.deleteNamespace();
  server.close();
});
```

#### SSE Subscription Tests
```typescript
// test/tests/subscription.test.ts
test('stream subscription receives pokes', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  const pokes: any[] = [];
  const subscription = client.subscribe('account-1', {
    position: 0,
    onPoke: (poke) => pokes.push(poke)
  });
  
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
  await client.deleteNamespace();
  server.close();
});
```

#### Utility Function Tests
```typescript
// test/tests/util.test.ts
test('category extraction', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  expect(await client.rpc('util.category', 'account-123')).toBe('account');
  expect(await client.rpc('util.category', 'account-123+456')).toBe('account');
  expect(await client.rpc('util.category', 'account')).toBe('account');
  
  server.close();
});

test('id extraction', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  expect(await client.rpc('util.id', 'account-123')).toBe('123');
  expect(await client.rpc('util.id', 'account-123+456')).toBe('123+456');
  expect(await client.rpc('util.id', 'account')).toBeNull();
  
  server.close();
});

test('cardinal id extraction', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  expect(await client.rpc('util.cardinalId', 'account-123')).toBe('123');
  expect(await client.rpc('util.cardinalId', 'account-123+456')).toBe('123');
  expect(await client.rpc('util.cardinalId', 'account')).toBeNull();
  
  server.close();
});

test('category check', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  expect(await client.rpc('util.isCategory', 'account')).toBe(true);
  expect(await client.rpc('util.isCategory', 'account-123')).toBe(false);
  
  server.close();
});

test('hash64 consistency', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  const hash1 = await client.rpc('util.hash64', 'account-123');
  const hash2 = await client.rpc('util.hash64', 'account-123');
  
  expect(hash1).toBe(hash2); // Deterministic
  expect(typeof hash1).toBe('number');
  
  server.close();
});
```

#### Namespace Isolation Tests
```typescript
// test/tests/namespace.test.ts
test('namespaces are isolated', async () => {
  const server = await startTestServer();
  
  const client1 = new MessageDBClient(server.url);
  const client2 = new MessageDBClient(server.url);
  
  // Write to first namespace (auto-created)
  await client1.writeMessage('account-123', { type: 'Opened', data: {} });
  const messagesA = await client1.getStream('account-123');
  expect(messagesA).toHaveLength(1);
  
  // Write to second namespace (different auto-created namespace)
  await client2.writeMessage('account-123', { type: 'Opened', data: {} });
  const messagesB = await client2.getStream('account-123');
  expect(messagesB).toHaveLength(1);
  
  // Verify they're different namespaces
  expect(client1.token).not.toBe(client2.token);
  
  await client1.deleteNamespace();
  await client2.deleteNamespace();
  server.close();
});
```

#### Concurrency Tests
```typescript
// test/tests/concurrency.test.ts
test('concurrent writes to different streams', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  const writes = Array.from({ length: 100 }, (_, i) =>
    client.writeMessage(`account-${i}`, {
      type: 'Opened',
      data: { index: i }
    })
  );
  
  const results = await Promise.all(writes);
  expect(results).toHaveLength(100);
  results.forEach(r => expect(r.position).toBe(0));
  
  await client.deleteNamespace();
  server.close();
});
```

#### Message DB Compatibility Tests
```typescript
// test/tests/compatibility.test.ts
// These tests verify compatibility with Message DB reference implementation

test('hash64 matches Message DB', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  // Test vectors from Message DB
  const testCases = [
    { input: 'account-123', expected: 'EXPECTED_HASH_FROM_MESSAGE_DB' },
    { input: 'order-456', expected: 'EXPECTED_HASH_FROM_MESSAGE_DB' },
    // Add reference hashes from actual Message DB instance
  ];
  
  for (const tc of testCases) {
    const hash = await client.rpc('util.hash64', tc.input);
    expect(hash).toBe(tc.expected);
  }
  
  server.close();
});

test('consumer group assignment matches Message DB', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  // Write test data matching Message DB test suite
  const streams = [
    'account-123', 'account-456', 'account-789',
    'account-123+alice', 'account-123+bob'
  ];
  
  for (const stream of streams) {
    await client.writeMessage(stream, { type: 'Test', data: {} });
  }
  
  // Verify consumer 0 of 2 gets same streams as Message DB
  const messages = await client.getCategory('account', {
    consumerGroup: { member: 0, size: 2 }
  });
  
  const streamNames = messages.map(m => m[1]);
  
  // Verify against known Message DB behavior
  // Streams with cardinal_id 123 should be together
  const has123 = streamNames.includes('account-123');
  const has123Alice = streamNames.includes('account-123+alice');
  const has123Bob = streamNames.includes('account-123+bob');
  
  expect(has123).toBe(has123Alice);
  expect(has123).toBe(has123Bob);
  
  await client.deleteNamespace();
  server.close();
});

test('time format matches ISO 8601', async () => {
  const server = await startTestServer();
  const client = new MessageDBClient(server.url);
  
  await client.writeMessage('test-1', { type: 'Test', data: {} });
  const messages = await client.getStream('test-1');
  
  const time = messages[0][6]; // time at index 6
  
  // Verify ISO 8601 format with Z suffix
  expect(time).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/);
  
  // Verify parseable as date
  const date = new Date(time);
  expect(date.toISOString()).toBe(time);
  
  await client.deleteNamespace();
  server.close();
});
```

### FR-3: Test Server Helper

```typescript
// test/lib/helpers.ts
export async function startTestServer(opts = {}) {
  const proc = Bun.spawn([
    './messagedb',
    'serve',
    '--test-mode',
    '--port=0',  // Random available port
  ]);
  
  // Wait for server ready
  const port = await waitForHealthy(proc);
  
  return {
    url: `http://localhost:${port}`,
    close: () => proc.kill()
  };
}

async function waitForHealthy(proc: any): Promise<number> {
  // Parse port from stdout
  // Poll health endpoint until ready
  // Return port number
}
```

### FR-4: Performance Benchmarks

**Benchmark Suite:**
```typescript
// test/benchmarks/performance.bench.ts
import { bench } from 'bun:test';

bench('stream write single message', async () => {
  await client.writeMessage('bench-stream', {
    type: 'Event',
    data: { value: Math.random() }
  });
});

bench('stream read 100 messages', async () => {
  await client.getStream('bench-stream', { batchSize: 100 });
});

bench('category read with consumer group', async () => {
  await client.getCategory('bench', {
    consumerGroup: { member: 0, size: 4 }
  });
});
```

### FR-5: Docker Deployment

**Dockerfile:**
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o messagedb ./cmd/messagedb

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/messagedb .

EXPOSE 8080
CMD ["./messagedb", "serve"]
```

**Docker Compose Example:**
```yaml
# docker-compose.yml
version: '3.8'

services:
  messagedb:
    build: .
    ports:
      - "8080:8080"
    environment:
      - MESSAGEDB_DB_HOST=postgres
      - MESSAGEDB_DB_PORT=5432
      - MESSAGEDB_DB_NAME=message_store
      - MESSAGEDB_DB_USER=message_store
      - MESSAGEDB_DB_PASSWORD=secret
    depends_on:
      - postgres
  
  postgres:
    image: postgres:14
    environment:
      - POSTGRES_DB=message_store
      - POSTGRES_USER=message_store
      - POSTGRES_PASSWORD=secret
    volumes:
      - pgdata:/var/lib/postgresql/data

volumes:
  pgdata:
```

### FR-6: CI/CD Pipeline

**GitHub Actions Workflow:**
```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Run Go tests
        run: go test -v ./...
      
      - name: Run Go linter
        uses: golangci/golangci-lint-action@v3
  
  test-external:
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
  
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Run benchmarks
        working-directory: ./test
        run: bun run benchmarks
      
      - name: Compare with baseline
        run: |
          # Compare performance metrics with baseline
          # Fail if performance regressed > 20%
```

### FR-7: Documentation

**Documentation Structure:**
```
docs/
├── README.md                   # Getting started
├── API.md                      # Complete API reference
├── DEPLOYMENT.md               # Production deployment guide
├── MIGRATION.md                # Migrating from Message DB
├── PERFORMANCE.md              # Performance tuning
└── examples/
    ├── basic-usage.md
    ├── optimistic-locking.md
    ├── consumer-groups.md
    └── subscriptions.md
```

## Implementation Strategy

### Phase 1: External Test Client (Day 1-2)
- Set up Bun.js test environment
- Implement TypeScript MessageDB client
- Create test server helper
- Add test utilities and fixtures

### Phase 2: Stream & Category Tests (Day 3-4)
- Implement stream operation tests
- Implement category operation tests
- Add optimistic locking tests
- Add error handling tests

### Phase 3: Subscription & Concurrency Tests (Day 5-6)
- Implement SSE subscription tests
- Add concurrent write tests
- Add namespace isolation tests
- Performance smoke tests

### Phase 4: Performance Benchmarks (Day 7)
- Set up benchmark suite
- Implement core operation benchmarks
- Add baseline metrics
- Create performance regression detection

### Phase 5: Docker & Deployment (Day 8-9)
- Create Dockerfile
- Add docker-compose example
- Test container build and run
- Add Kubernetes manifests (optional)

### Phase 6: CI/CD Pipeline (Day 10-11)
- Set up GitHub Actions workflows
- Add Go unit tests job
- Add external tests job
- Add benchmark job
- Add Docker build job

### Phase 7: Documentation & Examples (Day 12-14)
- Write API documentation
- Create deployment guide
- Add migration guide
- Create usage examples
- Write performance tuning guide

## Acceptance Criteria

### AC-1: External Tests Pass
- **GIVEN** MessageDB server running in test mode
- **WHEN** External test suite executes
- **THEN** All tests pass (100% success rate)

### AC-2: Performance Within Target
- **GIVEN** Benchmark suite running against server
- **WHEN** Performance metrics collected
- **THEN** All metrics within target ranges (p95 < 50ms, etc.)

### AC-3: Docker Image Builds
- **GIVEN** Dockerfile and source code
- **WHEN** Docker build command executed
- **THEN** Image builds successfully and runs

### AC-4: CI Pipeline Runs
- **GIVEN** GitHub Actions workflow configured
- **WHEN** Code pushed to repository
- **THEN** All CI jobs pass (tests, linting, benchmarks)

### AC-5: Documentation Complete
- **GIVEN** Documentation files
- **WHEN** User follows guides
- **THEN** Can successfully deploy and use MessageDB

### AC-6: Namespace Isolation Verified
- **GIVEN** Multiple concurrent tests
- **WHEN** Tests run in parallel
- **THEN** No data leakage between namespaces

### AC-7: Message DB Compatibility Verified
- **GIVEN** Reference data from Message DB
- **WHEN** Running compatibility tests
- **THEN** Hash function produces identical results
- **AND** Consumer group assignments match exactly

### AC-8: Utility Functions Work
- **GIVEN** Stream name "account-123+alice"
- **WHEN** Calling utility functions
- **THEN** Correct parsing (category="account", cardinalId="123", id="123+alice")

### AC-9: Compound IDs Tested
- **GIVEN** Streams with compound IDs
- **WHEN** Consumer group query executed
- **THEN** Streams with same cardinal ID route to same consumer

### AC-10: Time Format Standardized
- **GIVEN** Message written and retrieved
- **WHEN** Checking time field
- **THEN** Returns ISO 8601 UTC string with Z suffix

## Definition of Done

- [ ] Bun.js test suite implemented
- [ ] TypeScript MessageDB client working
- [ ] All stream operation tests passing
- [ ] All category operation tests passing
- [ ] Consumer group tests with compound IDs passing
- [ ] Correlation filtering tests passing
- [ ] All utility function tests passing
- [ ] SSE subscription tests passing
- [ ] Namespace isolation tests passing
- [ ] Concurrent operation tests passing
- [ ] Message DB compatibility tests passing
- [ ] Hash function compatibility verified
- [ ] Consumer group assignment compatibility verified
- [ ] Time format standardization verified
- [ ] Performance benchmarks implemented
- [ ] All benchmarks meet target metrics
- [ ] Dockerfile builds and runs
- [ ] Docker Compose example working
- [ ] GitHub Actions CI pipeline working
- [ ] All CI jobs passing on every commit
- [ ] API documentation complete (including utility functions)
- [ ] Deployment guide complete
- [ ] Migration guide from Message DB complete
- [ ] Usage examples complete (including compound IDs)
- [ ] Performance tuning guide complete
- [ ] Ready for alpha release

## Performance Expectations

| Operation | Target | Acceptable |
|-----------|--------|------------|
| Stream write | <10ms p95 | <20ms p95 |
| Stream read (100 msgs) | <20ms p95 | <40ms p95 |
| Category read (100 msgs) | <30ms p95 | <60ms p95 |
| SSE poke delivery | <5ms | <10ms |
| Namespace creation | <100ms | <200ms |
| Overall throughput | 1000+ writes/sec | 500+ writes/sec |

## Test Coverage Targets

- **External test coverage:** All API methods tested
- **Error scenarios:** All error codes tested
- **Concurrency scenarios:** Race conditions tested
- **Integration scenarios:** End-to-end workflows tested
- **Performance scenarios:** All critical paths benchmarked

## Non-Goals

- ❌ Load testing (use external tools)
- ❌ Chaos engineering (future version)
- ❌ Security penetration testing (future version)
- ❌ Client library generation (manual clients for now)
- ❌ GraphQL/WebSocket testing (not in scope)

## Dependencies

- **MDB002:** RPC API & Authentication must be complete
- **Bun.js:** For external test suite
- **Docker:** For containerization
- **GitHub Actions:** For CI/CD

## References

- ADR-003: External Test Suite with Bun.js
- ADR-001: RPC-Style API Format
- Message DB Test Suite: https://github.com/message-db/message-db/tree/master/test
