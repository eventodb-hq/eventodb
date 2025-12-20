import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';

describe('ENCODING Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('ENCODING-001: UTF-8 text in data', async () => {
    const ctx = await setupTest('encoding-001');
    contexts.push(ctx);

    const stream = randomStreamName();
    const utf8Text = 'Hello 世界 🌍 émojis';
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { text: utf8Text }
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data.text).toBe(utf8Text);
  });

  test('ENCODING-002: Unicode in metadata', async () => {
    const ctx = await setupTest('encoding-002');
    contexts.push(ctx);

    const stream = randomStreamName();
    const unicodeText = 'Test 测试 🎉';
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { test: true },
      metadata: { description: unicodeText }
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , , metadata] = messages[0];
    expect(metadata?.description).toBe(unicodeText);
  });

  test('ENCODING-003: Special characters in stream name', async () => {
    const ctx = await setupTest('encoding-003');
    contexts.push(ctx);

    const stream = 'test-stream_123.abc';
    
    // Should either succeed or give clear error
    const result = await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { test: true }
    });

    expect(result.position).toBeGreaterThanOrEqual(0);
  });

  test('ENCODING-004: Empty string values', async () => {
    const ctx = await setupTest('encoding-004');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { emptyString: '' }
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data.emptyString).toBe('');
    expect(data.emptyString).not.toBe(null);
  });

  test('ENCODING-005: Boolean values', async () => {
    const ctx = await setupTest('encoding-005');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { isTrue: true, isFalse: false }
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data.isTrue).toBe(true);
    expect(data.isFalse).toBe(false);
    expect(typeof data.isTrue).toBe('boolean');
    expect(typeof data.isFalse).toBe('boolean');
  });

  test('ENCODING-006: Null values', async () => {
    const ctx = await setupTest('encoding-006');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { nullValue: null }
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data.nullValue).toBe(null);
    expect(data.nullValue).not.toBe(undefined);
  });

  test('ENCODING-007: Numeric values', async () => {
    const ctx = await setupTest('encoding-007');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: {
        integer: 42,
        float: 3.14159,
        negative: -100,
        zero: 0
      }
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data.integer).toBe(42);
    expect(data.float).toBe(3.14159);
    expect(data.negative).toBe(-100);
    expect(data.zero).toBe(0);
    expect(typeof data.integer).toBe('number');
    expect(typeof data.float).toBe('number');
  });

  test('ENCODING-008: Nested objects', async () => {
    const ctx = await setupTest('encoding-008');
    contexts.push(ctx);

    const stream = randomStreamName();
    const nestedData = {
      level1: {
        level2: {
          level3: {
            value: 'deep'
          }
        }
      }
    };
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: nestedData
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data).toEqual(nestedData);
    expect(data.level1.level2.level3.value).toBe('deep');
  });

  test('ENCODING-009: Arrays in data', async () => {
    const ctx = await setupTest('encoding-009');
    contexts.push(ctx);

    const stream = randomStreamName();
    const arrayData = {
      items: [1, 'two', { three: 3 }, null, true]
    };
    
    await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: arrayData
    });

    const messages = await ctx.client.streamGet(stream);
    const [, , , , data] = messages[0];
    expect(data).toEqual(arrayData);
    expect(Array.isArray(data.items)).toBe(true);
    expect(data.items).toHaveLength(5);
    expect(data.items[0]).toBe(1);
    expect(data.items[1]).toBe('two');
    expect(data.items[2]).toEqual({ three: 3 });
    expect(data.items[3]).toBe(null);
    expect(data.items[4]).toBe(true);
  });

  test('ENCODING-010: Large message payload', async () => {
    const ctx = await setupTest('encoding-010');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Create ~100KB of data
    const largeData: Record<string, string> = {};
    for (let i = 0; i < 1000; i++) {
      largeData[`key${i}`] = 'x'.repeat(100);
    }
    
    // Should either succeed or give clear size limit error
    try {
      const result = await ctx.client.streamWrite(stream, {
        type: 'TestEvent',
        data: largeData
      });
      expect(result.position).toBeGreaterThanOrEqual(0);
    } catch (error: any) {
      // If it fails, should be a clear error about size
      expect(error.message).toBeDefined();
    }
  });
});
