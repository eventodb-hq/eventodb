# ISSUE006: Node.js SDK for EventoDB

**Status**: Not Started  
**Priority**: High  
**Effort**: 3-5 hours  
**Created**: 2024-12-20

---

## Overview

Implement a minimal, clean Node.js/TypeScript SDK for EventoDB that passes all tests defined in `docs/SDK-TEST-SPEC.md`. The SDK will use native `fetch` API and follow Node.js/TypeScript conventions.

**Location**: `clients/eventodb-node/`

**Key Principles**:
- Zero external dependencies (use native Node.js APIs)
- Dual CommonJS + ESM support
- TypeScript with full type definitions
- Tests run against live backend server
- Each test creates its own namespace for isolation

---

## Implementation Plan

### Phase 1: Project Setup (30 min)

**1.1 Initialize npm Project**
```bash
cd clients
mkdir eventodb-node
cd eventodb-node
npm init -y
```

**1.2 Configure Dependencies**
```json
{
  "devDependencies": {
    "typescript": "^5.3.0",
    "@types/node": "^20.10.0",
    "vitest": "^1.0.0"
  }
}
```

**1.3 TypeScript Configuration**
Create `tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "Node16",
    "lib": ["ES2022"],
    "moduleResolution": "Node16",
    "outDir": "./dist",
    "rootDir": "./src",
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true,
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist", "tests"]
}
```

**1.4 Package Configuration**
Update `package.json`:
```json
{
  "name": "@eventodb/client",
  "version": "0.1.0",
  "description": "Node.js client for EventoDB",
  "main": "./dist/index.js",
  "types": "./dist/index.d.ts",
  "type": "module",
  "exports": {
    ".": {
      "import": "./dist/index.js",
      "types": "./dist/index.d.ts"
    }
  },
  "scripts": {
    "build": "tsc",
    "test": "vitest run",
    "test:watch": "vitest",
    "prepublishOnly": "npm run build"
  },
  "engines": {
    "node": ">=18.0.0"
  }
}
```

**1.5 Project Structure**
```
clients/eventodb-node/
├── src/
│   ├── index.ts              # Main exports
│   ├── client.ts             # Core client class
│   ├── types.ts              # TypeScript types
│   └── errors.ts             # Error classes
├── tests/
│   ├── helpers.ts            # Test utilities
│   ├── write.test.ts         # WRITE-* tests
│   ├── read.test.ts          # READ-* tests
│   ├── last.test.ts          # LAST-* tests
│   ├── version.test.ts       # VERSION-* tests
│   ├── category.test.ts      # CATEGORY-* tests
│   ├── namespace.test.ts     # NS-* tests
│   ├── system.test.ts        # SYS-* tests
│   ├── auth.test.ts          # AUTH-* tests
│   ├── error.test.ts         # ERROR-* tests
│   └── encoding.test.ts      # ENCODING-* tests
├── tsconfig.json
├── package.json
├── vitest.config.ts
├── README.md
└── run_tests.sh
```

---

### Phase 2: Core Client Implementation (1.5 hours)

**2.1 Types Module** (`src/types.ts`)

```typescript
/**
 * Message to write to a stream
 */
export interface Message {
  type: string;
  data: Record<string, any>;
  metadata?: Record<string, any> | null;
}

/**
 * Options for writing messages
 */
export interface WriteOptions {
  id?: string;
  expectedVersion?: number;
}

/**
 * Result from writing a message
 */
export interface WriteResult {
  position: number;
  globalPosition: number;
}

/**
 * Options for reading from a stream
 */
export interface GetStreamOptions {
  position?: number;
  globalPosition?: number;
  batchSize?: number;
}

/**
 * Options for reading from a category
 */
export interface GetCategoryOptions {
  position?: number;
  globalPosition?: number;
  batchSize?: number;
  correlation?: string;
  consumerGroup?: {
    member: number;
    size: number;
  };
}

/**
 * Options for getting last message
 */
export interface GetLastOptions {
  type?: string;
}

/**
 * Options for creating a namespace
 */
export interface CreateNamespaceOptions {
  token?: string;
  description?: string;
  metadata?: Record<string, any>;
}

/**
 * Result from creating a namespace
 */
export interface NamespaceResult {
  namespace: string;
  token: string;
  createdAt: string;
}

/**
 * Result from deleting a namespace
 */
export interface DeleteNamespaceResult {
  namespace: string;
  deletedAt: string;
  messagesDeleted: number;
}

/**
 * Namespace information
 */
export interface NamespaceInfo {
  namespace: string;
  description: string;
  createdAt: string;
  messageCount: number;
  streamCount: number;
  lastActivity: string | null;
}

/**
 * Stream message format: 
 * [id, type, position, globalPosition, data, metadata, time]
 */
export type StreamMessage = [
  string,                    // id
  string,                    // type
  number,                    // position
  number,                    // globalPosition
  Record<string, any>,       // data
  Record<string, any> | null, // metadata
  string                     // time (ISO 8601)
];

/**
 * Category message format:
 * [id, streamName, type, position, globalPosition, data, metadata, time]
 */
export type CategoryMessage = [
  string,                    // id
  string,                    // streamName
  string,                    // type
  number,                    // position
  number,                    // globalPosition
  Record<string, any>,       // data
  Record<string, any> | null, // metadata
  string                     // time (ISO 8601)
];
```

**2.2 Error Module** (`src/errors.ts`)

```typescript
/**
 * Base error for EventoDB operations
 */
export class EventoDBError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: Record<string, any>
  ) {
    super(message);
    this.name = 'EventoDBError';
  }

  /**
   * Create error from server response
   */
  static fromResponse(errorData: {
    code: string;
    message: string;
    details?: Record<string, any>;
  }): EventoDBError {
    return new EventoDBError(
      errorData.code,
      errorData.message,
      errorData.details
    );
  }
}

/**
 * Network/connection errors
 */
export class NetworkError extends EventoDBError {
  constructor(message: string, cause?: Error) {
    super('NETWORK_ERROR', message, { cause });
    this.name = 'NetworkError';
  }
}

/**
 * Authentication errors
 */
export class AuthError extends EventoDBError {
  constructor(code: string, message: string) {
    super(code, message);
    this.name = 'AuthError';
  }
}
```

**2.3 Client Module** (`src/client.ts`)

```typescript
import { EventoDBError, NetworkError } from './errors.js';
import type {
  Message,
  WriteOptions,
  WriteResult,
  GetStreamOptions,
  GetCategoryOptions,
  GetLastOptions,
  CreateNamespaceOptions,
  NamespaceResult,
  DeleteNamespaceResult,
  NamespaceInfo,
  StreamMessage,
  CategoryMessage
} from './types.js';

/**
 * EventoDB Client
 * 
 * Core client for interacting with EventoDB via RPC API.
 */
export class EventoDBClient {
  private token?: string;

  constructor(
    private readonly baseURL: string,
    options: { token?: string } = {}
  ) {
    this.token = options.token;
  }

  /**
   * Make an RPC call to the server
   */
  private async rpc(method: string, ...args: any[]): Promise<any> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json'
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    let response: Response;
    try {
      response = await fetch(`${this.baseURL}/rpc`, {
        method: 'POST',
        headers,
        body: JSON.stringify([method, ...args])
      });
    } catch (error) {
      throw new NetworkError(
        `Failed to connect to ${this.baseURL}`,
        error as Error
      );
    }

    // Capture token from response header (auto-creation in test mode)
    const newToken = response.headers.get('X-EventoDB-Token');
    if (newToken && !this.token) {
      this.token = newToken;
    }

    if (!response.ok) {
      const errorData = await response.json();
      if (errorData.error) {
        throw EventoDBError.fromResponse(errorData.error);
      }
      throw new EventoDBError(
        'UNKNOWN_ERROR',
        `HTTP ${response.status}: ${response.statusText}`
      );
    }

    return response.json();
  }

  // ==================
  // Stream Operations
  // ==================

  /**
   * Write a message to a stream
   */
  async streamWrite(
    streamName: string,
    message: Message,
    options: WriteOptions = {}
  ): Promise<WriteResult> {
    return this.rpc('stream.write', streamName, message, options);
  }

  /**
   * Get messages from a stream
   */
  async streamGet(
    streamName: string,
    options: GetStreamOptions = {}
  ): Promise<StreamMessage[]> {
    return this.rpc('stream.get', streamName, options);
  }

  /**
   * Get the last message from a stream
   */
  async streamLast(
    streamName: string,
    options: GetLastOptions = {}
  ): Promise<StreamMessage | null> {
    return this.rpc('stream.last', streamName, options);
  }

  /**
   * Get the version of a stream
   */
  async streamVersion(streamName: string): Promise<number | null> {
    return this.rpc('stream.version', streamName);
  }

  // ====================
  // Category Operations
  // ====================

  /**
   * Get messages from a category
   */
  async categoryGet(
    categoryName: string,
    options: GetCategoryOptions = {}
  ): Promise<CategoryMessage[]> {
    return this.rpc('category.get', categoryName, options);
  }

  // =====================
  // Namespace Operations
  // =====================

  /**
   * Create a new namespace
   */
  async namespaceCreate(
    namespaceId: string,
    options: CreateNamespaceOptions = {}
  ): Promise<NamespaceResult> {
    return this.rpc('ns.create', namespaceId, options);
  }

  /**
   * Delete a namespace
   */
  async namespaceDelete(namespaceId: string): Promise<DeleteNamespaceResult> {
    return this.rpc('ns.delete', namespaceId);
  }

  /**
   * List all namespaces
   */
  async namespaceList(): Promise<NamespaceInfo[]> {
    return this.rpc('ns.list');
  }

  /**
   * Get namespace information
   */
  async namespaceInfo(namespaceId: string): Promise<NamespaceInfo> {
    return this.rpc('ns.info', namespaceId);
  }

  // ===================
  // System Operations
  // ===================

  /**
   * Get server version
   */
  async systemVersion(): Promise<string> {
    return this.rpc('sys.version');
  }

  /**
   * Get server health status
   */
  async systemHealth(): Promise<{ status: string }> {
    return this.rpc('sys.health');
  }

  /**
   * Get current authentication token
   */
  getToken(): string | undefined {
    return this.token;
  }
}
```

**2.4 Main Export** (`src/index.ts`)

```typescript
export { EventoDBClient } from './client.js';
export { EventoDBError, NetworkError, AuthError } from './errors.js';
export type {
  Message,
  WriteOptions,
  WriteResult,
  GetStreamOptions,
  GetCategoryOptions,
  GetLastOptions,
  CreateNamespaceOptions,
  NamespaceResult,
  DeleteNamespaceResult,
  NamespaceInfo,
  StreamMessage,
  CategoryMessage
} from './types.js';
```

---

### Phase 3: Test Infrastructure (1 hour)

**3.1 Vitest Configuration** (`vitest.config.ts`)

```typescript
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    globals: false,
    environment: 'node',
    testTimeout: 30000,
    hookTimeout: 30000
  }
});
```

**3.2 Test Helpers** (`tests/helpers.ts`)

```typescript
import { EventoDBClient } from '../src/client.js';

const MESSAGEDB_URL = process.env.MESSAGEDB_URL || 'http://localhost:8080';
const ADMIN_TOKEN = process.env.MESSAGEDB_ADMIN_TOKEN;

/**
 * Test context with isolated namespace
 */
export interface TestContext {
  client: EventoDBClient;
  namespaceId: string;
  token: string;
  cleanup: () => Promise<void>;
}

/**
 * Setup test with isolated namespace
 */
export async function setupTest(testName: string): Promise<TestContext> {
  // Create admin client for namespace management
  const adminClient = new EventoDBClient(MESSAGEDB_URL, { 
    token: ADMIN_TOKEN 
  });

  const namespaceId = `test-${testName}-${uniqueSuffix()}`;
  
  const result = await adminClient.namespaceCreate(namespaceId, {
    description: `Test namespace for ${testName}`
  });

  // Create client with namespace token
  const client = new EventoDBClient(MESSAGEDB_URL, { 
    token: result.token 
  });

  return {
    client,
    namespaceId,
    token: result.token,
    cleanup: async () => {
      await adminClient.namespaceDelete(namespaceId);
    }
  };
}

/**
 * Generate unique stream name
 */
export function randomStreamName(prefix = 'test'): string {
  return `${prefix}-${uniqueSuffix()}`;
}

/**
 * Generate unique suffix
 */
function uniqueSuffix(): string {
  return `${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
}

/**
 * Get EventoDB URL
 */
export function getEventoDBURL(): string {
  return MESSAGEDB_URL;
}
```

**3.3 Test Runner Script** (`run_tests.sh`)

```bash
#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}→ Building TypeScript...${NC}"
npm run build

echo -e "${YELLOW}→ Running tests...${NC}"
npm test

echo -e "${GREEN}✓ All tests passed!${NC}"
```

**3.4 Test Structure Pattern**

Each test file follows this pattern:

```typescript
import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';

describe('WRITE Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    // Cleanup all test namespaces
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('WRITE-001: Write minimal message', async () => {
    const ctx = await setupTest('write-001');
    contexts.push(ctx);

    const stream = randomStreamName();
    const result = await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { foo: 'bar' }
    });

    expect(result.position).toBeGreaterThanOrEqual(0);
    expect(result.globalPosition).toBeGreaterThanOrEqual(0);
  });

  // More tests...
});
```

---

### Phase 4: Test Implementation (1.5 hours)

Implement tests in priority order following `docs/SDK-TEST-SPEC.md`:

**Tier 1 (Must Have) - 1 hour**
- `write.test.ts`: WRITE-001 through WRITE-010
- `read.test.ts`: READ-001 through READ-010
- `auth.test.ts`: AUTH-001 through AUTH-004
- `error.test.ts`: ERROR-001 through ERROR-004

**Tier 2 (Should Have) - 30 min**
- `last.test.ts`: LAST-001 through LAST-004
- `version.test.ts`: VERSION-001 through VERSION-003
- `category.test.ts`: CATEGORY-001 through CATEGORY-008
- `namespace.test.ts`: NS-001 through NS-008
- `system.test.ts`: SYS-001, SYS-002

**Tier 3 (Nice to Have) - Future**
- `encoding.test.ts`: ENCODING-001 through ENCODING-010
- `edge.test.ts`: EDGE-001 through EDGE-008
- SSE tests (requires EventSource implementation)

---

### Phase 5: Documentation & Polish (30 min)

**5.1 README.md**

```markdown
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
- `streamGet(streamName, options?)` - Read messages
- `streamLast(streamName, options?)` - Get last message
- `streamVersion(streamName)` - Get stream version

### Category Operations

- `categoryGet(categoryName, options?)` - Read from category

### Namespace Operations

- `namespaceCreate(id, options?)` - Create namespace
- `namespaceDelete(id)` - Delete namespace
- `namespaceList()` - List namespaces
- `namespaceInfo(id)` - Get namespace info

### System Operations

- `systemVersion()` - Get server version
- `systemHealth()` - Get server health

## Testing

Tests run against a live EventoDB server:

```bash
# Start server
docker-compose up -d

# Run tests
npm test

# With custom URL
MESSAGEDB_URL=http://localhost:8080 npm test
```

## License

MIT
```

**5.2 Package Metadata**

Update `package.json` with:
- Author, license, repository
- Keywords: `eventodb`, `event-sourcing`, `message-store`, `cqrs`
- Homepage and bug tracker URLs

---

## Success Criteria

- [ ] All Tier 1 tests passing (WRITE, READ, AUTH, ERROR)
- [ ] All Tier 2 tests passing (CATEGORY, NS, SYS, LAST, VERSION)
- [ ] Zero external runtime dependencies
- [ ] Full TypeScript type definitions
- [ ] Dual ESM/CommonJS support
- [ ] Tests create/cleanup their own namespaces
- [ ] README with clear usage examples
- [ ] Built artifacts in `dist/` directory

---

## Implementation Notes

### Zero Dependencies Strategy

Use native Node.js APIs only:
- `fetch` for HTTP (available in Node 18+)
- No need for external HTTP libraries
- Built-in `crypto` for UUID generation if needed
- Use `vitest` for testing (dev dependency only)

### Type Safety

Leverage TypeScript's type system:
- Message format types match server arrays exactly
- Options objects are properly typed
- Error classes extend standard Error
- Export all types for consumer use

### Error Handling

```typescript
try {
  await client.streamWrite(stream, message, { expectedVersion: 5 });
} catch (error) {
  if (error instanceof EventoDBError) {
    console.log(`Error code: ${error.code}`);
    console.log(`Message: ${error.message}`);
    console.log(`Details:`, error.details);
  }
}
```

### Async/Await Pattern

All methods return Promises for async/await usage:
```typescript
const result = await client.streamWrite(stream, message);
const messages = await client.streamGet(stream);
```

---

## Integration with Test Runner

Update `bin/run_sdk_tests.sh` to include Node.js tests:

```bash
# Run Node.js tests
if [[ "$SDK" == "node" || "$SDK" == "all" ]]; then
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  Node.js SDK Tests${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    if [ -f "clients/eventodb-node/run_tests.sh" ]; then
        cd clients/eventodb-node
        if MESSAGEDB_URL="$MESSAGEDB_URL" MESSAGEDB_ADMIN_TOKEN="$ADMIN_TOKEN" ./run_tests.sh; then
            PASSED=$((PASSED + 1))
        else
            FAILED=$((FAILED + 1))
        fi
        cd ../..
    else
        echo -e "${RED}✗ Node.js SDK test runner not found${NC}"
        FAILED=$((FAILED + 1))
    fi
    echo ""
fi
```

---

## Potential Enhancements (Future)

1. **SSE Subscriptions**: Implement EventSource-based subscriptions
2. **Retry Logic**: Configurable retry with exponential backoff
3. **Connection Pooling**: HTTP agent with keep-alive
4. **Batched Writes**: Helper for writing multiple messages
5. **Stream Helpers**: Higher-level abstractions (projections, aggregates)
6. **CLI Tool**: Command-line interface for EventoDB operations
7. **Logging**: Optional structured logging integration
8. **Metrics**: Optional telemetry/metrics integration

---

## References

- Test Spec: `docs/SDK-TEST-SPEC.md`
- Elixir SDK: `clients/eventodb_ex/`
- TypeScript Client (test): `test_external/lib/client.ts`
- Test Runner: `bin/run_sdk_tests.sh`
