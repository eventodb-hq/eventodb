import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';

describe('EDGE Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('EDGE-001: Empty batch size behavior', async () => {
    const ctx = await setupTest('edge-001');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 10 messages
    for (let i = 0; i < 10; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.streamGet(stream, { batchSize: 0 });
    // Should return empty array or handle gracefully
    expect(Array.isArray(messages)).toBe(true);
  });

  test('EDGE-002: Negative position', async () => {
    const ctx = await setupTest('edge-002');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    await ctx.client.streamWrite(stream, {
      type: 'Event',
      data: { n: 1 }
    });

    // Should either error or return empty/all messages
    const messages = await ctx.client.streamGet(stream, { position: -1 });
    expect(Array.isArray(messages)).toBe(true);
  });

  test('EDGE-003: Very large batch size', async () => {
    const ctx = await setupTest('edge-003');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 5 messages
    for (let i = 0; i < 5; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    // Server validates batchSize <= 10000
    // Test with a large but valid batch size
    const messages = await ctx.client.streamGet(stream, { batchSize: 10000 });
    // Should return all 5 messages (server caps at 10000)
    expect(messages.length).toBe(5);
    
    // Test that exceeding limit gives clear error
    await expect(
      ctx.client.streamGet(stream, { batchSize: 1000000 })
    ).rejects.toThrow(/batchSize/i);
  });

  test('EDGE-004: Stream name edge cases', async () => {
    const ctx = await setupTest('edge-004');
    contexts.push(ctx);

    const testNames = [
      'a',
      'stream-with-many-dashes',
      'stream123',
      'UPPERCASE'
    ];

    for (const name of testNames) {
      const result = await ctx.client.streamWrite(name, {
        type: 'TestEvent',
        data: { streamName: name }
      });
      expect(result.position).toBeGreaterThanOrEqual(0);
    }
  });

  test('EDGE-005: Concurrent writes to same stream', async () => {
    const ctx = await setupTest('edge-005');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 10 messages concurrently
    const promises = [];
    for (let i = 0; i < 10; i++) {
      promises.push(
        ctx.client.streamWrite(stream, {
          type: 'Event',
          data: { n: i }
        })
      );
    }

    const results = await Promise.all(promises);
    
    // All should succeed
    expect(results).toHaveLength(10);
    
    // Positions should be unique
    const positions = results.map(r => r.position);
    const uniquePositions = new Set(positions);
    expect(uniquePositions.size).toBe(10);
    
    // Global positions should be monotonically increasing when sorted
    const globalPositions = results.map(r => r.globalPosition).sort((a, b) => a - b);
    for (let i = 1; i < globalPositions.length; i++) {
      expect(globalPositions[i]).toBeGreaterThan(globalPositions[i - 1]);
    }
  });

  test('EDGE-006: Read from position beyond stream end', async () => {
    const ctx = await setupTest('edge-006');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 5 messages (positions 0-4)
    for (let i = 0; i < 5; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    const messages = await ctx.client.streamGet(stream, { position: 100 });
    expect(messages).toEqual([]);
  });

  test('EDGE-007: Expected version -1 (no stream)', async () => {
    const ctx = await setupTest('edge-007');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Writing to non-existent stream with expectedVersion: -1 should succeed
    const result = await ctx.client.streamWrite(stream, {
      type: 'Event',
      data: { n: 1 }
    }, { expectedVersion: -1 });

    expect(result.position).toBe(0);

    // Now stream exists, writing with expectedVersion: -1 should fail
    await expect(
      ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: 2 }
      }, { expectedVersion: -1 })
    ).rejects.toThrow();
  });

  test('EDGE-008: Expected version 0 (first message)', async () => {
    const ctx = await setupTest('edge-008');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Stream doesn't exist yet, version should be -1
    // Writing with expectedVersion: 0 should fail
    await expect(
      ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: 1 }
      }, { expectedVersion: 0 })
    ).rejects.toThrow();
  });
});
