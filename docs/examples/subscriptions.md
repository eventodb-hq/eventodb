# Real-Time Subscriptions Example

MessageDB supports Server-Sent Events (SSE) for real-time notifications when new messages are written.

## Concept

```
┌─────────────┐          ┌─────────────┐          ┌─────────────┐
│   Client A  │          │  MessageDB  │          │   Client B  │
│  (Writer)   │          │   Server    │          │ (Subscriber)│
└─────────────┘          └─────────────┘          └─────────────┘
       │                        │                        │
       │                        │ ←─ SSE Connection ─────│
       │                        │         (open)         │
       │                        │                        │
       │── stream.write ───────>│                        │
       │<───── position=0 ──────│                        │
       │                        │───── poke event ──────>│
       │                        │   {stream, position}   │
       │                        │                        │
       │── stream.write ───────>│                        │
       │<───── position=1 ──────│                        │
       │                        │───── poke event ──────>│
       │                        │                        │
```

## Stream Subscription

### curl Example

```bash
# Subscribe to a stream (long-running connection)
curl -N "http://localhost:8080/subscribe?stream=account-123&position=0&token=$TOKEN"
```

**Events received:**
```
event: poke
data: {"stream":"account-123","position":0,"globalPosition":1001}

event: poke
data: {"stream":"account-123","position":1,"globalPosition":1002}
```

### JavaScript (Browser)

```javascript
const token = 'ns_ZGVmYXVsdA_...';
const streamName = 'account-123';

const eventSource = new EventSource(
  `http://localhost:8080/subscribe?stream=${streamName}&position=0&token=${token}`
);

eventSource.addEventListener('poke', (event) => {
  const poke = JSON.parse(event.data);
  console.log(`New message in ${poke.stream} at position ${poke.position}`);
});

eventSource.onerror = (error) => {
  console.error('SSE error:', error);
  // Handle reconnection
};

// Later: close connection
eventSource.close();
```

### TypeScript (Node.js/Bun)

```typescript
import { MessageDBClient } from './lib/client';

const client = new MessageDBClient('http://localhost:8080', {
  token: process.env.TOKEN
});

// Subscribe to stream
const subscription = client.subscribeToStream('account-123', {
  position: 0,
  onPoke: (poke) => {
    console.log(`New message at position ${poke.position}`);
    // Fetch and process the new message
    processNewMessage(poke);
  },
  onError: (error) => {
    console.error('Subscription error:', error);
  },
  onClose: () => {
    console.log('Subscription closed');
  }
});

// Later: close subscription
subscription.close();

async function processNewMessage(poke: { stream: string; position: number }) {
  const messages = await client.getStream(poke.stream, {
    position: poke.position,
    batchSize: 1
  });
  
  if (messages.length > 0) {
    const [id, type, position, globalPosition, data, metadata, time] = messages[0];
    console.log(`Processing ${type}: ${JSON.stringify(data)}`);
  }
}
```

## Category Subscription

### Subscribe to All Streams in a Category

```bash
curl -N "http://localhost:8080/subscribe?category=account&position=0&token=$TOKEN"
```

### With Consumer Group

```bash
# Consumer 0 of 4
curl -N "http://localhost:8080/subscribe?category=account&position=0&consumerGroupMember=0&consumerGroupSize=4&token=$TOKEN"
```

### TypeScript

```typescript
const subscription = client.subscribeToCategory('account', {
  position: 0,
  consumerGroup: {
    member: 0,
    size: 4
  },
  onPoke: async (poke) => {
    console.log(`New message in ${poke.stream}`);
    // Fetch messages for this stream
    await processCategoryMessage(poke);
  }
});

async function processCategoryMessage(poke: { stream: string; globalPosition: number }) {
  // For category subscriptions, fetch using global position
  const messages = await client.getCategory('account', {
    globalPosition: poke.globalPosition,
    batchSize: 1,
    consumerGroup: { member: 0, size: 4 }
  });
  
  for (const msg of messages) {
    const [id, streamName, type, position, globalPosition, data] = msg;
    console.log(`${streamName}: ${type}`);
  }
}
```

## Poke Event Format

```json
{
  "stream": "account-123",
  "position": 5,
  "globalPosition": 1234
}
```

| Field | Description |
|-------|-------------|
| `stream` | Full stream name (e.g., `account-123`) |
| `position` | Position within the stream (0-based) |
| `globalPosition` | Database-wide sequence number |

## Complete Example: Real-Time Account Monitor

### Backend (Processing Service)

```typescript
import { MessageDBClient } from './lib/client';

const client = new MessageDBClient('http://localhost:8080', {
  token: process.env.TOKEN
});

interface AccountBalance {
  accountId: string;
  balance: number;
  lastUpdated: Date;
}

// In-memory cache (use Redis in production)
const balances = new Map<string, AccountBalance>();

async function startAccountMonitor() {
  // Catch up with existing messages
  console.log('Catching up with existing messages...');
  await catchUp();
  
  // Subscribe for real-time updates
  console.log('Subscribing to account category...');
  const subscription = client.subscribeToCategory('account', {
    position: lastGlobalPosition,
    onPoke: handlePoke,
    onError: handleError
  });
  
  return subscription;
}

let lastGlobalPosition = 0;

async function catchUp() {
  while (true) {
    const messages = await client.getCategory('account', {
      globalPosition: lastGlobalPosition,
      batchSize: 1000
    });
    
    if (messages.length === 0) break;
    
    for (const msg of messages) {
      processAccountMessage(msg);
    }
    
    lastGlobalPosition = messages[messages.length - 1][4] + 1;
    console.log(`Processed ${messages.length} messages, position: ${lastGlobalPosition}`);
  }
}

async function handlePoke(poke: { stream: string; globalPosition: number }) {
  const messages = await client.getCategory('account', {
    globalPosition: lastGlobalPosition,
    batchSize: 100
  });
  
  for (const msg of messages) {
    processAccountMessage(msg);
    lastGlobalPosition = msg[4] + 1;
  }
}

function processAccountMessage(msg: any[]) {
  const [id, streamName, type, position, globalPosition, data] = msg;
  const accountId = streamName.split('-')[1]; // Extract ID from stream name
  
  let balance = balances.get(accountId) || {
    accountId,
    balance: 0,
    lastUpdated: new Date()
  };
  
  switch (type) {
    case 'AccountOpened':
      balance.balance = data.initialBalance || 0;
      break;
    case 'Deposited':
      balance.balance += data.amount;
      break;
    case 'Withdrawn':
      balance.balance -= data.amount;
      break;
  }
  
  balance.lastUpdated = new Date();
  balances.set(accountId, balance);
  
  console.log(`Account ${accountId}: $${balance.balance}`);
}

function handleError(error: Error) {
  console.error('Subscription error:', error);
  // Implement reconnection logic
}

// API endpoint for balance lookup
function getBalance(accountId: string): number {
  return balances.get(accountId)?.balance ?? 0;
}

// Start monitor
startAccountMonitor();
```

### Frontend (Browser)

```html
<!DOCTYPE html>
<html>
<head>
  <title>Account Balance Monitor</title>
</head>
<body>
  <h1>Account Balances</h1>
  <div id="balances"></div>
  
  <script>
    const token = 'ns_ZGVmYXVsdA_...';
    const balances = {};
    
    // Connect to SSE
    const eventSource = new EventSource(
      `http://localhost:8080/subscribe?category=account&token=${token}`
    );
    
    eventSource.addEventListener('poke', async (event) => {
      const poke = JSON.parse(event.data);
      
      // Fetch the new message
      const response = await fetch('http://localhost:8080/rpc', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify([
          'stream.get',
          poke.stream,
          { position: poke.position, batchSize: 1 }
        ])
      });
      
      const messages = await response.json();
      if (messages.length > 0) {
        updateBalance(poke.stream, messages[0]);
        renderBalances();
      }
    });
    
    function updateBalance(stream, msg) {
      const [id, type, position, globalPosition, data] = msg;
      const accountId = stream.split('-')[1];
      
      if (!balances[accountId]) {
        balances[accountId] = { balance: 0 };
      }
      
      switch (type) {
        case 'AccountOpened':
          balances[accountId].balance = data.initialBalance || 0;
          break;
        case 'Deposited':
          balances[accountId].balance += data.amount;
          break;
        case 'Withdrawn':
          balances[accountId].balance -= data.amount;
          break;
      }
    }
    
    function renderBalances() {
      const container = document.getElementById('balances');
      container.innerHTML = Object.entries(balances)
        .map(([id, data]) => `<p>Account ${id}: $${data.balance}</p>`)
        .join('');
    }
  </script>
</body>
</html>
```

## Reconnection Strategy

### Automatic Reconnection

```typescript
class ResilientSubscription {
  private eventSource: EventSource | null = null;
  private position: number;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 10;
  private reconnectDelay = 1000;
  
  constructor(
    private url: string,
    private onMessage: (poke: any) => void
  ) {
    this.position = 0;
  }
  
  start(position: number = 0) {
    this.position = position;
    this.connect();
  }
  
  private connect() {
    const url = `${this.url}&position=${this.position}`;
    console.log(`Connecting to ${url}`);
    
    this.eventSource = new EventSource(url);
    
    this.eventSource.addEventListener('poke', (event) => {
      const poke = JSON.parse(event.data);
      this.position = poke.globalPosition + 1;
      this.reconnectAttempts = 0; // Reset on successful message
      this.onMessage(poke);
    });
    
    this.eventSource.onerror = () => {
      this.handleDisconnect();
    };
  }
  
  private handleDisconnect() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnection attempts reached');
      return;
    }
    
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts);
    this.reconnectAttempts++;
    
    console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
    
    setTimeout(() => {
      this.connect();
    }, delay);
  }
  
  stop() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }
}

// Usage
const subscription = new ResilientSubscription(
  `http://localhost:8080/subscribe?category=account&token=${token}`,
  (poke) => {
    console.log(`Message in ${poke.stream} at ${poke.position}`);
  }
);

subscription.start(0);
```

## Backpressure Handling

### Queue-Based Processing

```typescript
class QueuedSubscription {
  private queue: any[] = [];
  private processing = false;
  
  constructor(
    private client: MessageDBClient,
    private handler: (messages: any[]) => Promise<void>
  ) {}
  
  start(stream: string) {
    return this.client.subscribeToStream(stream, {
      onPoke: (poke) => {
        this.queue.push(poke);
        this.processQueue();
      }
    });
  }
  
  private async processQueue() {
    if (this.processing || this.queue.length === 0) return;
    
    this.processing = true;
    
    try {
      while (this.queue.length > 0) {
        const poke = this.queue.shift();
        
        const messages = await this.client.getStream(poke.stream, {
          position: poke.position,
          batchSize: 1
        });
        
        if (messages.length > 0) {
          await this.handler(messages);
        }
      }
    } finally {
      this.processing = false;
    }
  }
}
```

### Debounced Processing

```typescript
class DebouncedSubscription {
  private pendingPokes = new Map<string, any>();
  private debounceTimer: Timer | null = null;
  private debounceMs = 100;
  
  constructor(
    private client: MessageDBClient,
    private handler: (pokes: Map<string, any>) => Promise<void>
  ) {}
  
  start(category: string) {
    return this.client.subscribeToCategory(category, {
      onPoke: (poke) => {
        // Store latest poke per stream
        this.pendingPokes.set(poke.stream, poke);
        this.scheduleFlush();
      }
    });
  }
  
  private scheduleFlush() {
    if (this.debounceTimer) return;
    
    this.debounceTimer = setTimeout(async () => {
      this.debounceTimer = null;
      
      const pokes = new Map(this.pendingPokes);
      this.pendingPokes.clear();
      
      await this.handler(pokes);
    }, this.debounceMs);
  }
}

// Usage: Process batches of updates
const subscription = new DebouncedSubscription(client, async (pokes) => {
  console.log(`Processing ${pokes.size} streams`);
  
  for (const [stream, poke] of pokes) {
    const messages = await client.getStream(stream, {
      position: poke.position,
      batchSize: 10
    });
    await processMessages(messages);
  }
});
```

## Testing Subscriptions

### Test Helper

```typescript
import { describe, test, expect } from 'bun:test';

async function waitForPokes(
  subscription: any,
  count: number,
  timeout: number = 5000
): Promise<any[]> {
  const pokes: any[] = [];
  
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`Timeout waiting for ${count} pokes, got ${pokes.length}`));
    }, timeout);
    
    // Store original onPoke
    const originalOnPoke = subscription.onPoke;
    
    subscription.onPoke = (poke: any) => {
      pokes.push(poke);
      originalOnPoke?.(poke);
      
      if (pokes.length >= count) {
        clearTimeout(timer);
        resolve(pokes);
      }
    };
  });
}

test('receives pokes for new messages', async () => {
  const pokes: any[] = [];
  
  const subscription = client.subscribeToStream('test-stream', {
    onPoke: (poke) => pokes.push(poke)
  });
  
  // Write messages
  await client.writeMessage('test-stream', { type: 'Event1', data: {} });
  await client.writeMessage('test-stream', { type: 'Event2', data: {} });
  
  // Wait for pokes
  await Bun.sleep(100);
  
  expect(pokes.length).toBe(2);
  expect(pokes[0].position).toBe(0);
  expect(pokes[1].position).toBe(1);
  
  subscription.close();
});
```

## Best Practices

1. **Always handle reconnection**: Network issues happen
2. **Start from checkpoint**: Don't reprocess old messages
3. **Handle backpressure**: Don't let pokes pile up
4. **Use consumer groups for scale**: Distribute load
5. **Implement graceful shutdown**: Close connections properly
6. **Monitor connection health**: Track reconnection rate

---

## Next Steps

- [Basic Usage](./basic-usage.md) - Core API operations
- [Consumer Groups](./consumer-groups.md) - Scale processing
- [Optimistic Locking](./optimistic-locking.md) - Prevent conflicts
