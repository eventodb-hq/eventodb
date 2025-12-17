/**
 * External Tests for MessageDB - Phase MDB003_2A
 * 
 * Stream Operation Tests:
 * - MDB003_2A_T1: Test write and read message
 * - MDB003_2A_T2: Test write returns correct position
 * - MDB003_2A_T3: Test write with expectedVersion succeeds
 * - MDB003_2A_T4: Test optimistic locking prevents conflicts
 * - MDB003_2A_T5: Test get with position filter
 * - MDB003_2A_T6: Test get with batchSize limit
 * - MDB003_2A_T7: Test last message retrieval
 * - MDB003_2A_T8: Test stream version
 * - MDB003_2A_T9: Test empty stream returns empty array
 * - MDB003_2A_T10: Test message metadata preserved
 */

import { test, expect, describe } from 'bun:test';
import { 
  setupTest, 
  randomStreamName, 
  type TestContext 
} from '../lib';

// NOTE: Server cleanup is handled by client.test.ts (shared server pattern)
// Each test cleans up its own namespace via t.cleanup()

// =========================================
// Phase MDB003_2A: Stream Operation Tests
// =========================================

describe('MDB003_2A: Stream Operations', () => {
  
  test('MDB003_2A_T1: write and read message', async () => {
    const t = await setupTest('T1 write read');
    try {
      const stream = randomStreamName('account');

      // Write a message
      const writeResult = await t.client.writeMessage(stream, {
        type: 'AccountOpened',
        data: { balance: 0, owner: 'Alice' }
      });

      expect(writeResult.position).toBe(0);
      expect(typeof writeResult.globalPosition).toBe('number');

      // Read the message back
      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(1);
      
      // Message format: [id, type, position, globalPosition, data, metadata, time]
      // Verify message structure
      const msg = messages[0];
      expect(msg[1]).toBe('AccountOpened'); // type is at index 1
      expect(msg[4]).toEqual({ balance: 0, owner: 'Alice' }); // data is at index 4
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T2: write returns correct position', async () => {
    const t = await setupTest('T2 positions');
    try {
      const stream = randomStreamName('account');

      // Write multiple messages and verify positions
      const r1 = await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
      expect(r1.position).toBe(0);

      const r2 = await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
      expect(r2.position).toBe(1);

      const r3 = await t.client.writeMessage(stream, { type: 'Event3', data: { n: 3 } });
      expect(r3.position).toBe(2);

      // Global positions should be increasing
      expect(r2.globalPosition).toBeGreaterThan(r1.globalPosition);
      expect(r3.globalPosition).toBeGreaterThan(r2.globalPosition);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T3: write with expectedVersion succeeds', async () => {
    const t = await setupTest('T3 expected version success');
    try {
      const stream = randomStreamName('account');

      // First write
      await t.client.writeMessage(stream, { type: 'Event1', data: {} });

      // Second write with correct expectedVersion
      const result = await t.client.writeMessage(stream, {
        type: 'Event2',
        data: {}
      }, { expectedVersion: 0 });

      expect(result.position).toBe(1);

      // Third write with correct expectedVersion
      const result2 = await t.client.writeMessage(stream, {
        type: 'Event3',
        data: {}
      }, { expectedVersion: 1 });

      expect(result2.position).toBe(2);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T4: optimistic locking prevents conflicts', async () => {
    const t = await setupTest('T4 optimistic locking');
    try {
      const stream = randomStreamName('account');

      // Initialize stream
      await t.client.writeMessage(stream, {
        type: 'Opened',
        data: { balance: 0 }
      });

      // This should succeed (version is now 0)
      await t.client.writeMessage(stream, {
        type: 'Deposited',
        data: { amount: 100 }
      }, { expectedVersion: 0 });

      // This should fail - version is now 1, not 0
      try {
        await t.client.writeMessage(stream, {
          type: 'Withdrawn',
          data: { amount: 50 }
        }, { expectedVersion: 0 });
        
        // Should not reach here
        expect(true).toBe(false);
      } catch (error: any) {
        // Should get version conflict error
        expect(error.message).toContain('VERSION');
      }

      // Verify stream state
      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(2); // Only 2 messages, third was rejected
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T5: get with position filter', async () => {
    const t = await setupTest('T5 position filter');
    try {
      const stream = randomStreamName('account');

      // Write 5 messages
      for (let i = 0; i < 5; i++) {
        await t.client.writeMessage(stream, { type: `Event${i}`, data: { i } });
      }

      // Read from position 0 (all messages)
      const all = await t.client.getStream(stream, { position: 0 });
      expect(all.length).toBe(5);

      // Read from position 2 (should skip first 2 messages)
      const fromPos2 = await t.client.getStream(stream, { position: 2 });
      expect(fromPos2.length).toBe(3);
      expect(fromPos2[0][1]).toBe('Event2'); // First returned message should be Event2

      // Read from position 4 (last message only)
      const fromPos4 = await t.client.getStream(stream, { position: 4 });
      expect(fromPos4.length).toBe(1);
      expect(fromPos4[0][1]).toBe('Event4');

      // Read from position 5 (beyond end)
      const fromPos5 = await t.client.getStream(stream, { position: 5 });
      expect(fromPos5.length).toBe(0);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T6: get with batchSize limit', async () => {
    const t = await setupTest('T6 batch size');
    try {
      const stream = randomStreamName('account');

      // Write 10 messages
      for (let i = 0; i < 10; i++) {
        await t.client.writeMessage(stream, { type: `Event${i}`, data: { i } });
      }

      // Read with batch size 3
      const batch3 = await t.client.getStream(stream, { batchSize: 3 });
      expect(batch3.length).toBe(3);
      expect(batch3[0][1]).toBe('Event0');
      expect(batch3[1][1]).toBe('Event1');
      expect(batch3[2][1]).toBe('Event2');

      // Read with batch size 5 from position 5
      const batch5 = await t.client.getStream(stream, { batchSize: 5, position: 5 });
      expect(batch5.length).toBe(5);
      expect(batch5[0][1]).toBe('Event5');

      // Read with batch size larger than remaining
      const batchLarge = await t.client.getStream(stream, { batchSize: 100 });
      expect(batchLarge.length).toBe(10);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T7: last message retrieval', async () => {
    const t = await setupTest('T7 last message');
    try {
      const stream = randomStreamName('account');

      // Empty stream - last returns null
      const lastEmpty = await t.client.getLastMessage(stream);
      expect(lastEmpty).toBeNull();

      // Write some messages
      await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
      await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
      await t.client.writeMessage(stream, { type: 'LastEvent', data: { n: 3, final: true } });

      // Get last message
      const last = await t.client.getLastMessage(stream);
      expect(last).not.toBeNull();
      expect(last[1]).toBe('LastEvent'); // type at index 1
      expect(last[4]).toEqual({ n: 3, final: true }); // data at index 4
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T8: stream version', async () => {
    const t = await setupTest('T8 stream version');
    try {
      const stream = randomStreamName('account');

      // Non-existent stream returns null
      const v0 = await t.client.getStreamVersion(stream);
      expect(v0).toBeNull();

      // Write first message - version becomes 0
      await t.client.writeMessage(stream, { type: 'Event1', data: {} });
      const v1 = await t.client.getStreamVersion(stream);
      expect(v1).toBe(0);

      // Write second message - version becomes 1
      await t.client.writeMessage(stream, { type: 'Event2', data: {} });
      const v2 = await t.client.getStreamVersion(stream);
      expect(v2).toBe(1);

      // Write third message - version becomes 2
      await t.client.writeMessage(stream, { type: 'Event3', data: {} });
      const v3 = await t.client.getStreamVersion(stream);
      expect(v3).toBe(2);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T9: empty stream returns empty array', async () => {
    const t = await setupTest('T9 empty stream');
    try {
      const stream = randomStreamName('nonexistent');

      // Get messages from non-existent stream
      const messages = await t.client.getStream(stream);
      expect(messages).toEqual([]);
      expect(Array.isArray(messages)).toBe(true);
      expect(messages.length).toBe(0);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T10: message metadata preserved', async () => {
    const t = await setupTest('T10 metadata');
    try {
      const stream = randomStreamName('test');

      // Write message with metadata
      await t.client.writeMessage(stream, {
        type: 'TestEvent',
        data: { foo: 'bar', count: 42 },
        metadata: {
          correlationStreamName: 'workflow-123',
          causationMessageId: 'msg-456',
          userId: 'user-789',
          custom: { nested: 'value' }
        }
      });

      // Read message back
      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(1);
      
      const msg = messages[0];
      // Message format: [id, type, position, globalPosition, data, metadata, time]
      const metadata = msg[5]; // metadata at index 5
      
      expect(metadata).toBeDefined();
      expect(metadata.correlationStreamName).toBe('workflow-123');
      expect(metadata.causationMessageId).toBe('msg-456');
      expect(metadata.userId).toBe('user-789');
      expect(metadata.custom).toEqual({ nested: 'value' });
    } finally {
      await t.cleanup();
    }
  });

  // Additional edge case tests

  test('MDB003_2A_T11: write with custom message ID', async () => {
    const t = await setupTest('T11 custom id');
    try {
      const stream = randomStreamName('test');
      // Custom ID must be a valid UUID format
      const customId = '12345678-1234-1234-1234-123456789abc';

      await t.client.writeMessage(stream, {
        type: 'TestEvent',
        data: {}
      }, { id: customId });

      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(1);
      // Message format: [id, type, position, globalPosition, data, metadata, time]
      expect(messages[0][0]).toBe(customId); // id at index 0
      expect(messages[0][1]).toBe('TestEvent'); // type at index 1
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T12: multiple writes to different streams', async () => {
    const t = await setupTest('T12 different streams');
    try {
      const stream1 = randomStreamName('account');
      const stream2 = randomStreamName('order');
      const stream3 = randomStreamName('user');

      // Write to different streams
      await t.client.writeMessage(stream1, { type: 'AccountEvent', data: {} });
      await t.client.writeMessage(stream2, { type: 'OrderEvent', data: {} });
      await t.client.writeMessage(stream3, { type: 'UserEvent', data: {} });

      // Each stream should have its own messages
      const msgs1 = await t.client.getStream(stream1);
      const msgs2 = await t.client.getStream(stream2);
      const msgs3 = await t.client.getStream(stream3);

      expect(msgs1.length).toBe(1);
      expect(msgs2.length).toBe(1);
      expect(msgs3.length).toBe(1);

      expect(msgs1[0][1]).toBe('AccountEvent');
      expect(msgs2[0][1]).toBe('OrderEvent');
      expect(msgs3[0][1]).toBe('UserEvent');
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T13: message time format is ISO 8601 UTC', async () => {
    const t = await setupTest('T13 time format');
    try {
      const stream = randomStreamName('test');

      await t.client.writeMessage(stream, { type: 'TestEvent', data: {} });

      const messages = await t.client.getStream(stream);
      // Message format: [id, type, position, globalPosition, data, metadata, time]
      const time = messages[0][6]; // time at index 6

      // Verify ISO 8601 format with Z suffix (UTC)
      expect(time).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$/);

      // Verify parseable as date
      const date = new Date(time);
      expect(date instanceof Date).toBe(true);
      expect(isNaN(date.getTime())).toBe(false);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T14: expectedVersion -1 means no stream (stream must not exist)', async () => {
    const t = await setupTest('T14 expected version -1');
    try {
      const stream = randomStreamName('test');

      // First write with expectedVersion -1 should succeed (stream doesn't exist)
      const result = await t.client.writeMessage(stream, {
        type: 'Event1',
        data: {}
      }, { expectedVersion: -1 });

      expect(result.position).toBe(0);

      // Second write with expectedVersion -1 should fail (stream now exists)
      try {
        await t.client.writeMessage(stream, {
          type: 'Event2',
          data: {}
        }, { expectedVersion: -1 });
        
        expect(true).toBe(false); // Should not reach
      } catch (error: any) {
        expect(error.message).toContain('VERSION');
      }
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2A_T15: large message data is preserved', async () => {
    const t = await setupTest('T15 large data');
    try {
      const stream = randomStreamName('test');

      // Create large data object
      const largeData = {
        items: Array.from({ length: 100 }, (_, i) => ({
          id: `item-${i}`,
          name: `Item number ${i}`,
          description: `This is a description for item ${i}`.repeat(10),
          values: Array.from({ length: 50 }, (_, j) => j * i)
        }))
      };

      await t.client.writeMessage(stream, {
        type: 'LargeEvent',
        data: largeData
      });

      const messages = await t.client.getStream(stream);
      expect(messages.length).toBe(1);
      // Message format: [id, type, position, globalPosition, data, metadata, time]
      expect(messages[0][4]).toEqual(largeData); // data at index 4
    } finally {
      await t.cleanup();
    }
  });
});
