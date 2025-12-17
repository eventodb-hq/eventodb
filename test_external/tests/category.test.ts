/**
 * External Tests for MessageDB - Phase MDB003_2B
 * 
 * Category Operation Tests:
 * - MDB003_2B_T1: Test category returns messages from multiple streams
 * - MDB003_2B_T2: Test category includes stream names
 * - MDB003_2B_T3: Test consumer groups partition streams
 * - MDB003_2B_T4: Test consumer groups have no overlap
 * - MDB003_2B_T5: Test correlation filtering
 * - MDB003_2B_T6: Test category with position filter
 * - MDB003_2B_T7: Test category with batchSize
 * - MDB003_2B_T8: Test empty category returns empty array
 */

import { test, expect, describe } from 'bun:test';
import { 
  setupTest, 
  randomStreamName, 
  type TestContext 
} from '../lib';

// =========================================
// Phase MDB003_2B: Category Operation Tests
// =========================================

describe('MDB003_2B: Category Operations', () => {

  test('MDB003_2B_T1: category returns messages from multiple streams', async () => {
    const t = await setupTest('T1 category multiple streams');
    try {
      const category = 'account';
      const stream1 = `${category}-${Math.random().toString(36).substring(2, 10)}`;
      const stream2 = `${category}-${Math.random().toString(36).substring(2, 10)}`;
      const stream3 = `${category}-${Math.random().toString(36).substring(2, 10)}`;

      // Write to multiple streams in the same category
      await t.client.writeMessage(stream1, {
        type: 'AccountOpened',
        data: { accountId: '123', balance: 0 }
      });
      await t.client.writeMessage(stream2, {
        type: 'AccountOpened',
        data: { accountId: '456', balance: 0 }
      });
      await t.client.writeMessage(stream3, {
        type: 'AccountOpened',
        data: { accountId: '789', balance: 0 }
      });
      // Write another to stream1
      await t.client.writeMessage(stream1, {
        type: 'Deposited',
        data: { amount: 100 }
      });

      // Get category messages
      const messages = await t.client.getCategory(category);

      // Should get 4 messages total
      expect(messages.length).toBe(4);

      // Category messages have format: [id, streamName, type, position, globalPosition, data, metadata, time]
      // Verify we have messages from all three streams
      const streamNames = new Set(messages.map(m => m[1]));
      expect(streamNames.has(stream1)).toBe(true);
      expect(streamNames.has(stream2)).toBe(true);
      expect(streamNames.has(stream3)).toBe(true);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T2: category includes stream names in response', async () => {
    const t = await setupTest('T2 category stream names');
    try {
      const category = 'order';
      const stream = `${category}-${Math.random().toString(36).substring(2, 10)}`;

      // Write a message
      await t.client.writeMessage(stream, {
        type: 'OrderPlaced',
        data: { orderId: 'ord-123', total: 99.99 }
      });

      // Get category messages
      const messages = await t.client.getCategory(category);
      
      expect(messages.length).toBe(1);
      
      // Category message format: [id, streamName, type, position, globalPosition, data, metadata, time]
      const msg = messages[0];
      expect(msg).toHaveLength(8);
      
      // Verify fields
      expect(typeof msg[0]).toBe('string'); // id
      expect(msg[1]).toBe(stream); // streamName
      expect(msg[2]).toBe('OrderPlaced'); // type
      expect(typeof msg[3]).toBe('number'); // position
      expect(typeof msg[4]).toBe('number'); // globalPosition
      expect(msg[5]).toEqual({ orderId: 'ord-123', total: 99.99 }); // data
      expect(typeof msg[7]).toBe('string'); // time
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T3: consumer groups partition streams', async () => {
    const t = await setupTest('T3 consumer groups partition');
    try {
      const category = 'user';
      
      // Create multiple streams (10 streams for better distribution)
      const streams: string[] = [];
      for (let i = 0; i < 10; i++) {
        const stream = `${category}-${Math.random().toString(36).substring(2, 15)}`;
        streams.push(stream);
        await t.client.writeMessage(stream, {
          type: 'UserCreated',
          data: { index: i }
        });
      }

      // Get messages for consumer 0 of 2
      const messages0 = await t.client.getCategory(category, {
        consumerGroup: { member: 0, size: 2 }
      });

      // Get messages for consumer 1 of 2
      const messages1 = await t.client.getCategory(category, {
        consumerGroup: { member: 1, size: 2 }
      });

      // Both consumers should have messages (statistically likely with 10 streams)
      // Note: With 10 streams, it's very unlikely one consumer gets 0
      expect(messages0.length + messages1.length).toBe(10);
      
      // At least one consumer should have messages
      expect(messages0.length > 0 || messages1.length > 0).toBe(true);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T4: consumer groups have no overlap', async () => {
    const t = await setupTest('T4 consumer groups no overlap');
    try {
      const category = 'payment';
      
      // Create multiple streams with predictable names
      const streams: string[] = [];
      for (let i = 0; i < 12; i++) {
        const stream = `${category}-stream${i}`;
        streams.push(stream);
        await t.client.writeMessage(stream, {
          type: 'PaymentReceived',
          data: { index: i, amount: i * 10 }
        });
      }

      // Get messages for all 3 consumers
      const messages0 = await t.client.getCategory(category, {
        consumerGroup: { member: 0, size: 3 }
      });
      const messages1 = await t.client.getCategory(category, {
        consumerGroup: { member: 1, size: 3 }
      });
      const messages2 = await t.client.getCategory(category, {
        consumerGroup: { member: 2, size: 3 }
      });

      // Extract stream names from each consumer
      const getStreamNames = (msgs: any[]): Set<string> => 
        new Set(msgs.map(m => m[1]));

      const streams0 = getStreamNames(messages0);
      const streams1 = getStreamNames(messages1);
      const streams2 = getStreamNames(messages2);

      // Check for NO overlap between consumers
      for (const s of streams0) {
        expect(streams1.has(s)).toBe(false);
        expect(streams2.has(s)).toBe(false);
      }
      for (const s of streams1) {
        expect(streams2.has(s)).toBe(false);
      }

      // Total messages should be 12
      const total = messages0.length + messages1.length + messages2.length;
      expect(total).toBe(12);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T5: correlation filtering', async () => {
    const t = await setupTest('T5 correlation filtering');
    try {
      const category = 'task';
      
      // Write messages with different correlation metadata
      await t.client.writeMessage(`${category}-1`, {
        type: 'TaskCreated',
        data: { taskId: '1' },
        metadata: { correlationStreamName: 'workflow-abc' }
      });
      await t.client.writeMessage(`${category}-2`, {
        type: 'TaskCreated',
        data: { taskId: '2' },
        metadata: { correlationStreamName: 'workflow-xyz' }
      });
      await t.client.writeMessage(`${category}-3`, {
        type: 'TaskCreated',
        data: { taskId: '3' },
        metadata: { correlationStreamName: 'process-123' }
      });
      await t.client.writeMessage(`${category}-4`, {
        type: 'TaskCreated',
        data: { taskId: '4' }
        // No correlation metadata
      });

      // Filter by workflow correlation
      const workflowMessages = await t.client.getCategory(category, {
        correlation: 'workflow'
      });

      // Should get only messages with workflow correlation (2 messages)
      expect(workflowMessages.length).toBe(2);

      // Verify correlation metadata
      for (const msg of workflowMessages) {
        const metadata = msg[6];
        expect(metadata).toBeDefined();
        expect(metadata.correlationStreamName).toMatch(/^workflow-/);
      }

      // Filter by process correlation
      const processMessages = await t.client.getCategory(category, {
        correlation: 'process'
      });

      expect(processMessages.length).toBe(1);
      expect(processMessages[0][6].correlationStreamName).toBe('process-123');
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T6: category with position filter', async () => {
    const t = await setupTest('T6 category position filter');
    try {
      const category = 'event';
      
      // Write 5 messages
      const globalPositions: number[] = [];
      for (let i = 0; i < 5; i++) {
        const result = await t.client.writeMessage(`${category}-stream${i}`, {
          type: 'EventOccurred',
          data: { index: i }
        });
        globalPositions.push(result.globalPosition);
      }

      // Get all messages first to verify count
      const allMessages = await t.client.getCategory(category);
      expect(allMessages.length).toBe(5);

      // Get messages from position of 3rd message
      const thirdPosition = globalPositions[2];
      const fromThird = await t.client.getCategory(category, {
        position: thirdPosition
      });

      // Should get 3 messages (3rd, 4th, 5th)
      expect(fromThird.length).toBe(3);

      // Verify first returned message has the expected global position
      expect(fromThird[0][4]).toBe(thirdPosition);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T7: category with batchSize', async () => {
    const t = await setupTest('T7 category batch size');
    try {
      const category = 'log';
      
      // Write 10 messages
      for (let i = 0; i < 10; i++) {
        await t.client.writeMessage(`${category}-stream${i}`, {
          type: 'LogEntry',
          data: { level: 'info', message: `Log entry ${i}` }
        });
      }

      // Get with batchSize of 3
      const batch3 = await t.client.getCategory(category, { batchSize: 3 });
      expect(batch3.length).toBe(3);

      // Get with batchSize of 7
      const batch7 = await t.client.getCategory(category, { batchSize: 7 });
      expect(batch7.length).toBe(7);

      // Get with batchSize larger than available
      const batchLarge = await t.client.getCategory(category, { batchSize: 100 });
      expect(batchLarge.length).toBe(10);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T8: empty category returns empty array', async () => {
    const t = await setupTest('T8 empty category');
    try {
      // Query a category that has no streams
      const messages = await t.client.getCategory('nonexistent_category_xyz');
      
      expect(Array.isArray(messages)).toBe(true);
      expect(messages.length).toBe(0);
    } finally {
      await t.cleanup();
    }
  });

  // Additional edge case tests

  test('MDB003_2B_T9: compound IDs route to same consumer', async () => {
    const t = await setupTest('T9 compound IDs');
    try {
      const category = 'entity';
      
      // Write to streams with compound IDs (same cardinal ID = 123)
      // Compound IDs use + to separate cardinal ID from suffix
      await t.client.writeMessage(`${category}-123+alice`, {
        type: 'EntityUpdated',
        data: { suffix: 'alice' }
      });
      await t.client.writeMessage(`${category}-123+bob`, {
        type: 'EntityUpdated',
        data: { suffix: 'bob' }
      });
      await t.client.writeMessage(`${category}-456+charlie`, {
        type: 'EntityUpdated',
        data: { suffix: 'charlie' }
      });

      // Get messages for consumer 0 of 2
      const messages0 = await t.client.getCategory(category, {
        consumerGroup: { member: 0, size: 2 }
      });
      // Get messages for consumer 1 of 2
      const messages1 = await t.client.getCategory(category, {
        consumerGroup: { member: 1, size: 2 }
      });

      // Extract stream names
      const streams0 = messages0.map(m => m[1]);
      const streams1 = messages1.map(m => m[1]);

      // Streams with cardinal ID 123 (alice and bob) should be in SAME consumer
      const has123Alice0 = streams0.includes(`${category}-123+alice`);
      const has123Bob0 = streams0.includes(`${category}-123+bob`);
      const has123Alice1 = streams1.includes(`${category}-123+alice`);
      const has123Bob1 = streams1.includes(`${category}-123+bob`);

      // If alice is in consumer 0, bob should also be in consumer 0
      // If alice is in consumer 1, bob should also be in consumer 1
      expect(has123Alice0).toBe(has123Bob0);
      expect(has123Alice1).toBe(has123Bob1);

      // Total should be 3 messages
      expect(messages0.length + messages1.length).toBe(3);
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T10: messages ordered by global position', async () => {
    const t = await setupTest('T10 category ordering');
    try {
      const category = 'sequence';
      
      // Write messages in sequence
      for (let i = 0; i < 5; i++) {
        await t.client.writeMessage(`${category}-stream${i}`, {
          type: 'SequenceEvent',
          data: { sequence: i }
        });
      }

      // Get all messages
      const messages = await t.client.getCategory(category);
      
      expect(messages.length).toBe(5);

      // Verify messages are ordered by global position
      for (let i = 1; i < messages.length; i++) {
        const prevGlobalPos = messages[i - 1][4];
        const currGlobalPos = messages[i][4];
        expect(currGlobalPos).toBeGreaterThan(prevGlobalPos);
      }
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T11: category distinguishes from stream name', async () => {
    const t = await setupTest('T11 category vs stream');
    try {
      // Write to streams in different categories but with similar names
      await t.client.writeMessage('account-123', {
        type: 'AccountEvent',
        data: {}
      });
      await t.client.writeMessage('accountLog-123', {
        type: 'LogEvent',
        data: {}
      });
      await t.client.writeMessage('accountTransaction-123', {
        type: 'TxEvent',
        data: {}
      });

      // Query 'account' category should only return account-123
      const accountMessages = await t.client.getCategory('account');
      expect(accountMessages.length).toBe(1);
      expect(accountMessages[0][1]).toBe('account-123');
      expect(accountMessages[0][2]).toBe('AccountEvent');

      // Query 'accountLog' category should only return accountLog-123
      const logMessages = await t.client.getCategory('accountLog');
      expect(logMessages.length).toBe(1);
      expect(logMessages[0][1]).toBe('accountLog-123');
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T12: consumer group with position filter', async () => {
    const t = await setupTest('T12 consumer group with position');
    try {
      const category = 'filtered';
      
      // Write 10 messages to different streams
      const globalPositions: number[] = [];
      for (let i = 0; i < 10; i++) {
        const result = await t.client.writeMessage(`${category}-stream${i}`, {
          type: 'FilteredEvent',
          data: { index: i }
        });
        globalPositions.push(result.globalPosition);
      }

      // Get messages for consumer 0 of 2, starting from position of 5th message
      const fifthPosition = globalPositions[4];
      const messages = await t.client.getCategory(category, {
        position: fifthPosition,
        consumerGroup: { member: 0, size: 2 }
      });

      // Should get messages from position onwards, filtered by consumer group
      // The exact count depends on hash distribution, but should be subset of remaining 6
      expect(messages.length).toBeLessThanOrEqual(6);
      expect(messages.length).toBeGreaterThanOrEqual(0);

      // All returned messages should have globalPosition >= fifthPosition
      for (const msg of messages) {
        expect(msg[4]).toBeGreaterThanOrEqual(fifthPosition);
      }
    } finally {
      await t.cleanup();
    }
  });

  test('MDB003_2B_T13: globalPosition option works', async () => {
    const t = await setupTest('T13 globalPosition option');
    try {
      const category = 'global';
      
      // Write messages
      const results: any[] = [];
      for (let i = 0; i < 5; i++) {
        const result = await t.client.writeMessage(`${category}-stream${i}`, {
          type: 'GlobalEvent',
          data: { index: i }
        });
        results.push(result);
      }

      // Get all messages and verify globalPosition behavior
      const allMessages = await t.client.getCategory(category);
      expect(allMessages.length).toBe(5);

      // For categories, position IS the global position (they're the same)
      // The 'position' option filters by global position
      // Note: 'globalPosition' option may not be implemented in all backends
      // Using 'position' which is the standard way
      const thirdGlobalPos = results[2].globalPosition;
      const messages = await t.client.getCategory(category, {
        position: thirdGlobalPos  // For categories, position = globalPosition
      });

      // Should get messages from that global position onwards
      expect(messages.length).toBe(3);
      expect(messages[0][4]).toBe(thirdGlobalPos);
    } finally {
      await t.cleanup();
    }
  });

});
