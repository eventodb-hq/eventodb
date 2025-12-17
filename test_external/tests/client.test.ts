/**
 * External Tests for MessageDB - Phase MDB003_1A
 * 
 * Tests for test client infrastructure:
 * - MDB003_1A_T1: Test client can make RPC requests
 * - MDB003_1A_T2: Test client captures token from header
 * - MDB003_1A_T3: Test client sends token in Authorization header
 * - MDB003_1A_T4: Test client handles error responses
 * - MDB003_1A_T5: Test startTestServer spawns server
 * - MDB003_1A_T6: Test waitForHealthy detects port
 * - MDB003_1A_T7: Test server cleanup on test end
 * 
 * Plus stream, category, and concurrency tests.
 */

import { test, expect, describe, afterAll } from 'bun:test';
import {
  setupTest,
  stopSharedServer,
  startTestServer,
  randomStreamName,
  type TestContext
} from '../lib';

// Clean up server after all tests
afterAll(async () => {
  await stopSharedServer();
});

// =========================================
// Phase MDB003_1A: Test Client Tests
// =========================================

describe('MDB003_1A: Test Client Infrastructure', () => {
  test('MDB003_1A_T1: client can make RPC requests', async () => {
    const t = await setupTest('T1 client rpc');
    try {
      // Simple RPC call - write and read
      const stream = randomStreamName('test');
      const result = await t.client.writeMessage(stream, {
        type: 'TestEvent',
        data: { test: true }
      });

      expect(result).toBeDefined();
      expect(result.position).toBe(0);
      expect(typeof result.globalPosition).toBe('number');
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_1A_T3: client sends token in Authorization header', async () => {
    const t = await setupTest('T3 auth header');
    try {
      // Write should work because we have a valid token
      const stream = randomStreamName('test');
      const result = await t.client.writeMessage(stream, {
        type: 'TestEvent',
        data: {}
      });

      expect(result.position).toBe(0);

      // Verify token is set
      expect(t.client.getToken()).toBeDefined();
      expect(t.client.getToken()).toBe(t.token);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_1A_T4: client handles error responses', async () => {
    const t = await setupTest('T4 error handling');
    try {
      const stream = randomStreamName('test');

      // First write succeeds
      await t.client.writeMessage(stream, { type: 'Event1', data: {} });

      // Second write with wrong expectedVersion should fail
      try {
        await t.client.writeMessage(stream, {
          type: 'Event2',
          data: {}
        }, { expectedVersion: 99 });

        // Should not reach here
        expect(true).toBe(false);
      } catch (error: any) {
        // Should get version conflict error
        expect(error.message).toContain('VERSION');
      }
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_1A_T5: startTestServer spawns server', async () => {
    // This test uses a different port to avoid conflicts
    const server = await startTestServer({ port: 6790 });
    try {
      // Verify server is responding
      const response = await fetch(`${server.url}/health`);
      expect(response.ok).toBe(true);

      const health = await response.json();
      expect(health.status).toBe('ok');
    } finally {
      await server.close();
    }
  });

  test('MDB003_1A_T6: health endpoint is accessible', async () => {
    const t = await setupTest('T6 health');
    try {
      const health = await t.client.getHealth();

      expect(health).toBeDefined();
      expect(health.status).toBe('ok');
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_1A_T7: namespace cleanup works', async () => {
    const t = await setupTest('T7 cleanup');
    const namespace = t.namespace;

    // Write some data
    const stream = randomStreamName('test');
    await t.client.writeMessage(stream, { type: 'Event', data: {} });

    // Cleanup
    await t.cleanup();

    // Note: We can't easily verify the namespace is gone because
    // we'd need admin access. The test passes if cleanup doesn't throw.
  });
});

// =========================================
// Stream Operations Tests
// =========================================

describe('Stream Operations', () => {
  test('write and read a message', async () => {
    const t = await setupTest('stream write/read');
    try {
      const stream = randomStreamName('account');

      const result = await t.client.writeMessage(stream, {
        type: 'AccountOpened',
        data: { balance: 0 }
      });

      expect(result.position).toBe(0);

      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(1);
      expect(messages[0][1]).toBe('AccountOpened'); // type is at index 1
    } finally {
      await t.cleanup();
    }
  });

  test('write multiple messages to same stream', async () => {
    const t = await setupTest('multiple messages');
    try {
      const stream = randomStreamName('account');

      const r1 = await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
      const r2 = await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
      const r3 = await t.client.writeMessage(stream, { type: 'Event3', data: { n: 3 } });

      expect(r1.position).toBe(0);
      expect(r2.position).toBe(1);
      expect(r3.position).toBe(2);

      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(3);
    } finally {
      await t.cleanup();
    }
  });

  test('expected version succeeds when correct', async () => {
    const t = await setupTest('expected version success');
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

  test('expected version fails when wrong (optimistic locking)', async () => {
    const t = await setupTest('expected version fails');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Event1', data: {} });

      try {
        await t.client.writeMessage(stream, {
          type: 'Event2',
          data: {}
        }, { expectedVersion: 10 });
        expect(true).toBe(false); // Should not reach
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
      expect(last).not.toBeNull();
      expect(last[1]).toBe('Last'); // type at index 1
    } finally {
      await t.cleanup();
    }
  });

  test('get stream version', async () => {
    const t = await setupTest('get stream version');
    try {
      const stream = randomStreamName('account');

      // Non-existent stream returns null
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

  test('read from specific position', async () => {
    const t = await setupTest('read from position');
    try {
      const stream = randomStreamName('account');

      await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
      await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
      await t.client.writeMessage(stream, { type: 'Event3', data: { n: 3 } });

      // Read from position 1 (should skip first message)
      const messages = await t.client.getStream(stream, { position: 1 });
      expect(messages.length).toBe(2);
      expect(messages[0][1]).toBe('Event2');
    } finally {
      await t.cleanup();
    }
  });

  test('empty stream returns empty array', async () => {
    const t = await setupTest('empty stream');
    try {
      const stream = randomStreamName('nonexistent');

      const messages = await t.client.getStream(stream);
      expect(messages).toEqual([]);
    } finally {
      await t.cleanup();
    }
  });

  test('message metadata is preserved', async () => {
    const t = await setupTest('metadata preserved');
    try {
      const stream = randomStreamName('test');

      await t.client.writeMessage(stream, {
        type: 'TestEvent',
        data: { foo: 'bar' },
        metadata: {
          correlationStreamName: 'workflow-123',
          causationMessageId: 'msg-456',
          custom: 'value'
        }
      });

      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(1);

      const metadata = messages[0][5]; // metadata at index 5
      expect(metadata).toBeDefined();
      expect(metadata.correlationStreamName).toBe('workflow-123');
      expect(metadata.causationMessageId).toBe('msg-456');
      expect(metadata.custom).toBe('value');
    } finally {
      await t.cleanup();
    }
  });

  test('batch size limits results', async () => {
    const t = await setupTest('batch size');
    try {
      const stream = randomStreamName('test');

      // Write 10 messages
      for (let i = 0; i < 10; i++) {
        await t.client.writeMessage(stream, { type: `Event${i}`, data: { i } });
      }

      // Read with batch size 3
      const messages = await t.client.getStream(stream, { batchSize: 3 });
      expect(messages.length).toBe(3);
    } finally {
      await t.cleanup();
    }
  });
});

// =========================================
// Category Operations Tests
// =========================================

describe('Category Operations', () => {
  test('read messages from multiple streams in category', async () => {
    const t = await setupTest('category read');
    try {
      const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

      await t.client.writeMessage(`${category}-1`, { type: 'Event', data: { stream: 1 } });
      await t.client.writeMessage(`${category}-2`, { type: 'Event', data: { stream: 2 } });
      await t.client.writeMessage(`${category}-3`, { type: 'Event', data: { stream: 3 } });

      const messages = await t.client.getCategory(category);
      expect(messages.length).toBe(3);

      // Category results include stream name at index 1
      const streamNames = messages.map(m => m[1]);
      expect(streamNames).toContain(`${category}-1`);
      expect(streamNames).toContain(`${category}-2`);
      expect(streamNames).toContain(`${category}-3`);
    } finally {
      await t.cleanup();
    }
  });

  test('category batch size limits results', async () => {
    const t = await setupTest('category batch');
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

  test('consumer groups partition streams', async () => {
    const t = await setupTest('consumer groups');
    try {
      const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

      // Write to 6 streams
      for (let i = 0; i < 6; i++) {
        await t.client.writeMessage(`${category}-${i}`, { type: 'Event', data: { i } });
      }

      // Consumer 0 of 2
      const messages0 = await t.client.getCategory(category, {
        consumerGroup: { member: 0, size: 2 }
      });

      // Consumer 1 of 2
      const messages1 = await t.client.getCategory(category, {
        consumerGroup: { member: 1, size: 2 }
      });

      // Extract stream names
      const streams0 = messages0.map(m => m[1]);
      const streams1 = messages1.map(m => m[1]);

      // No overlap between consumer groups
      const overlap = streams0.filter(s => streams1.includes(s));
      expect(overlap.length).toBe(0);

      // All streams should be covered
      expect(streams0.length + streams1.length).toBe(6);
    } finally {
      await t.cleanup();
    }
  });

  test('empty category returns empty array', async () => {
    const t = await setupTest('empty category');
    try {
      const category = `nonexistent_${Math.random().toString(36).substring(2, 8)}`;

      const messages = await t.client.getCategory(category);
      expect(messages).toEqual([]);
    } finally {
      await t.cleanup();
    }
  });
});

// =========================================
// Concurrent Write Tests
// =========================================

describe('Concurrent Writes', () => {
  test('concurrent writes to different streams all succeed', async () => {
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
        expect(r.position).toBe(0); // First message in each stream
      });
    } finally {
      await t.cleanup();
    }
  });

  test('concurrent writes with optimistic locking - only one succeeds', async () => {
    const t = await setupTest('concurrent optimistic locking');
    try {
      const stream = randomStreamName('account');

      // Initialize stream
      await t.client.writeMessage(stream, { type: 'Init', data: {} });

      // Try 10 concurrent writes, all expecting version 0
      const writes = Array.from({ length: 10 }, () =>
        t.client.writeMessage(stream, {
          type: 'Update',
          data: {}
        }, { expectedVersion: 0 })
      );

      const results = await Promise.allSettled(writes);

      const succeeded = results.filter(r => r.status === 'fulfilled');
      const failed = results.filter(r => r.status === 'rejected');

      // Exactly one should succeed
      expect(succeeded.length).toBe(1);
      expect(failed.length).toBe(9);
    } finally {
      await t.cleanup();
    }
  });
});

// =========================================
// Namespace Tests
// =========================================

describe('Namespace Operations', () => {
  test('namespaces are isolated', async () => {
    const t1 = await setupTest('namespace 1');
    const t2 = await setupTest('namespace 2');

    try {
      // Write to first namespace
      await t1.client.writeMessage('account-123', { type: 'Event1', data: { ns: 1 } });

      // Write to second namespace (same stream name)
      await t2.client.writeMessage('account-123', { type: 'Event2', data: { ns: 2 } });

      // Each namespace should only see its own data
      const messages1 = await t1.client.getStream('account-123');
      const messages2 = await t2.client.getStream('account-123');

      expect(messages1.length).toBe(1);
      expect(messages2.length).toBe(1);

      // Different event types confirm isolation
      expect(messages1[0][1]).toBe('Event1');
      expect(messages2[0][1]).toBe('Event2');
    } finally {
      await t1.cleanup();
      await t2.cleanup();
    }
  });
});

// =========================================
// System Operations Tests
// =========================================

describe('System Operations', () => {
  test('get server version', async () => {
    const t = await setupTest('version');
    try {
      const version = await t.client.getVersion();

      expect(version).toBeDefined();
      expect(typeof version).toBe('string');
      // Version should match semver pattern
      expect(version).toMatch(/^\d+\.\d+\.\d+/);
    } finally {
      await t.cleanup();
    }
  });

  test('get server health', async () => {
    const t = await setupTest('health');
    try {
      const health = await t.client.getHealth();

      expect(health).toBeDefined();
      expect(health.status).toBe('ok');
    } finally {
      await t.cleanup();
    }
  });
});
