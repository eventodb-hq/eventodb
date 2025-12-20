import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext } from './helpers.js';

describe('VERSION Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('VERSION-001: Version of non-existent stream', async () => {
    const ctx = await setupTest('version-001');
    contexts.push(ctx);

    const stream = randomStreamName();
    const version = await ctx.client.streamVersion(stream);
    
    expect(version).toBe(null);
  });

  test('VERSION-002: Version of stream with messages', async () => {
    const ctx = await setupTest('version-002');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 3 messages (positions 0, 1, 2)
    await ctx.client.streamWrite(stream, { type: 'Event', data: { n: 1 } });
    await ctx.client.streamWrite(stream, { type: 'Event', data: { n: 2 } });
    await ctx.client.streamWrite(stream, { type: 'Event', data: { n: 3 } });

    const version = await ctx.client.streamVersion(stream);
    expect(version).toBe(2); // Last position (0-indexed)
  });

  test('VERSION-003: Version after write', async () => {
    const ctx = await setupTest('version-003');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // Write 1 message (position 0)
    await ctx.client.streamWrite(stream, { type: 'Event', data: { n: 1 } });
    
    // Write another message (position 1)
    await ctx.client.streamWrite(stream, { type: 'Event', data: { n: 2 } });
    
    const version = await ctx.client.streamVersion(stream);
    expect(version).toBe(1);
  });
});
