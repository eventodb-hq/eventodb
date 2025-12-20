import { describe, test, expect, afterEach } from 'vitest';
import { setupTest, randomStreamName, type TestContext, getEventoDBURL } from './helpers.js';
import { EventoDBClient } from '../src/client.js';
import { NetworkError, EventoDBError } from '../src/errors.js';

describe('ERROR Tests', () => {
  const contexts: TestContext[] = [];

  afterEach(async () => {
    await Promise.all(contexts.map(ctx => ctx.cleanup()));
    contexts.length = 0;
  });

  test('ERROR-002: Missing required argument', async () => {
    const ctx = await setupTest('error-002');
    contexts.push(ctx);

    const stream = randomStreamName();
    
    // TypeScript should catch this, but test runtime behavior
    await expect(
      // @ts-expect-error Testing missing argument
      ctx.client.streamWrite(stream)
    ).rejects.toThrow();
  });

  test('ERROR-003: Invalid stream name type', async () => {
    const ctx = await setupTest('error-003');
    contexts.push(ctx);

    // TypeScript should catch this, but test runtime behavior
    await expect(
      // @ts-expect-error Testing invalid type
      ctx.client.streamWrite(123, { type: 'Test', data: {} })
    ).rejects.toThrow();
  });

  test('ERROR-004: Connection refused', async () => {
    const client = new EventoDBClient('http://localhost:99999');
    const stream = randomStreamName();

    await expect(
      client.streamWrite(stream, {
        type: 'TestEvent',
        data: { foo: 'bar' }
      })
    ).rejects.toThrow(NetworkError);
  });
});
