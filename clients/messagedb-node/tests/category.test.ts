import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';

describe('CATEGORY Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('CATEGORY-001: Read from category', async () => {
    const ctx = await setupTest('category-001');
    contexts.push(ctx);

    const category = 'testcat';
    
    // Write messages to 3 different streams in the category
    await ctx.client.streamWrite(`${category}-1`, {
      type: 'Event',
      data: { stream: 1 }
    });
    await ctx.client.streamWrite(`${category}-2`, {
      type: 'Event',
      data: { stream: 2 }
    });
    await ctx.client.streamWrite(`${category}-3`, {
      type: 'Event',
      data: { stream: 3 }
    });

    const messages = await ctx.client.categoryGet(category);
    // Category should return messages from all streams in that category
    expect(messages.length).toBe(3);
  });

  test('CATEGORY-002: Read category with position filter', async () => {
    const ctx = await setupTest('category-002');
    contexts.push(ctx);

    const category = 'testcat2';
    const results = [];
    
    // Write 4 messages
    for (let i = 0; i < 4; i++) {
      const result = await ctx.client.streamWrite(`${category}-${i}`, {
        type: 'Event',
        data: { n: i }
      });
      results.push(result);
    }

    // Read from global position of 3rd message
    const targetGlobalPos = results[2].globalPosition;
    const messages = await ctx.client.categoryGet(category, {
      position: targetGlobalPos
    });

    // Should get messages at or after the target position
    expect(messages.length).toBeGreaterThanOrEqual(1);
    for (const msg of messages) {
      const [, , , , globalPosition] = msg;
      expect(globalPosition).toBeGreaterThanOrEqual(targetGlobalPos);
    }
  });

  test('CATEGORY-003: Read category with batch size', async () => {
    const ctx = await setupTest('category-003');
    contexts.push(ctx);

    const category = 'testcat3';
    
    // Write 50 messages across fewer streams
    for (let i = 0; i < 50; i++) {
      await ctx.client.streamWrite(`${category}-${i % 5}`, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.categoryGet(category, { batchSize: 10 });
    expect(messages.length).toBeLessThanOrEqual(10);
    expect(messages.length).toBeGreaterThan(0);
  });

  test('CATEGORY-004: Category message format', async () => {
    const ctx = await setupTest('category-004');
    contexts.push(ctx);

    const category = 'testcat4';
    await ctx.client.streamWrite(`${category}-123`, {
      type: 'TestEvent',
      data: { foo: 'bar' }
    });

    const messages = await ctx.client.categoryGet(category);
    expect(messages.length).toBeGreaterThanOrEqual(1);

    // Category messages have 8 elements: [id, streamName, type, position, globalPosition, data, metadata, time]
    const message = messages[0];
    expect(message).toHaveLength(8);

    const [id, streamName, type, position, globalPosition, data, metadata, time] = message;
    expect(typeof id).toBe('string');
    expect(streamName).toContain(category);
    expect(typeof type).toBe('string');
    expect(typeof position).toBe('number');
    expect(typeof globalPosition).toBe('number');
    expect(typeof data).toBe('object');
    expect(typeof time).toBe('string');
  });

  test('CATEGORY-005: Category with consumer group', async () => {
    const ctx = await setupTest('category-005');
    contexts.push(ctx);

    const category = 'testcat5';
    
    // Write messages to 4 different streams
    await ctx.client.streamWrite(`${category}-1`, { type: 'Event', data: { s: 1 } });
    await ctx.client.streamWrite(`${category}-2`, { type: 'Event', data: { s: 2 } });
    await ctx.client.streamWrite(`${category}-3`, { type: 'Event', data: { s: 3 } });
    await ctx.client.streamWrite(`${category}-4`, { type: 'Event', data: { s: 4 } });

    // Get messages for consumer group member 0 of 2
    const messages = await ctx.client.categoryGet(category, {
      consumerGroup: {
        member: 0,
        size: 2
      }
    });

    // Should get a subset of messages (deterministic based on stream hash)
    expect(messages.length).toBeGreaterThanOrEqual(1);
    expect(messages.length).toBeLessThanOrEqual(4);
  });

  test('CATEGORY-006: Category with correlation filter', async () => {
    const ctx = await setupTest('category-006');
    contexts.push(ctx);

    const category = 'testcat6';
    
    // Write message with correlation metadata
    await ctx.client.streamWrite(`${category}-1`, {
      type: 'Event',
      data: { n: 1 },
      metadata: { correlationStreamName: 'workflow-123' }
    });
    
    // Write message with different correlation
    await ctx.client.streamWrite(`${category}-2`, {
      type: 'Event',
      data: { n: 2 },
      metadata: { correlationStreamName: 'other-456' }
    });

    const messages = await ctx.client.categoryGet(category, {
      correlation: 'workflow'
    });

    // Should only get messages correlated to 'workflow'
    // Note: correlation filtering might not be supported in test mode
    expect(messages.length).toBeGreaterThanOrEqual(1);
    for (const msg of messages) {
      const [, , , , , , metadata] = msg;
      if (metadata && typeof metadata === 'object' && 'correlationStreamName' in metadata) {
        expect(metadata.correlationStreamName).toContain('workflow');
      }
    }
  });

  test('CATEGORY-007: Read from empty category', async () => {
    const ctx = await setupTest('category-007');
    contexts.push(ctx);

    const category = randomStreamName('nonexistent');
    const messages = await ctx.client.categoryGet(category);
    
    expect(messages).toEqual([]);
  });

  test('CATEGORY-008: Category global position ordering', async () => {
    const ctx = await setupTest('category-008');
    contexts.push(ctx);

    const category = randomStreamName('category');
    
    // Write messages to multiple streams
    for (let i = 0; i < 10; i++) {
      await ctx.client.streamWrite(`${category}-${i % 3}`, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.categoryGet(category);
    
    // Messages should be in ascending global position order
    for (let i = 1; i < messages.length; i++) {
      const prevGlobalPos = messages[i - 1][4];
      const currGlobalPos = messages[i][4];
      expect(currGlobalPos).toBeGreaterThanOrEqual(prevGlobalPos);
    }
  });
});
