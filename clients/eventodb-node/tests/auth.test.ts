import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext, getEventoDBURL } from './helpers.js';
import { EventoDBClient } from '../src/client.js';
import { EventoDBError } from '../src/errors.js';

describe('AUTH Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('AUTH-001: Valid token authentication', async () => {
    const ctx = await setupTest('auth-001');
    contexts.push(ctx);

    const stream = randomStreamName();
    const result = await ctx.client.streamWrite(stream, {
      type: 'TestEvent',
      data: { foo: 'bar' }
    });

    expect(result.position).toBeGreaterThanOrEqual(0);
  });

  test.skip('AUTH-002: Missing token (skipped in test mode)', async () => {
    // Note: In test mode, server auto-creates namespaces
    // This test would only work in production mode
    const client = new EventoDBClient(getEventoDBURL());
    const stream = randomStreamName();

    await expect(
      client.streamWrite(stream, {
        type: 'TestEvent',
        data: { foo: 'bar' }
      })
    ).rejects.toThrow(EventoDBError);
  });

  test.skip('AUTH-003: Invalid token format (skipped in test mode)', async () => {
    // Note: In test mode, server auto-creates namespaces
    // This test would only work in production mode
    const client = new EventoDBClient(getEventoDBURL(), {
      token: 'invalid-token'
    });
    const stream = randomStreamName();

    await expect(
      client.streamWrite(stream, {
        type: 'TestEvent',
        data: { foo: 'bar' }
      })
    ).rejects.toThrow(EventoDBError);
  });

  test('AUTH-004: Token namespace isolation', async () => {
    const ctx1 = await setupTest('auth-004-ns1');
    const ctx2 = await setupTest('auth-004-ns2');
    contexts.push(ctx1, ctx2);

    const stream = 'shared-stream-name';
    
    // Write message to stream in namespace 1
    await ctx1.client.streamWrite(stream, {
      type: 'NS1Event',
      data: { namespace: 1 }
    });

    // Try to read same stream name using namespace 2 token
    const messages = await ctx2.client.streamGet(stream);
    
    // Should be empty - different namespace
    expect(messages).toEqual([]);
  });
});
