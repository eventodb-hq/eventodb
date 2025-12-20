# Consumer Groups Example

Consumer groups distribute message processing across multiple consumers, enabling horizontal scaling.

## Concept

```
┌─────────────────────────────────────────────────────────────┐
│                    Category: account                         │
│                                                              │
│  account-123  account-456  account-789  account-012         │
│      │            │            │            │                │
│      └────────────┴────────────┴────────────┘                │
│                      │                                       │
│         ┌───────────┼───────────┐                           │
│         ▼           ▼           ▼                           │
│    Consumer 0  Consumer 1  Consumer 2                       │
│    (member=0)  (member=1)  (member=2)                       │
│    (size=3)    (size=3)    (size=3)                         │
│                                                              │
│    Receives:   Receives:   Receives:                        │
│    account-123 account-456 account-789                      │
│    account-012 ...         ...                              │
└─────────────────────────────────────────────────────────────┘
```

Each stream is assigned to exactly one consumer based on a hash of its ID.

## Basic Usage

### Query as Consumer Group Member

```bash
# Consumer 0 of 3
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '[
    "category.get",
    "account",
    {
      "consumerGroup": {
        "member": 0,
        "size": 3
      }
    }
  ]'

# Consumer 1 of 3
curl -X POST http://localhost:8080/rpc \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '[
    "category.get",
    "account",
    {
      "consumerGroup": {
        "member": 1,
        "size": 3
      }
    }
  ]'
```

### TypeScript Implementation

```typescript
import { EventoDBClient } from './lib/client';

const client = new EventoDBClient('http://localhost:8080', {
  token: process.env.TOKEN
});

// Read messages for this consumer
async function getMyMessages(
  category: string,
  member: number,
  size: number,
  opts: { position?: number; batchSize?: number } = {}
) {
  return client.getCategory(category, {
    ...opts,
    consumerGroup: { member, size }
  });
}

// Usage
const CONSUMER_ID = parseInt(process.env.CONSUMER_ID || '0');
const CONSUMER_COUNT = parseInt(process.env.CONSUMER_COUNT || '4');

const messages = await getMyMessages('account', CONSUMER_ID, CONSUMER_COUNT, {
  batchSize: 100
});

console.log(`Consumer ${CONSUMER_ID}/${CONSUMER_COUNT} received ${messages.length} messages`);
```

## Stream Assignment

### How Assignment Works

Consumer assignment uses the stream's **cardinal ID**:

```
Stream Name          Cardinal ID    Hash        Consumer (size=4)
-----------          -----------    ----        -----------------
account-123          123            12345678    12345678 % 4 = 2
account-456          456            87654321    87654321 % 4 = 1
account-123+retry    123            12345678    12345678 % 4 = 2 (same!)
```

**Key Point**: Compound IDs (with `+`) share the same cardinal ID and consumer.

### Verify Assignment

```typescript
async function verifyConsumerGroups(client: EventoDBClient) {
  // Write test data
  const streams = ['account-1', 'account-2', 'account-3', 'account-4', 'account-5'];
  for (const stream of streams) {
    await client.writeMessage(stream, { type: 'Test', data: {} });
  }
  
  // Query each consumer
  const consumerSize = 2;
  const assignments: Record<number, string[]> = {};
  
  for (let member = 0; member < consumerSize; member++) {
    const messages = await client.getCategory('account', {
      consumerGroup: { member, size: consumerSize }
    });
    
    // Extract unique stream names
    const streamNames = [...new Set(messages.map(m => m[1]))];
    assignments[member] = streamNames;
    
    console.log(`Consumer ${member}: ${streamNames.join(', ')}`);
  }
  
  // Verify no overlap
  const allStreams = Object.values(assignments).flat();
  const uniqueStreams = new Set(allStreams);
  
  if (allStreams.length !== uniqueStreams.size) {
    console.error('ERROR: Streams assigned to multiple consumers!');
  } else {
    console.log('OK: No overlap between consumers');
  }
}
```

## Consumer Implementation

### Basic Consumer

```typescript
interface ConsumerConfig {
  category: string;
  member: number;
  size: number;
  batchSize: number;
  pollingInterval: number;
}

class CategoryConsumer {
  private running = false;
  private position = 0;
  
  constructor(
    private client: EventoDBClient,
    private config: ConsumerConfig,
    private handler: (messages: any[]) => Promise<void>
  ) {}
  
  async start() {
    this.running = true;
    console.log(`Starting consumer ${this.config.member}/${this.config.size}`);
    
    while (this.running) {
      try {
        const messages = await this.client.getCategory(this.config.category, {
          globalPosition: this.position,
          batchSize: this.config.batchSize,
          consumerGroup: {
            member: this.config.member,
            size: this.config.size
          }
        });
        
        if (messages.length > 0) {
          await this.handler(messages);
          // Update position to after last message
          this.position = messages[messages.length - 1][4] + 1;
        } else {
          // No messages, wait before polling again
          await sleep(this.config.pollingInterval);
        }
      } catch (error) {
        console.error('Consumer error:', error);
        await sleep(this.config.pollingInterval);
      }
    }
  }
  
  stop() {
    this.running = false;
  }
}

// Usage
const consumer = new CategoryConsumer(client, {
  category: 'account',
  member: 0,
  size: 4,
  batchSize: 100,
  pollingInterval: 1000
}, async (messages) => {
  for (const msg of messages) {
    const [id, streamName, type, position, globalPosition, data] = msg;
    console.log(`Processing ${type} from ${streamName}`);
    // Handle message...
  }
});

consumer.start();

// Later...
// consumer.stop();
```

### Consumer with Checkpointing

```typescript
interface Checkpoint {
  position: number;
  updatedAt: Date;
}

class CheckpointedConsumer {
  private position = 0;
  private running = false;
  
  constructor(
    private client: EventoDBClient,
    private config: ConsumerConfig,
    private handler: (messages: any[]) => Promise<void>,
    private checkpointStore: {
      load: () => Promise<Checkpoint | null>;
      save: (checkpoint: Checkpoint) => Promise<void>;
    }
  ) {}
  
  async start() {
    // Load checkpoint
    const checkpoint = await this.checkpointStore.load();
    if (checkpoint) {
      this.position = checkpoint.position;
      console.log(`Resuming from position ${this.position}`);
    }
    
    this.running = true;
    
    while (this.running) {
      const messages = await this.client.getCategory(this.config.category, {
        globalPosition: this.position,
        batchSize: this.config.batchSize,
        consumerGroup: {
          member: this.config.member,
          size: this.config.size
        }
      });
      
      if (messages.length > 0) {
        await this.handler(messages);
        
        // Update and save checkpoint
        this.position = messages[messages.length - 1][4] + 1;
        await this.checkpointStore.save({
          position: this.position,
          updatedAt: new Date()
        });
      } else {
        await sleep(this.config.pollingInterval);
      }
    }
  }
  
  stop() {
    this.running = false;
  }
}

// Simple file-based checkpoint store
const fileCheckpointStore = (filename: string) => ({
  async load(): Promise<Checkpoint | null> {
    try {
      const data = await Bun.file(filename).text();
      return JSON.parse(data);
    } catch {
      return null;
    }
  },
  async save(checkpoint: Checkpoint): Promise<void> {
    await Bun.write(filename, JSON.stringify(checkpoint));
  }
});

// Usage
const consumer = new CheckpointedConsumer(
  client,
  { category: 'account', member: 0, size: 4, batchSize: 100, pollingInterval: 1000 },
  async (messages) => { /* handle */ },
  fileCheckpointStore('./consumer-0-checkpoint.json')
);
```

## Consumer with SSE

### Real-time Consumer

```typescript
class RealtimeConsumer {
  private subscription: { close: () => void } | null = null;
  private position = 0;
  
  constructor(
    private client: EventoDBClient,
    private config: ConsumerConfig,
    private handler: (messages: any[]) => Promise<void>
  ) {}
  
  async start() {
    // Load initial position (from checkpoint)
    // this.position = await loadCheckpoint();
    
    // Process existing messages first
    await this.catchUp();
    
    // Subscribe for real-time updates
    this.subscription = this.client.subscribeToCategory(this.config.category, {
      position: this.position,
      consumerGroup: {
        member: this.config.member,
        size: this.config.size
      },
      onPoke: async (poke) => {
        // Fetch and process new messages
        const messages = await this.client.getCategory(this.config.category, {
          globalPosition: this.position,
          batchSize: this.config.batchSize,
          consumerGroup: {
            member: this.config.member,
            size: this.config.size
          }
        });
        
        if (messages.length > 0) {
          await this.handler(messages);
          this.position = messages[messages.length - 1][4] + 1;
        }
      },
      onError: (error) => {
        console.error('Subscription error:', error);
        // Implement reconnection logic
      }
    });
    
    console.log(`Consumer ${this.config.member} subscribed`);
  }
  
  async catchUp() {
    console.log(`Catching up from position ${this.position}`);
    
    while (true) {
      const messages = await this.client.getCategory(this.config.category, {
        globalPosition: this.position,
        batchSize: this.config.batchSize,
        consumerGroup: {
          member: this.config.member,
          size: this.config.size
        }
      });
      
      if (messages.length === 0) break;
      
      await this.handler(messages);
      this.position = messages[messages.length - 1][4] + 1;
    }
    
    console.log(`Caught up to position ${this.position}`);
  }
  
  stop() {
    this.subscription?.close();
  }
}
```

## Scaling Consumers

### Dynamic Scaling

When scaling consumers, temporarily have overlap during transition:

```
Before: 2 consumers (size=2)
┌───────────────┬───────────────┐
│  Consumer 0   │  Consumer 1   │
│  (50% load)   │  (50% load)   │
└───────────────┴───────────────┘

During scale-up: 4 consumers
┌───────────────┬───────────────┐
│ Consumer 0    │ Consumer 1    │
│ (new: size=4) │ (old: size=2) │ ← Overlap during transition
│ Consumer 2    │ Consumer 3    │
│ (new: size=4) │ (new: size=4) │
└───────────────┴───────────────┘

After: 4 consumers (size=4)
┌───────┬───────┬───────┬───────┐
│ C0    │ C1    │ C2    │ C3    │
│ (25%) │ (25%) │ (25%) │ (25%) │
└───────┴───────┴───────┴───────┘
```

### Scaling Script

```typescript
async function scaleConsumers(oldSize: number, newSize: number) {
  console.log(`Scaling from ${oldSize} to ${newSize} consumers`);
  
  // 1. Start new consumers with new size
  const newConsumers = Array.from({ length: newSize }, (_, i) => 
    new Consumer({ member: i, size: newSize })
  );
  
  for (const consumer of newConsumers) {
    await consumer.start();
  }
  
  // 2. Wait for new consumers to catch up
  await waitForCatchUp(newConsumers);
  
  // 3. Stop old consumers
  await stopOldConsumers();
  
  console.log('Scaling complete');
}
```

## Idempotent Processing

Since messages might be reprocessed (on restart, scaling, etc.), ensure idempotency:

```typescript
async function handleMessage(msg: any[], processedIds: Set<string>) {
  const [id, streamName, type, position, globalPosition, data] = msg;
  
  // Skip already processed messages
  if (processedIds.has(id)) {
    console.log(`Skipping already processed message ${id}`);
    return;
  }
  
  // Process message
  await processAccountEvent(streamName, type, data);
  
  // Mark as processed
  processedIds.add(id);
}

// Or use database for tracking
async function handleMessageWithDB(msg: any[], db: Database) {
  const [id] = msg;
  
  // Try to insert processed record
  try {
    await db.run('INSERT INTO processed_messages (id) VALUES (?)', id);
  } catch (error) {
    if (error.code === 'SQLITE_CONSTRAINT') {
      // Already processed
      return;
    }
    throw error;
  }
  
  // Process message
  await processMessage(msg);
}
```

## Monitoring Consumer Groups

### Health Check

```typescript
interface ConsumerHealth {
  member: number;
  size: number;
  position: number;
  lag: number;
  lastProcessed: Date;
  messagesPerSecond: number;
}

async function getConsumerHealth(consumer: CheckpointedConsumer): Promise<ConsumerHealth> {
  // Get latest global position in category
  const latest = await client.getCategory(consumer.category, { batchSize: 1 });
  const latestPosition = latest.length > 0 ? latest[0][4] : 0;
  
  return {
    member: consumer.member,
    size: consumer.size,
    position: consumer.position,
    lag: latestPosition - consumer.position,
    lastProcessed: consumer.lastProcessedAt,
    messagesPerSecond: consumer.throughput
  };
}
```

### Metrics Endpoint

```typescript
app.get('/health', async (req, res) => {
  const health = await getConsumerHealth(consumer);
  
  res.json({
    status: health.lag < 1000 ? 'healthy' : 'lagging',
    consumer: {
      member: health.member,
      size: health.size
    },
    position: health.position,
    lag: health.lag,
    throughput: health.messagesPerSecond
  });
});
```

## Best Practices

1. **Choose appropriate group size**: Start small (2-4), scale based on load
2. **Use checkpointing**: Always persist position for crash recovery
3. **Implement idempotency**: Messages may be reprocessed
4. **Monitor lag**: Alert when consumers fall behind
5. **Handle errors gracefully**: Don't crash on individual message failures
6. **Use descriptive stream IDs**: UUIDs distribute better than sequential IDs

---

## Next Steps

- [Subscriptions](./subscriptions.md) - Real-time notifications via SSE
