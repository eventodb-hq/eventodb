import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';
import { MessageDBError } from '../src/errors.js';

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
    expect(result.position).toBe(0); // First message should be at position 0
  });

  test('WRITE-002: Write message with metadata', async () => {
    const ctx = await setupTest('write-002');
    contexts.push(ctx);

    const stream = randomStreamName();
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { foo: 'bar' },
      metadata: { correlationId: '123' }
    });

    const messages = await ctx.client.streamGet(stream);
    expect(messages).toHaveLength(1);
    const [, , , , , metadata] = messages[0];
    expect(metadata).toEqual({ correlationId: '123' });
  });

  test('WRITE-003: Write with custom message ID', async () => {
    const ctx = await setupTest('write-003');
    contexts.push(ctx);

    const stream = randomStreamName();
    const customUuid = '550e8400-e29b-41d4-a716-446655440000';
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { foo: 'bar' }
    }, { id: customUuid });

    const messages = await ctx.client.streamGet(stream);
    expect(messages).toHaveLength(1);
    const [id] = messages[0];
    expect(id).toBe(customUuid);
  });

  test('WRITE-004: Write with expected version (success)', async () => {
    const ctx = await setupTest('write-004');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 2 messages
    await ctx.client.streamWrite(stream, { type: 'Event1', data: { n: 1 } });
    await ctx.client.streamWrite(stream, { type: 'Event2', data: { n: 2 } });

    // Write with expectedVersion = 1 (last position)
    const result = await ctx.client.streamWrite(stream, {
      type: 'Event3',
      data: { n: 3 }
    }, { expectedVersion: 1 });

    expect(result.position).toBe(2);
  });

  test('WRITE-005: Write with expected version (conflict)', async () => {
    const ctx = await setupTest('write-005');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 2 messages
    await ctx.client.streamWrite(stream, { type: 'Event1', data: { n: 1 } });
    await ctx.client.streamWrite(stream, { type: 'Event2', data: { n: 2 } });

    // Try to write with wrong expectedVersion
    await expect(
      ctx.client.streamWrite(stream, {
        type: 'Event3',
        data: { n: 3 }
      }, { expectedVersion: 5 })
    ).rejects.toThrow();
  });

  test('WRITE-006: Write multiple messages sequentially', async () => {
    const ctx = await setupTest('write-006');
    contexts.push(ctx);

    const stream = randomStreamName();
    const results = [];

    // Write 5 messages
    for (let i = 0; i < 5; i++) {
      const result = await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
      results.push(result);
    }

    // Check positions are sequential
    expect(results[0].position).toBe(0);
    expect(results[1].position).toBe(1);
    expect(results[2].position).toBe(2);
    expect(results[3].position).toBe(3);
    expect(results[4].position).toBe(4);

    // Check global positions are monotonically increasing
    for (let i = 1; i < 5; i++) {
      expect(results[i].globalPosition).toBeGreaterThan(results[i - 1].globalPosition);
    }
  });

  test('WRITE-007: Write to stream with ID', async () => {
    const ctx = await setupTest('write-007');
    contexts.push(ctx);

    const stream = 'account-123';
    const result = await ctx.client.streamWrite(stream, {
      type: 'Deposited',
      data: { amount: 100 }
    });

    expect(result.position).toBeGreaterThanOrEqual(0);
  });

  test('WRITE-008: Write with empty data object', async () => {
    const ctx = await setupTest('write-008');
    contexts.push(ctx);

    const stream = randomStreamName();
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: {}
    });

    const messages = await ctx.client.streamGet(stream);
    expect(messages).toHaveLength(1);
    const [, , , , data] = messages[0];
    expect(data).toEqual({});
  });

  test('WRITE-009: Write with null metadata', async () => {
    const ctx = await setupTest('write-009');
    contexts.push(ctx);

    const stream = randomStreamName();
    // Note: Server may not accept null metadata, omit metadata field instead
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { x: 1 }
      // metadata omitted (will be null/undefined in result)
    });

    const messages = await ctx.client.streamGet(stream);
    expect(messages).toHaveLength(1);
    const [, , , , , metadata] = messages[0];
    // Metadata should be null when not provided
    expect(metadata === null || metadata === undefined).toBe(true);
  });
});
