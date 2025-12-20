# ISSUE006: Node.js/TypeScript SDK for MessageDB

**Status**: Planned  
**Priority**: High  
**Effort**: 4-6 hours  
**Created**: 2024-12-20  
**Related**: `docs/SDK-TEST-SPEC.md`, `ISSUE004-sdk-elixir.md`

---

## **Overview**

Implement a production-ready, minimal Node.js/TypeScript SDK for MessageDB that passes all tests defined in `docs/SDK-TEST-SPEC.md`. The SDK will be extracted from the existing `test_external/lib/client.ts` implementation and packaged as a standalone, publishable npm package.

**Location**: `clients/messagedb-node/`

**Key Principles**:
- Keep it minimal - no over-engineering
- Zero runtime dependencies (use native `fetch` and `EventSource`)
- Full TypeScript support with comprehensive types
- Follow Node.js/npm conventions
- Tests run against live backend server
- Each test creates its own namespace for isolation

---

## **Implementation Plan**

### Phase 1: Project Setup (30 min)

**1.1 Initialize npm Project**
```bash
cd clients
mkdir messagedb-node
cd messagedb-node
npm init -y
```

**1.2 Configure TypeScript and Build**
Create `tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ES2020",
    "lib": ["ES2020"],
    "moduleResolution": "node",
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true,
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist", "test"]
}
```

**1.3 Configure package.json**
```json
{
  "name": "@messagedb/client",
  "version": "0.1.0",
  "description": "Official Node.js/TypeScript client for MessageDB",
  "type": "module",
  "main": "./dist/index.js",
  "types": "./dist/index.d.ts",
  "exports": {
    ".": {
      "types": "./dist/index.d.ts",
      "import": "./dist/index.js"
    }
  },
  "files": [
    "dist",
    "README.md",
    "LICENSE"
  ],
  "scripts": {
    "build": "tsc",
    "test": "npm run build && node --test test/**/*.test.js",
    "prepublishOnly": "npm run build",
    "clean": "rm -rf dist"
  },
  "keywords": [
    "messagedb",
    "event-sourcing",
    "message-store",
    "event-store",
    "cqrs"
  ],
  "author": "MessageDB Team",
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "https://github.com/yourusername/messagedb.git",
    "directory": "clients/messagedb-node"
  },
  "devDependencies": {
    "@types/node": "^20.10.0",
    "typescript": "^5.3.0"
  },
  "engines": {
    "node": ">=18.0.0"
  }
}
```

**1.4 Project Structure**
```
clients/messagedb-node/
├── src/
│   ├── index.ts              # Main exports
│   ├── client.ts             # MessageDBClient class
│   ├── types.ts              # TypeScript types & interfaces
│   └── errors.ts             # Error classes
├── test/
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
│   ├── encoding.test.ts      # ENCODING-* tests
│   └── sse.test.ts           # SSE-* tests (optional)
├── tsconfig.json
├── package.json
├── README.md
├── CHANGELOG.md
└── LICENSE
```

---

### Phase 2: Core SDK Implementation (2 hours)

**2.1 Types Module** (`src/types.ts`)

Define comprehensive TypeScript types:
```typescript
/**
 * Message to write to a stream
 */
export interface Message {
  /** Event type identifier */
  type: string;
  /** Event payload data */
  data: Record<string, any>;
  /** Optional metadata */
  metadata?: {
    correlationStreamName?: string;
    causationMessageId?: string;
    [key: string]: any;
  };
}

/**
 * Options for writing a message
 */
export interface WriteOptions {
  /** Custom message ID (UUID) */
  id?: string;
  /** Expected stream version for optimistic locking (-1 = no stream) */
  expectedVersion?: number;
}

/**
 * Result from writing a message
 */
export interface WriteResult {
  /** Position in the stream (0-indexed) */
  position: number;
  /** Global position across all streams */
  globalPosition: number;
}

/**
 * Options for reading from a stream
 */
export interface GetStreamOptions {
  /** Start reading from this stream position */
  position?: number;
  /** Start reading from this global position */
  globalPosition?: number;
  /** Maximum number of messages to return (-1 for unlimited) */
  batchSize?: number;
}

/**
 * Options for reading from a category
 */
export interface GetCategoryOptions extends GetStreamOptions {
  /** Filter by correlation stream name prefix */
  correlation?: string;
  /** Consumer group settings for load balancing */
  consumerGroup?: {
    member: number;
    size: number;
  };
}

/**
 * Options for getting last message
 */
export interface GetLastOptions {
  /** Filter by message type */
  type?: string;
}

/**
 * Stream message (8 fields)
 * [id, type, position, globalPosition, data, metadata, time]
 */
export type StreamMessage = [
  string,              // id
  string,              // type
  number,              // position
  number,              // globalPosition
  Record<string, any>, // data
  Record<string, any> | null, // metadata
  string               // time (ISO 8601)
];

/**
 * Category message (8 fields)
 * [id, streamName, type, position, globalPosition, data, metadata, time]
 */
export type CategoryMessage = [
  string,              // id
  string,              // streamName
  string,              // type
  number,              // position
  number,              // globalPosition
  Record<string, any>, // data
  Record<string, any> | null, // metadata
  string               // time (ISO 8601)
];

/**
 * Options for creating a namespace
 */
export interface CreateNamespaceOptions {
  /** Custom token for the namespace */
  token?: string;
  /** Human-readable description */
  description?: string;
  /** Additional metadata */
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
 * Server health status
 */
export interface HealthStatus {
  status: string;
  backend: string;
  connections: number;
}

/**
 * Options for subscribing to updates
 */
export interface SubscribeOptions {
  /** Start from this position */
  position?: number;
  /** Consumer group settings */
  consumerGroup?: {
    member: number;
    size: number;
  };
  /** Callback for poke events */
  onPoke?: (poke: PokeEvent) => void;
  /** Callback for errors */
  onError?: (error: Error) => void;
  /** Callback when connection closes */
  onClose?: () => void;
}

/**
 * Poke event from SSE subscription
 */
export interface PokeEvent {
  stream?: string;
  category?: string;
  position: number;
  globalPosition: number;
}

/**
 * Active subscription handle
 */
export interface Subscription {
  close: () => void;
}

/**
 * Client configuration options
 */
export interface ClientOptions {
  /** Authentication token */
  token?: string;
  /** Request timeout in milliseconds (default: 30000) */
  timeout?: number;
}
```

**2.2 Error Module** (`src/errors.ts`)

```typescript
/**
 * Base error class for MessageDB errors
 */
export class MessageDBError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: any
  ) {
    super(message);
    this.name = 'MessageDBError';
    Error.captureStackTrace(this, this.constructor);
  }
}

/**
 * Error indicating stream version conflict (optimistic locking)
 */
export class VersionConflictError extends MessageDBError {
  constructor(message: string, details?: any) {
    super('STREAM_VERSION_CONFLICT', message, details);
    this.name = 'VersionConflictError';
  }
}

/**
 * Error indicating authentication failure
 */
export class AuthenticationError extends MessageDBError {
  constructor(message: string, details?: any) {
    super('AUTH_REQUIRED', message, details);
    this.name = 'AuthenticationError';
  }
}

/**
 * Error indicating namespace not found
 */
export class NamespaceNotFoundError extends MessageDBError {
  constructor(message: string, details?: any) {
    super('NAMESPACE_NOT_FOUND', message, details);
    this.name = 'NamespaceNotFoundError';
  }
}

/**
 * Error indicating namespace already exists
 */
export class NamespaceExistsError extends MessageDBError {
  constructor(message: string, details?: any) {
    super('NAMESPACE_EXISTS', message, details);
    this.name = 'NamespaceExistsError';
  }
}

/**
 * Parse error from RPC response
 */
export function parseError(errorData: any): MessageDBError {
  const code = errorData.code || 'UNKNOWN_ERROR';
  const message = errorData.message || 'An unknown error occurred';
  const details = errorData.details;

  switch (code) {
    case 'STREAM_VERSION_CONFLICT':
      return new VersionConflictError(message, details);
    case 'AUTH_REQUIRED':
    case 'AUTH_INVALID':
      return new AuthenticationError(message, details);
    case 'NAMESPACE_NOT_FOUND':
      return new NamespaceNotFoundError(message, details);
    case 'NAMESPACE_EXISTS':
      return new NamespaceExistsError(message, details);
    default:
      return new MessageDBError(code, message, details);
  }
}
```

**2.3 Client Module** (`src/client.ts`)

Core implementation (extracted and refined from `test_external/lib/client.ts`):
```typescript
import {
  Message,
  WriteOptions,
  WriteResult,
  GetStreamOptions,
  GetCategoryOptions,
  GetLastOptions,
  StreamMessage,
  CategoryMessage,
  CreateNamespaceOptions,
  NamespaceResult,
  DeleteNamespaceResult,
  NamespaceInfo,
  HealthStatus,
  SubscribeOptions,
  PokeEvent,
  Subscription,
  ClientOptions
} from './types.js';
import { MessageDBError, parseError } from './errors.js';

/**
 * MessageDB Client
 * 
 * Official Node.js/TypeScript client for MessageDB.
 * 
 * @example
 * ```typescript
 * const client = new MessageDBClient('http://localhost:8080', {
 *   token: 'ns_...'
 * });
 * 
 * // Write a message
 * const result = await client.writeMessage('account-123', {
 *   type: 'Deposited',
 *   data: { amount: 100 }
 * });
 * 
 * // Read messages
 * const messages = await client.getStream('account-123');
 * ```
 */
export class MessageDBClient {
  private token?: string;
  private timeout: number;

  constructor(
    private baseURL: string,
    options: ClientOptions = {}
  ) {
    this.token = options.token;
    this.timeout = options.timeout || 30000;
  }

  /**
   * Make an RPC call to the MessageDB server.
   * Request format: ["method", arg1, arg2, ...]
   */
  private async rpc(method: string, ...args: any[]): Promise<any> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json'
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(`${this.baseURL}/rpc`, {
        method: 'POST',
        headers,
        body: JSON.stringify([method, ...args]),
        signal: controller.signal
      });

      // Auto-capture token from response header (for test mode)
      const newToken = response.headers.get('X-MessageDB-Token');
      if (newToken && !this.token) {
        this.token = newToken;
      }

      if (!response.ok) {
        const errorData = await response.json();
        throw parseError(errorData.error || errorData);
      }

      return response.json();
    } catch (error) {
      if (error instanceof MessageDBError) {
        throw error;
      }
      if (error.name === 'AbortError') {
        throw new MessageDBError('TIMEOUT', 'Request timeout');
      }
      throw new MessageDBError(
        'NETWORK_ERROR',
        `Network error: ${error.message}`
      );
    } finally {
      clearTimeout(timeoutId);
    }
  }

  // ==================
  // Stream Operations
  // ==================

  /**
   * Write a message to a stream.
   * 
   * @param streamName - Name of the stream (e.g., 'account-123')
   * @param message - Message to write
   * @param options - Write options (id, expectedVersion)
   * @returns Write result with position and globalPosition
   */
  async writeMessage(
    streamName: string,
    message: Message,
    options?: WriteOptions
  ): Promise<WriteResult> {
    return this.rpc('stream.write', streamName, message, options || {});
  }

  /**
   * Get messages from a stream.
   * 
   * @param streamName - Name of the stream
   * @param options - Query options (position, batchSize, etc.)
   * @returns Array of messages (8-element arrays)
   */
  async getStream(
    streamName: string,
    options?: GetStreamOptions
  ): Promise<StreamMessage[]> {
    return this.rpc('stream.get', streamName, options || {});
  }

  /**
   * Get the last message from a stream.
   * 
   * @param streamName - Name of the stream
   * @param options - Filter options (type)
   * @returns Last message or null if stream is empty
   */
  async getLastMessage(
    streamName: string,
    options?: GetLastOptions
  ): Promise<StreamMessage | null> {
    return this.rpc('stream.last', streamName, options || {});
  }

  /**
   * Get the current version of a stream.
   * 
   * @param streamName - Name of the stream
   * @returns Stream version (last position) or null if stream doesn't exist
   */
  async getStreamVersion(streamName: string): Promise<number | null> {
    return this.rpc('stream.version', streamName);
  }

  // ====================
  // Category Operations
  // ====================

  /**
   * Get messages from all streams in a category.
   * 
   * @param categoryName - Category name (e.g., 'account')
   * @param options - Query options (position, correlation, consumerGroup, etc.)
   * @returns Array of category messages (8-element arrays with streamName)
   */
  async getCategory(
    categoryName: string,
    options?: GetCategoryOptions
  ): Promise<CategoryMessage[]> {
    return this.rpc('category.get', categoryName, options || {});
  }

  // =====================
  // Namespace Operations
  // =====================

  /**
   * Create a new namespace.
   * 
   * @param namespaceId - Namespace identifier
   * @param options - Creation options (token, description, metadata)
   * @returns Namespace info with generated token
   */
  async createNamespace(
    namespaceId: string,
    options?: CreateNamespaceOptions
  ): Promise<NamespaceResult> {
    return this.rpc('ns.create', namespaceId, options || {});
  }

  /**
   * Delete a namespace and all its data.
   * 
   * @param namespaceId - Namespace identifier
   * @returns Deletion info
   */
  async deleteNamespace(namespaceId: string): Promise<DeleteNamespaceResult> {
    return this.rpc('ns.delete', namespaceId);
  }

  /**
   * List all namespaces.
   * 
   * @returns Array of namespace info objects
   */
  async listNamespaces(): Promise<NamespaceInfo[]> {
    return this.rpc('ns.list');
  }

  /**
   * Get information about a namespace.
   * 
   * @param namespaceId - Namespace identifier
   * @returns Namespace info
   */
  async getNamespaceInfo(namespaceId: string): Promise<NamespaceInfo> {
    return this.rpc('ns.info', namespaceId);
  }

  // ===================
  // System Operations
  // ===================

  /**
   * Get the MessageDB server version.
   * 
   * @returns Version string (e.g., "1.0.0")
   */
  async getVersion(): Promise<string> {
    return this.rpc('sys.version');
  }

  /**
   * Get the server health status.
   * 
   * @returns Health status object
   */
  async getHealth(): Promise<HealthStatus> {
    return this.rpc('sys.health');
  }

  // =====================
  // Subscription (SSE)
  // =====================

  /**
   * Subscribe to a stream for real-time updates via Server-Sent Events.
   * 
   * @param streamName - Name of the stream
   * @param options - Subscription options (position, callbacks)
   * @returns Subscription handle with close() method
   */
  subscribeToStream(
    streamName: string,
    options: SubscribeOptions = {}
  ): Subscription {
    const params = new URLSearchParams({
      stream: streamName,
      position: String(options.position || 0)
    });

    if (this.token) {
      params.set('token', this.token);
    }

    return this.createSubscription(params, options);
  }

  /**
   * Subscribe to a category for real-time updates via Server-Sent Events.
   * 
   * @param categoryName - Category name
   * @param options - Subscription options (position, consumerGroup, callbacks)
   * @returns Subscription handle with close() method
   */
  subscribeToCategory(
    categoryName: string,
    options: SubscribeOptions = {}
  ): Subscription {
    const params = new URLSearchParams({
      category: categoryName,
      position: String(options.position || 0)
    });

    if (options.consumerGroup) {
      params.set('consumerGroupMember', String(options.consumerGroup.member));
      params.set('consumerGroupSize', String(options.consumerGroup.size));
    }

    if (this.token) {
      params.set('token', this.token);
    }

    return this.createSubscription(params, options);
  }

  /**
   * Internal helper to create SSE subscription
   */
  private createSubscription(
    params: URLSearchParams,
    options: SubscribeOptions
  ): Subscription {
    const url = `${this.baseURL}/subscribe?${params}`;
    const eventSource = new EventSource(url);

    eventSource.addEventListener('poke', (event) => {
      if (options.onPoke) {
        try {
          const data = JSON.parse(event.data);
          options.onPoke(data);
        } catch (err) {
          if (options.onError) {
            options.onError(new Error(`Failed to parse poke: ${err}`));
          }
        }
      }
    });

    eventSource.onerror = () => {
      if (options.onError) {
        options.onError(new Error('SSE connection error'));
      }
    };

    return {
      close: () => {
        eventSource.close();
      }
    };
  }

  // =================
  // Token Management
  // =================

  /**
   * Get the current authentication token.
   * 
   * @returns Current token or undefined
   */
  getToken(): string | undefined {
    return this.token;
  }

  /**
   * Set the authentication token.
   * 
   * @param token - New token to use
   */
  setToken(token: string): void {
    this.token = token;
  }
}
```

**2.4 Index Module** (`src/index.ts`)

```typescript
export { MessageDBClient } from './client.js';
export {
  MessageDBError,
  VersionConflictError,
  AuthenticationError,
  NamespaceNotFoundError,
  NamespaceExistsError
} from './errors.js';
export type {
  Message,
  WriteOptions,
  WriteResult,
  GetStreamOptions,
  GetCategoryOptions,
  GetLastOptions,
  StreamMessage,
  CategoryMessage,
  CreateNamespaceOptions,
  NamespaceResult,
  DeleteNamespaceResult,
  NamespaceInfo,
  HealthStatus,
  SubscribeOptions,
  PokeEvent,
  Subscription,
  ClientOptions
} from './types.js';
```

---

### Phase 3: Test Infrastructure (1 hour)

**3.1 Test Helpers** (`test/helpers.ts`)

Use Node.js built-in test runner (available in Node 18+):
```typescript
import { MessageDBClient } from '../dist/index.js';
import { spawn, ChildProcess } from 'child_process';

const DEFAULT_PORT = 6789;
const SERVER_BIN = process.env.MESSAGEDB_BIN || '../../golang/messagedb';
const DEFAULT_TOKEN = 'ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000';
const STARTUP_TIMEOUT_MS = 10000;

export interface TestContext {
  namespace: string;
  token: string;
  client: MessageDBClient;
  cleanup: () => Promise<void>;
}

let namespaceCounter = 0;

/**
 * Generate unique namespace ID
 */
function generateNamespaceId(testName: string): string {
  const counter = (namespaceCounter++).toString(36);
  const timestamp = Date.now().toString(36);
  const random = Math.random().toString(36).substring(2, 6);
  return `t_${counter}_${timestamp}_${random}`;
}

/**
 * Wait for server to be healthy
 */
async function waitForHealthy(url: string, timeoutMs: number): Promise<void> {
  const startTime = Date.now();
  
  while (Date.now() - startTime < timeoutMs) {
    try {
      const response = await fetch(`${url}/health`);
      if (response.ok) return;
    } catch {
      // Keep trying
    }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  
  throw new Error(`Server not healthy after ${timeoutMs}ms`);
}

/**
 * Setup test with isolated namespace
 */
export async function setupTest(testName: string): Promise<TestContext> {
  const serverUrl = process.env.MESSAGEDB_URL || `http://localhost:${DEFAULT_PORT}`;
  
  // Create unique namespace
  const namespace = generateNamespaceId(testName);
  const token = `ns_${Buffer.from(namespace).toString('base64url')}_${'0'.repeat(64)}`;
  
  // Create namespace using admin client
  const admin = new MessageDBClient(serverUrl, { token: DEFAULT_TOKEN });
  await admin.createNamespace(namespace, { token });
  
  // Create client for this namespace
  const client = new MessageDBClient(serverUrl, { token });
  
  return {
    namespace,
    token,
    client,
    cleanup: async () => {
      try {
        await admin.deleteNamespace(namespace);
      } catch {
        // Ignore cleanup errors
      }
    }
  };
}

/**
 * Generate random stream name
 */
export function randomStreamName(category: string = 'test'): string {
  const id = Math.random().toString(36).substring(2, 15);
  return `${category}-${id}`;
}
```

**3.2 Test Structure Pattern**

Example test file (`test/write.test.ts`):
```typescript
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { setupTest, randomStreamName } from './helpers.js';

test('WRITE-001: Write minimal message', async () => {
  const t = await setupTest('write-001');
  try {
    const stream = randomStreamName();
    const message = { type: 'TestEvent', data: { foo: 'bar' } };
    
    const result = await t.client.writeMessage(stream, message);
    
    assert.ok(result.position >= 0);
    assert.ok(result.globalPosition >= 0);
    assert.equal(result.position, 0); // First message
  } finally {
    await t.cleanup();
  }
});

test('WRITE-002: Write message with metadata', async () => {
  const t = await setupTest('write-002');
  try {
    const stream = randomStreamName();
    const message = {
      type: 'TestEvent',
      data: { foo: 'bar' },
      metadata: { correlationId: '123' }
    };
    
    const result = await t.client.writeMessage(stream, message);
    assert.ok(result.position >= 0);
    
    // Read back and verify metadata
    const messages = await t.client.getStream(stream);
    assert.equal(messages.length, 1);
    assert.deepEqual(messages[0][5], { correlationId: '123' }); // metadata at index 5
  } finally {
    await t.cleanup();
  }
});

// ... more tests following SDK-TEST-SPEC.md
```

---

### Phase 4: Test Implementation (2 hours)

Implement tests in priority order following `docs/SDK-TEST-SPEC.md`:

**Tier 1 (Must Have) - 1.5 hours**
- `write.test.ts`: WRITE-001 through WRITE-010
- `read.test.ts`: READ-001 through READ-010
- `auth.test.ts`: AUTH-001 through AUTH-004
- `error.test.ts`: ERROR-001 through ERROR-007

**Tier 2 (Should Have) - 30 min**
- `last.test.ts`: LAST-001 through LAST-004
- `version.test.ts`: VERSION-001 through VERSION-003
- `category.test.ts`: CATEGORY-001 through CATEGORY-008
- `namespace.test.ts`: NS-001 through NS-008
- `system.test.ts`: SYS-001, SYS-002

**Tier 3 (Nice to Have) - Future**
- `encoding.test.ts`: ENCODING-001 through ENCODING-010
- `sse.test.ts`: SSE-001 through SSE-009
- Edge case tests

---

### Phase 5: Documentation & Polish (30 min)

**5.1 README.md**
```markdown
# @messagedb/client

Official Node.js/TypeScript client for MessageDB - a simple, fast message store.

## Features

- ✅ Zero runtime dependencies (uses native `fetch` and `EventSource`)
- ✅ Full TypeScript support with comprehensive types
- ✅ Async/await API
- ✅ Stream and category operations
- ✅ Namespace management
- ✅ Real-time subscriptions via Server-Sent Events
- ✅ Optimistic locking support
- ✅ Consumer groups for load balancing

## Installation

```bash
npm install @messagedb/client
# or
yarn add @messagedb/client
# or
pnpm add @messagedb/client
```

## Requirements

- Node.js 18+ (uses native `fetch` and `EventSource`)

## Quick Start

```typescript
import { MessageDBClient } from '@messagedb/client';

// Create client
const client = new MessageDBClient('http://localhost:8080', {
  token: 'ns_...'  // Your namespace token
});

// Write a message
const result = await client.writeMessage('account-123', {
  type: 'Deposited',
  data: { amount: 100, currency: 'USD' }
});

console.log(`Written at position ${result.position}`);

// Read messages
const messages = await client.getStream('account-123');
messages.forEach(msg => {
  const [id, type, position, globalPosition, data, metadata, time] = msg;
  console.log(`${type}: ${JSON.stringify(data)}`);
});

// Get last message
const lastMsg = await client.getLastMessage('account-123');

// Subscribe to updates
const subscription = client.subscribeToStream('account-123', {
  onPoke: (poke) => {
    console.log('New message at position', poke.position);
  }
});
```

## API Reference

### Client Creation

```typescript
const client = new MessageDBClient(baseURL, options);
```

**Options:**
- `token?: string` - Authentication token
- `timeout?: number` - Request timeout in ms (default: 30000)

### Stream Operations

#### Write Message

```typescript
const result = await client.writeMessage(streamName, message, options);
```

**Message:**
- `type: string` - Event type
- `data: object` - Event data
- `metadata?: object` - Optional metadata

**Options:**
- `id?: string` - Custom message ID (UUID)
- `expectedVersion?: number` - Expected stream version for optimistic locking

**Returns:** `{ position: number, globalPosition: number }`

#### Read Stream

```typescript
const messages = await client.getStream(streamName, options);
```

**Options:**
- `position?: number` - Start from position
- `globalPosition?: number` - Start from global position
- `batchSize?: number` - Max messages to return (-1 for unlimited)

**Returns:** Array of messages (8-element arrays)

#### Get Last Message

```typescript
const message = await client.getLastMessage(streamName, options);
```

**Options:**
- `type?: string` - Filter by message type

**Returns:** Message or `null` if empty

#### Get Stream Version

```typescript
const version = await client.getStreamVersion(streamName);
```

**Returns:** Last position or `null` if stream doesn't exist

### Category Operations

#### Read Category

```typescript
const messages = await client.getCategory(categoryName, options);
```

**Options:**
- `position?: number` - Start from global position
- `batchSize?: number` - Max messages to return
- `correlation?: string` - Filter by correlation stream prefix
- `consumerGroup?: { member: number, size: number }` - Consumer group settings

**Returns:** Array of category messages (8-element arrays with streamName)

### Namespace Operations

```typescript
// Create namespace
const result = await client.createNamespace(namespaceId, {
  description: 'My namespace',
  token: 'custom-token' // optional
});

// Delete namespace
await client.deleteNamespace(namespaceId);

// List namespaces
const namespaces = await client.listNamespaces();

// Get namespace info
const info = await client.getNamespaceInfo(namespaceId);
```

### System Operations

```typescript
// Get server version
const version = await client.getVersion();

// Get health status
const health = await client.getHealth();
```

### Subscriptions (Server-Sent Events)

```typescript
// Subscribe to stream
const sub = client.subscribeToStream('account-123', {
  position: 0,
  onPoke: (poke) => console.log('New message!', poke),
  onError: (error) => console.error('SSE error', error),
  onClose: () => console.log('Connection closed')
});

// Subscribe to category
const sub = client.subscribeToCategory('account', {
  consumerGroup: { member: 0, size: 2 },
  onPoke: (poke) => console.log('New message!', poke)
});

// Close subscription
sub.close();
```

## Error Handling

The client throws specific error classes for different scenarios:

```typescript
import {
  MessageDBError,
  VersionConflictError,
  AuthenticationError,
  NamespaceNotFoundError
} from '@messagedb/client';

try {
  await client.writeMessage(stream, message, { expectedVersion: 5 });
} catch (error) {
  if (error instanceof VersionConflictError) {
    console.log('Version conflict - retry with current version');
  } else if (error instanceof AuthenticationError) {
    console.log('Invalid token');
  } else if (error instanceof MessageDBError) {
    console.log('MessageDB error:', error.code, error.message);
  }
}
```

## Testing

```bash
# Run tests (requires MessageDB server running)
npm test

# With custom server URL
MESSAGEDB_URL=http://localhost:8080 npm test
```

Tests automatically create and clean up isolated namespaces.

## Development

```bash
# Install dependencies
npm install

# Build TypeScript
npm run build

# Run tests
npm test

# Clean build artifacts
npm run clean
```

## License

MIT

## Contributing

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.
```

**5.2 CHANGELOG.md**
```markdown
# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2024-12-20

### Added
- Initial release
- Complete MessageDB RPC API support
- TypeScript types and interfaces
- Stream operations (write, read, last, version)
- Category operations (read with consumer groups)
- Namespace management
- System operations
- SSE subscriptions for real-time updates
- Comprehensive error handling
- Zero runtime dependencies
```

**5.3 LICENSE**
```
MIT License

Copyright (c) 2024 MessageDB Team

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

## **Success Criteria**

### Core Requirements
- [ ] All Tier 1 tests passing (WRITE, READ, AUTH, ERROR)
- [ ] All Tier 2 tests passing (CATEGORY, NS, SYS, LAST, VERSION)
- [ ] Zero runtime dependencies (only TypeScript as devDependency)
- [ ] Full TypeScript support with .d.ts files
- [ ] Tests create/cleanup their own namespaces
- [ ] Clean, idiomatic Node.js/TypeScript code
- [ ] Comprehensive README with examples

### Package Quality
- [ ] Works with Node.js 18+ (native fetch/EventSource)
- [ ] ES modules format
- [ ] Source maps for debugging
- [ ] Type declarations (.d.ts) generated
- [ ] Proper package.json exports configuration
- [ ] No compilation warnings or errors

### Documentation
- [ ] README with installation, usage, API reference
- [ ] CHANGELOG tracking versions
- [ ] MIT LICENSE file
- [ ] Code comments and JSDoc for public APIs
- [ ] TypeScript examples in documentation

---

## **Implementation Summary**

### ✅ Core Components

1. **Types** (`src/types.ts`)
   - Comprehensive TypeScript interfaces
   - Message, WriteOptions, WriteResult
   - Stream and category message types
   - Namespace, system, and subscription types

2. **Errors** (`src/errors.ts`)
   - MessageDBError base class
   - Specific error types (VersionConflict, Auth, etc.)
   - Error parsing from RPC responses

3. **Client** (`src/client.ts`)
   - MessageDBClient class
   - All RPC methods implemented
   - Token management
   - SSE subscriptions
   - Timeout handling

4. **Exports** (`src/index.ts`)
   - Clean public API
   - All types exported
   - Error classes exported

### 📊 Test Coverage Target

**Total Test Cases**: 60+ (matching SDK-TEST-SPEC.md)

- **Tier 1** (Must Have): 35+ tests
  - WRITE: 10 tests
  - READ: 10 tests
  - AUTH: 4 tests
  - ERROR: 7 tests
  - LAST: 4 tests

- **Tier 2** (Should Have): 25+ tests
  - VERSION: 3 tests
  - CATEGORY: 8 tests
  - NAMESPACE: 8 tests
  - SYSTEM: 2 tests

- **Tier 3** (Nice to Have): Future
  - ENCODING: 10 tests
  - SSE: 9 tests
  - EDGE: 8 tests

### 🎯 API Coverage

All MessageDB RPC endpoints:
- ✅ `stream.write` - Write with optimistic locking
- ✅ `stream.get` - Read with filters
- ✅ `stream.last` - Get last message by type
- ✅ `stream.version` - Get stream version
- ✅ `category.get` - Read with consumer groups
- ✅ `ns.create` - Create namespace
- ✅ `ns.delete` - Delete namespace
- ✅ `ns.list` - List namespaces
- ✅ `ns.info` - Get namespace info
- ✅ `sys.version` - Get server version
- ✅ `sys.health` - Get server health
- ✅ SSE subscriptions for real-time updates

---

## **Differences from Elixir SDK**

### Language-Specific Patterns

1. **Return Values**
   - **Elixir**: `{:ok, result, updated_client}` tuples
   - **Node.js**: `Promise<result>` with automatic token capture

2. **Error Handling**
   - **Elixir**: Pattern matching on `{:ok, _}` / `{:error, _}`
   - **Node.js**: Try/catch with typed error classes

3. **Configuration**
   - **Elixir**: Mix project with `req` dependency
   - **Node.js**: npm package with zero runtime dependencies

4. **Testing**
   - **Elixir**: ExUnit with async: false
   - **Node.js**: Built-in test runner (Node 18+)

### Advantages of Node.js Approach

1. **Zero Dependencies**: Uses native `fetch` and `EventSource` (Node 18+)
2. **TypeScript**: Strong typing without runtime overhead
3. **Async/Await**: Natural async patterns, no "client threading"
4. **npm Ecosystem**: Easy distribution and installation
5. **Browser Compatibility**: Same code can run in browsers (with bundler)

---

## **Publishing Checklist**

Before publishing to npm:

- [ ] All tests passing
- [ ] Version bumped in package.json
- [ ] CHANGELOG updated
- [ ] README examples tested
- [ ] Type declarations generated
- [ ] No TypeScript errors
- [ ] Build artifacts in dist/
- [ ] package.json "files" field correct
- [ ] npm pack and inspect tarball
- [ ] Test installation in separate project

```bash
# Build and test
npm run build
npm test

# Check package contents
npm pack
tar -tzf messagedb-client-0.1.0.tgz

# Publish to npm
npm publish --access public
```

---

## **Future Enhancements**

1. **Connection Pooling**: Reuse HTTP connections for better performance
2. **Retry Logic**: Automatic retry with exponential backoff
3. **Batched Writes**: Helper for writing multiple messages efficiently
4. **Projection Helpers**: High-level abstractions for building projections
5. **React Hooks**: Optional package with React hooks for subscriptions
6. **Middleware**: Plugin system for logging, metrics, etc.
7. **Browser Bundle**: Pre-built browser bundle via CDN

---

## **Integration with Test Runner**

Update `bin/run_sdk_tests.sh` to include Node.js tests:

```bash
# Run Node.js tests
if [[ "$SDK" == "nodejs" || "$SDK" == "all" ]]; then
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  Node.js SDK Tests${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    if [ -f "clients/messagedb-node/package.json" ]; then
        cd clients/messagedb-node
        npm install
        if MESSAGEDB_URL="$MESSAGEDB_URL" npm test; then
            PASSED=$((PASSED + 1))
        else
            FAILED=$((FAILED + 1))
        fi
        cd ../..
    else
        echo -e "${RED}✗ Node.js SDK not found${NC}"
        FAILED=$((FAILED + 1))
    fi
    echo ""
fi
```

---

## **References**

- Test Spec: `docs/SDK-TEST-SPEC.md`
- Elixir SDK Plan: `@meta/@pm/issues/ISSUE004-sdk-elixir.md`
- Existing TypeScript Client: `test_external/lib/client.ts`
- Test Runner: `bin/run_sdk_tests.sh`
- Node.js Test Runner: https://nodejs.org/api/test.html
- Native fetch: https://nodejs.org/api/globals.html#fetch
- Native EventSource: Available via undici in Node 18+

---

## **Timeline**

- **Day 1 (2-3 hours)**: Setup + Core implementation
  - Initialize project
  - Implement types, errors, client
  - Basic README

- **Day 2 (2-3 hours)**: Tests + Documentation
  - Implement Tier 1 tests
  - Implement Tier 2 tests
  - Complete documentation
  - Test against live server

**Total Estimated Time**: 4-6 hours

---

## **Notes**

### EventSource in Node.js

Node.js 18+ doesn't have native EventSource, but it's available via undici (which provides fetch). For the SDK, we can:

1. **Option A**: Add `eventsource` as optional dependency for Node.js
2. **Option B**: Document that SSE requires polyfill in Node.js
3. **Option C**: Use `undici` which is bundled in Node 18+

Recommendation: Option A for best DX.

```json
{
  "optionalDependencies": {
    "eventsource": "^2.0.2"
  }
}
```

```typescript
// In client.ts for SSE
const EventSource = globalThis.EventSource || 
  (await import('eventsource')).default;
```

### Test Execution

Tests can be run with:
```bash
# Node built-in test runner
node --test test/**/*.test.js

# Or with tsx for TypeScript
npx tsx --test test/**/*.test.ts
```

Choose Node built-in for zero dependencies approach.
