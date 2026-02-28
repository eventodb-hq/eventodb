/**
 * External Tests for EventoDB - ADR-011
 *
 * Namespace Stream and Category Listing Tests:
 * - ns.streams: list streams with prefix, cursor pagination
 * - ns.categories: list distinct categories with counts
 */

import { test, expect, describe, afterAll } from 'bun:test';
import {
  stopSharedServer,
  setupTest,
} from '../lib';

afterAll(async () => {
  await stopSharedServer();
});

// ==========================================
// ns.streams tests
// ==========================================

describe('ADR011: ns.streams', () => {

  test('returns empty array for empty namespace', async () => {
    const ctx = await setupTest('streams-empty');
    try {
      const result = await ctx.client.listStreams();
      expect(Array.isArray(result)).toBe(true);
      expect(result.length).toBe(0);
    } finally {
      await ctx.cleanup();
    }
  });

  test('returns streams after writes', async () => {
    const ctx = await setupTest('streams-after-writes');
    try {
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('account-2', { type: 'Created', data: {} });
      await ctx.client.writeMessage('order-1', { type: 'Created', data: {} });

      const result = await ctx.client.listStreams();
      expect(result.length).toBe(3);

      // Verify structure
      const first = result[0];
      expect(typeof first.stream).toBe('string');
      expect(typeof first.version).toBe('number');
      expect(typeof first.lastActivity).toBe('string');
    } finally {
      await ctx.cleanup();
    }
  });

  test('results sorted lexicographically', async () => {
    const ctx = await setupTest('streams-sorted');
    try {
      await ctx.client.writeMessage('order-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('user-1', { type: 'Created', data: {} });

      const result = await ctx.client.listStreams();
      expect(result.length).toBe(3);
      expect(result[0].stream).toBe('account-1');
      expect(result[1].stream).toBe('order-1');
      expect(result[2].stream).toBe('user-1');
    } finally {
      await ctx.cleanup();
    }
  });

  test('prefix filter works', async () => {
    const ctx = await setupTest('streams-prefix');
    try {
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('account-2', { type: 'Created', data: {} });
      await ctx.client.writeMessage('order-1', { type: 'Created', data: {} });

      const result = await ctx.client.listStreams({ prefix: 'account' });
      expect(result.length).toBe(2);
      for (const s of result) {
        expect(s.stream.startsWith('account')).toBe(true);
      }
    } finally {
      await ctx.cleanup();
    }
  });

  test('cursor pagination: no duplicates, no gaps', async () => {
    const ctx = await setupTest('streams-pagination');
    try {
      await ctx.client.writeMessage('stream-a', { type: 'Created', data: {} });
      await ctx.client.writeMessage('stream-b', { type: 'Created', data: {} });
      await ctx.client.writeMessage('stream-c', { type: 'Created', data: {} });
      await ctx.client.writeMessage('stream-d', { type: 'Created', data: {} });

      const page1 = await ctx.client.listStreams({ limit: 2 });
      expect(page1.length).toBe(2);

      const cursor = page1[1].stream;
      const page2 = await ctx.client.listStreams({ limit: 2, cursor });
      expect(page2.length).toBe(2);

      // No overlap
      for (const s of page2) {
        expect(s.stream > cursor).toBe(true);
      }

      // All 4 covered
      const allNames = new Set([
        ...page1.map(s => s.stream),
        ...page2.map(s => s.stream),
      ]);
      expect(allNames.size).toBe(4);
    } finally {
      await ctx.cleanup();
    }
  });

  test('version reflects last message position', async () => {
    const ctx = await setupTest('streams-version');
    try {
      await ctx.client.writeMessage('account-1', { type: 'A', data: {} });
      await ctx.client.writeMessage('account-1', { type: 'B', data: {} });
      await ctx.client.writeMessage('account-1', { type: 'C', data: {} });

      const result = await ctx.client.listStreams();
      expect(result.length).toBe(1);
      expect(result[0].version).toBe(2); // 0-based, 3 messages → version 2
    } finally {
      await ctx.cleanup();
    }
  });

  test('lastActivity is a valid ISO 8601 timestamp', async () => {
    const ctx = await setupTest('streams-last-activity');
    try {
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });

      const result = await ctx.client.listStreams();
      expect(result.length).toBe(1);
      const parsed = new Date(result[0].lastActivity);
      expect(isNaN(parsed.getTime())).toBe(false);
    } finally {
      await ctx.cleanup();
    }
  });

  test('empty prefix returns all streams', async () => {
    const ctx = await setupTest('streams-empty-prefix');
    try {
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('order-1', { type: 'Created', data: {} });

      const result = await ctx.client.listStreams({ prefix: '' });
      expect(result.length).toBe(2);
    } finally {
      await ctx.cleanup();
    }
  });

});

// ==========================================
// ns.categories tests
// ==========================================

describe('ADR011: ns.categories', () => {

  test('returns empty array for empty namespace', async () => {
    const ctx = await setupTest('cats-empty');
    try {
      const result = await ctx.client.listCategories();
      expect(Array.isArray(result)).toBe(true);
      expect(result.length).toBe(0);
    } finally {
      await ctx.cleanup();
    }
  });

  test('derives categories from stream names', async () => {
    const ctx = await setupTest('cats-derive');
    try {
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('account-2', { type: 'Created', data: {} });
      await ctx.client.writeMessage('order-1', { type: 'Created', data: {} });

      const result = await ctx.client.listCategories();
      expect(result.length).toBe(2);
      expect(result[0].category).toBe('account');
      expect(result[1].category).toBe('order');
    } finally {
      await ctx.cleanup();
    }
  });

  test('streamCount and messageCount are accurate', async () => {
    const ctx = await setupTest('cats-counts');
    try {
      await ctx.client.writeMessage('account-1', { type: 'A', data: {} });
      await ctx.client.writeMessage('account-1', { type: 'B', data: {} });
      await ctx.client.writeMessage('account-2', { type: 'A', data: {} });
      await ctx.client.writeMessage('order-1', { type: 'A', data: {} });

      const result = await ctx.client.listCategories();
      expect(result.length).toBe(2);

      const catMap = Object.fromEntries(result.map(c => [c.category, c]));
      expect(catMap['account'].streamCount).toBe(2);
      expect(catMap['account'].messageCount).toBe(3);
      expect(catMap['order'].streamCount).toBe(1);
      expect(catMap['order'].messageCount).toBe(1);
    } finally {
      await ctx.cleanup();
    }
  });

  test('stream with no dash is its own category', async () => {
    const ctx = await setupTest('cats-no-dash');
    try {
      await ctx.client.writeMessage('account', { type: 'Created', data: {} });
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });

      const result = await ctx.client.listCategories();
      expect(result.length).toBe(1);
      expect(result[0].category).toBe('account');
      expect(result[0].streamCount).toBe(2);
      expect(result[0].messageCount).toBe(2);
    } finally {
      await ctx.cleanup();
    }
  });

  test('results sorted lexicographically', async () => {
    const ctx = await setupTest('cats-sorted');
    try {
      await ctx.client.writeMessage('user-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('account-1', { type: 'Created', data: {} });
      await ctx.client.writeMessage('order-1', { type: 'Created', data: {} });

      const result = await ctx.client.listCategories();
      expect(result.length).toBe(3);
      expect(result[0].category).toBe('account');
      expect(result[1].category).toBe('order');
      expect(result[2].category).toBe('user');
    } finally {
      await ctx.cleanup();
    }
  });

});
