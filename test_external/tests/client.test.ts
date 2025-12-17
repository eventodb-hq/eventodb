/**
 * External Tests for MessageDB
 * 
 * Each test gets its own unique namespace - no shared state.
 */

import { test, expect, describe } from 'bun:test';
import { setupTest, randomStreamName, type TestContext } from '../lib';

describe('Stream Operations', () => {
  test('write and read a message', async () => {
    const t = await setupTest('write and read a message');
    try {
      const stream = randomStreamName('account');

      const result = await t.client.writeMessage(stream, {
        type: 'AccountOpened',
        data: { balance: 0 }
      });

      expect(result.position).toBe(0);

      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(1);
      expect(messages[0][1]).toBe('AccountOpened');
    } finally {
      await t.cleanup();
    }
  });

  test('write multiple messages to same stream', async () => {
    const t = await setupTest('write multiple messages');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
      await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
      await t.client.writeMessage(stream, { type: 'Event3', data: { n: 3 } });

      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(3);
    } finally {
      await t.cleanup();
    }
  });

  test('expected version succeeds when correct', async () => {
    const t = await setupTest('expected version succeeds');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Event1', data: {} });

      const result = await t.client.writeMessage(stream, {
        type: 'Event2',
        data: {}
      }, { expectedVersion: 0 });

      expect(result.position).toBe(1);
    } finally {
      await t.cleanup();
    }
  });

  test('expected version fails when wrong', async () => {
    const t = await setupTest('expected version fails');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Event1', data: {} });

      try {
        await t.client.writeMessage(stream, {
          type: 'Event2',
          data: {}
        }, { expectedVersion: 10 });
        expect(true).toBe(false);
      } catch (error: any) {
        expect(error.message).toContain('VERSION');
      }
    } finally {
      await t.cleanup();
    }
  });

  test('get last message from stream', async () => {
    const t = await setupTest('get last message');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
      await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
      await t.client.writeMessage(stream, { type: 'Last', data: { n: 3 } });

      const last = await t.client.getLastMessage(stream);
      expect(last[1]).toBe('Last');
    } finally {
      await t.cleanup();
    }
  });

  test('get stream version', async () => {
    const t = await setupTest('get stream version');
    try {
      const stream = randomStreamName('account');

      const v1 = await t.client.getStreamVersion(stream);
      expect(v1).toBeNull();

      await t.client.writeMessage(stream, { type: 'Event1', data: {} });
      const v2 = await t.client.getStreamVersion(stream);
      expect(v2).toBe(0);

      await t.client.writeMessage(stream, { type: 'Event2', data: {} });
      const v3 = await t.client.getStreamVersion(stream);
      expect(v3).toBe(1);
    } finally {
      await t.cleanup();
    }
  });

  test('read from position', async () => {
    const t = await setupTest('read from position');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
      await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
      await t.client.writeMessage(stream, { type: 'Event3', data: { n: 3 } });

      const messages = await t.client.getStream(stream, { position: 1 });
      expect(messages.length).toBe(2);
    } finally {
      await t.cleanup();
    }
  });
});

describe('Category Operations', () => {
  test('read messages from category', async () => {
    const t = await setupTest('read messages from category');
    try {
      const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

      await t.client.writeMessage(`${category}-1`, { type: 'Event', data: { stream: 1 } });
      await t.client.writeMessage(`${category}-2`, { type: 'Event', data: { stream: 2 } });
      await t.client.writeMessage(`${category}-3`, { type: 'Event', data: { stream: 3 } });

      const messages = await t.client.getCategory(category);
      expect(messages.length).toBe(3);
    } finally {
      await t.cleanup();
    }
  });

  test('category read respects batch size', async () => {
    const t = await setupTest('category batch size');
    try {
      const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

      for (let i = 0; i < 10; i++) {
        await t.client.writeMessage(`${category}-${i}`, { type: 'Event', data: { i } });
      }

      const messages = await t.client.getCategory(category, { batchSize: 3 });
      expect(messages.length).toBe(3);
    } finally {
      await t.cleanup();
    }
  });
});

describe('Concurrent Writes', () => {
  test('concurrent writes to different streams', async () => {
    const t = await setupTest('concurrent different streams');
    try {
      const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

      const writes = Array.from({ length: 50 }, (_, i) =>
        t.client.writeMessage(`${category}-${i}`, {
          type: 'Event',
          data: { index: i }
        })
      );

      const results = await Promise.all(writes);

      expect(results.length).toBe(50);
      results.forEach(r => {
        expect(r.position).toBe(0);
      });
    } finally {
      await t.cleanup();
    }
  });

  test('concurrent writes with optimistic locking', async () => {
    const t = await setupTest('concurrent optimistic locking');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Init', data: {} });

      const writes = Array.from({ length: 10 }, () =>
        t.client.writeMessage(stream, {
          type: 'Update',
          data: {}
        }, { expectedVersion: 0 })
      );

      const results = await Promise.allSettled(writes);

      const succeeded = results.filter(r => r.status === 'fulfilled');
      const failed = results.filter(r => r.status === 'rejected');

      expect(succeeded.length).toBe(1);
      expect(failed.length).toBe(9);
    } finally {
      await t.cleanup();
    }
  });
});
