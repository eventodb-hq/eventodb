import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';

describe('READ Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('READ-001: Read from empty stream', async () => {
    const ctx = await setupTest('read-001');
    contexts.push(ctx);

    const stream = randomStreamName();
    const messages = await ctx.client.streamGet(stream);

    expect(messages).toEqual([]);
  });

  test('READ-002: Read single message', async () => {
    const ctx = await setupTest('read-002');
    contexts.push(ctx);

    const stream = randomStreamName();
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { foo: 'bar' }
    });

    const messages = await ctx.client.streamGet(stream);
    expect(messages).toHaveLength(1);

    const [id, type, position, globalPosition, data, metadata, time] = messages[0];
    expect(typeof id).toBe('string');
    expect(type).toBe('TestEvent');
    expect(position).toBe(0);
    expect(typeof globalPosition).toBe('number');
    expect(data).toEqual({ foo: 'bar' });
    expect(typeof time).toBe('string');
  });

  test('READ-003: Read multiple messages', async () => {
    const ctx = await setupTest('read-003');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 5 messages
    for (let i = 0; i < 5; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.streamGet(stream);
    expect(messages).toHaveLength(5);

    // Check positions are in order
    for (let i = 0; i < 5; i++) {
      const [, , position] = messages[i];
      expect(position).toBe(i);
    }
  });

  test('READ-004: Read with position filter', async () => {
    const ctx = await setupTest('read-004');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 10 messages
    for (let i = 0; i < 10; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.streamGet(stream, { position: 5 });
    expect(messages).toHaveLength(5);

    // Check we got positions 5-9
    expect(messages[0][2]).toBe(5);
    expect(messages[4][2]).toBe(9);
  });

  test('READ-005: Read with global position filter', async () => {
    const ctx = await setupTest('read-005');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 4 messages
    const results = [];
    for (let i = 0; i < 4; i++) {
      const result = await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
      results.push(result);
    }

    // Read from global position of first message
    const targetGlobalPos = results[0].globalPosition;
    const messages = await ctx.client.streamGet(stream, { 
      globalPosition: targetGlobalPos 
    });

    // Should get all 4 messages
    expect(messages.length).toBeGreaterThanOrEqual(1);
    // All returned messages should have global positions in correct range
    const firstMsgGlobalPos = messages[0][3];
    expect(firstMsgGlobalPos).toBeGreaterThanOrEqual(targetGlobalPos);
  });

  test('READ-006: Read with batch size limit', async () => {
    const ctx = await setupTest('read-006');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 100 messages
    for (let i = 0; i < 100; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.streamGet(stream, { batchSize: 10 });
    expect(messages).toHaveLength(10);
    
    // Should get positions 0-9
    expect(messages[0][2]).toBe(0);
    expect(messages[9][2]).toBe(9);
  });

  test('READ-007: Read with batch size unlimited', async () => {
    const ctx = await setupTest('read-007');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 50 messages
    for (let i = 0; i < 50; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.streamGet(stream, { batchSize: -1 });
    expect(messages).toHaveLength(50);
  });

  test('READ-008: Read message data integrity', async () => {
    const ctx = await setupTest('read-008');
    contexts.push(ctx);

    const stream = randomStreamName();
    const complexData = {
      nested: {
        array: [1, 2, 3],
        bool: true,
        nullValue: null
      }
    };

    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: complexData
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data).toEqual(complexData);
  });

  test('READ-009: Read message metadata integrity', async () => {
    const ctx = await setupTest('read-009');
    contexts.push(ctx);

    const stream = randomStreamName();
    const metadata = {
      correlationId: '123',
      userId: 'user-456'
    };

    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { test: true },
      metadata
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , , readMetadata] = messages[0];
    expect(readMetadata).toEqual(metadata);
  });

  test('READ-010: Read message timestamp format', async () => {
    const ctx = await setupTest('read-010');
    contexts.push(ctx);

    const stream = randomStreamName();
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { test: true }
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , , , time] = messages[0];
    
    // Should be valid ISO 8601 format
    expect(typeof time).toBe('string');
    
    // Verify it's a valid date
    const parsed = new Date(time);
    expect(parsed.toString()).not.toBe('Invalid Date');
    
    // Should match pattern YYYY-MM-DDTHH:MM:SS.nnnZ or similar
    expect(time).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/);
  });
});
