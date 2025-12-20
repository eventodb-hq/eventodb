import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';

describe('LAST Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('LAST-001: Last message from non-empty stream', async () => {
    const ctx = await setupTest('last-001');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 5 messages
    for (let i = 0; i < 5; i++) {
      await ctx.client.streamWrite(stream, {
        type: 'Event',
        data: { n: i }
      });
    }

    const lastMessage = await ctx.client.streamLast(stream);
    expect(lastMessage).not.toBe(null);
    
    if (lastMessage) {
      const [, , position, , data] = lastMessage;
      expect(position).toBe(4); // Last position in 0-indexed stream
      expect(data).toEqual({ n: 4 });
    }
  });

  test('LAST-002: Last message from empty stream', async () => {
    const ctx = await setupTest('last-002');
    contexts.push(ctx);

    const stream = randomStreamName();
    const lastMessage = await ctx.client.streamLast(stream);
    
    expect(lastMessage).toBe(null);
  });

  test('LAST-003: Last message filtered by type', async () => {
    const ctx = await setupTest('last-003');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write messages: TypeA, TypeB, TypeA, TypeB, TypeA
    await ctx.client.streamWrite(stream, { type: 'TypeA', data: { pos: 0 } });
    await ctx.client.streamWrite(stream, { type: 'TypeB', data: { pos: 1 } });
    await ctx.client.streamWrite(stream, { type: 'TypeA', data: { pos: 2 } });
    await ctx.client.streamWrite(stream, { type: 'TypeB', data: { pos: 3 } });
    await ctx.client.streamWrite(stream, { type: 'TypeA', data: { pos: 4 } });

    const lastTypeB = await ctx.client.streamLast(stream, { type: 'TypeB' });
    expect(lastTypeB).not.toBe(null);
    
    if (lastTypeB) {
      const [, type, position, , data] = lastTypeB;
      expect(type).toBe('TypeB');
      expect(position).toBe(3);
      expect(data).toEqual({ pos: 3 });
    }
  });

  test('LAST-004: Last message type filter no match', async () => {
    const ctx = await setupTest('last-004');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write only TypeA messages
    await ctx.client.streamWrite(stream, { type: 'TypeA', data: { n: 1 } });
    await ctx.client.streamWrite(stream, { type: 'TypeA', data: { n: 2 } });

    const lastTypeB = await ctx.client.streamLast(stream, { type: 'TypeB' });
    expect(lastTypeB).toBe(null);
  });
});
