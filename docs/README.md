# MessageDB Go Server - Getting Started

MessageDB is a high-performance event store and message store implemented in Go, designed for Pub/Sub, Event Sourcing, Messaging, and Evented Microservices applications.

## Features

- **Event Sourcing**: Store and retrieve events as immutable facts
- **Pub/Sub**: Real-time notifications via Server-Sent Events (SSE)
- **Multi-tenancy**: Namespace isolation with token-based authentication
- **Consumer Groups**: Distribute workload across multiple consumers
- **Correlation Filtering**: Query messages by correlation stream
- **Optimistic Locking**: Prevent concurrent write conflicts
- **High Performance**: In-memory SQLite for testing, PostgreSQL for production

## Quick Start

### Prerequisites

- Go 1.21 or later
- (Optional) PostgreSQL 14+ for production use

### Installation

```bash
# Clone the repository
git clone https://github.com/message-db/message-db.git
cd message-db/golang

# Build the server
go build -o messagedb ./cmd/messagedb
```

### Running the Server

```bash
# Start server (in-memory SQLite, test mode)
./messagedb serve --test-mode --port=8080

# The server prints the default namespace token:
# ═══════════════════════════════════════════════════════
# DEFAULT NAMESPACE TOKEN:
# ns_ZGVmYXVsdA_a1b2c3d4e5f6...
# ═══════════════════════════════════════════════════════
```

### First Request

```bash
# Write a message to a stream
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ns_ZGVmYXVsdA_YOUR_TOKEN" \
  -d '["stream.write", "account-123", {"type": "AccountOpened", "data": {"balance": 0}}]'

# Response:
# {"position": 0, "globalPosition": 1}
```

```bash
# Read messages from a stream
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ns_ZGVmYXVsdA_YOUR_TOKEN" \
  -d '["stream.get", "account-123"]'

# Response:
# [["uuid", "AccountOpened", 0, 1, {"balance": 0}, null, "2024-01-15T10:30:00Z"]]
```

## Core Concepts

### Streams

A stream is an ordered sequence of events identified by a stream name. Stream names follow the pattern `category-id`:

- `account-123` - Account stream with ID 123
- `order-abc` - Order stream with ID abc
- `payment-xyz+retry` - Compound ID with cardinal ID xyz

### Categories

A category groups all streams sharing the same prefix. For example:
- Category `account` contains all `account-*` streams
- Query all events in a category with `category.get`

### Messages

Messages are immutable events with:
- **id**: Unique identifier (UUID)
- **type**: Event type (e.g., "AccountOpened")
- **data**: Event payload (JSON object)
- **metadata**: Optional metadata (correlation, causation, custom fields)
- **position**: Stream-local position (0-based)
- **globalPosition**: Database-wide sequence number

### Namespaces

Namespaces provide multi-tenant isolation. Each namespace:
- Has its own token for authentication
- Contains isolated streams and messages
- Can be created, listed, and deleted via API

## RPC API

MessageDB uses a simple RPC-style API over HTTP POST. Request format:

```json
["method", arg1, arg2, ...]
```

### Available Methods

| Method | Description |
|--------|-------------|
| `stream.write` | Write a message to a stream |
| `stream.get` | Read messages from a stream |
| `stream.last` | Get the last message from a stream |
| `stream.version` | Get current stream version |
| `category.get` | Read messages from a category |
| `ns.create` | Create a namespace |
| `ns.delete` | Delete a namespace |
| `ns.list` | List all namespaces |
| `ns.info` | Get namespace information |
| `sys.version` | Get server version |
| `sys.health` | Get server health status |

See [API Reference](./API.md) for detailed documentation.

## Real-Time Subscriptions

Subscribe to stream or category updates via SSE:

```bash
# Subscribe to a stream
curl -N "http://localhost:8080/subscribe?stream=account-123&token=YOUR_TOKEN"

# Subscribe to a category
curl -N "http://localhost:8080/subscribe?category=account&token=YOUR_TOKEN"
```

Poke events are sent when new messages are written:

```json
{"stream": "account-123", "position": 5, "globalPosition": 1234}
```

## TypeScript Client

A TypeScript client is available for testing:

```typescript
import { MessageDBClient } from './lib/client';

const client = new MessageDBClient('http://localhost:8080', {
  token: 'ns_ZGVmYXVsdA_YOUR_TOKEN'
});

// Write a message
const result = await client.writeMessage('account-123', {
  type: 'Deposited',
  data: { amount: 100 }
});

// Read messages
const messages = await client.getStream('account-123');
```

## Next Steps

- [API Reference](./API.md) - Complete API documentation
- [Deployment Guide](./DEPLOYMENT.md) - Production deployment
- [Migration Guide](./MIGRATION.md) - Migrating from PostgreSQL Message DB
- [Performance Tuning](./PERFORMANCE.md) - Optimization tips
- [Examples](./examples/) - Code examples

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     HTTP Clients                             │
│          (curl, TypeScript client, any HTTP client)          │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                  MessageDB Go Server                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │ RPC Handler │  │ Auth Middle │  │ SSE Handler │          │
│  │   /rpc      │  │   ware      │  │ /subscribe  │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
│                          │                                   │
│  ┌───────────────────────┴───────────────────────┐          │
│  │              Store Interface                   │          │
│  │  (SQLite in-memory / PostgreSQL)              │          │
│  └───────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

## License

MIT License - see [LICENSE](../MIT-License.txt)
