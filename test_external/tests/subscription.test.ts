/**
 * External Tests for MessageDB - Phase MDB003_3A
 * 
 * SSE Subscription Tests - Real-time reactive notifications
 */

import { test, expect, describe, afterAll } from 'bun:test';
import { 
  setupTest, 
  stopSharedServer,
  randomStreamName, 
  type PokeEvent
} from '../lib';

afterAll(async () => {
  await stopSharedServer();
});

/**
 * SSE Connection - collects pokes via EventSource
 */
interface SSEConnection {
  close: () => void;
  waitForPokes: (count: number, timeoutMs?: number) => Promise<PokeEvent[]>;
  ready: Promise<void>;
}

function createSSEConnection(
  baseUrl: string,
  params: Record<string, string>,
  onPoke?: (poke: PokeEvent) => void
): SSEConnection {
  const url = new URL('/subscribe', baseUrl);
  for (const [key, value] of Object.entries(params)) {
    url.searchParams.set(key, value);
  }

  const pokes: PokeEvent[] = [];
  let isReady = false;
  let readyResolve: (() => void) | null = null;
  const readyPromise = new Promise<void>((resolve) => { 
    readyResolve = resolve;
    // Fallback: assume ready after 100ms if no signal received
    setTimeout(() => {
      if (!isReady) {
        isReady = true;
        resolve();
      }
    }, 100);
  });
  
  const waiters: { count: number; resolve: (p: PokeEvent[]) => void }[] = [];
  
  const checkWaiters = () => {
    for (let i = waiters.length - 1; i >= 0; i--) {
      if (pokes.length >= waiters[i].count) {
        waiters[i].resolve([...pokes]);
        waiters.splice(i, 1);
      }
    }
  };

  const controller = new AbortController();
  let closed = false;

  // Start SSE connection  
  (async () => {
    try {
      const response = await fetch(url.toString(), {
        method: 'GET',
        headers: { 'Accept': 'text/event-stream' },
        signal: controller.signal,
      });

      if (!response.ok || !response.body) {
        readyResolve?.();
        return;
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (!closed) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let eventType = '';
        let eventData = '';

        for (const line of lines) {
          if (line.startsWith(':')) {
            // SSE comment - check for ready signal
            if (line.includes('ready') && !isReady) {
              isReady = true;
              readyResolve?.();
              readyResolve = null;
            }
          } else if (line.startsWith('event:')) {
            eventType = line.slice(6).trim();
          } else if (line.startsWith('data:')) {
            eventData = line.slice(5).trim();
          } else if (line === '') {
            if (eventType === 'poke' && eventData) {
              try {
                const poke = JSON.parse(eventData) as PokeEvent;
                pokes.push(poke);
                onPoke?.(poke);
                checkWaiters();
              } catch {}
            }
            eventType = '';
            eventData = '';
          }
        }
      }
    } catch {}
  })();

  return {
    close: () => {
      closed = true;
      controller.abort();
    },
    ready: readyPromise,
    waitForPokes: async (count: number, timeoutMs: number = 500): Promise<PokeEvent[]> => {
      await readyPromise;
      
      if (pokes.length >= count) return [...pokes];
      
      return new Promise((resolve) => {
        const waiter = { count, resolve };
        waiters.push(waiter);
        setTimeout(() => {
          const idx = waiters.indexOf(waiter);
          if (idx >= 0) {
            waiters.splice(idx, 1);
            resolve([...pokes]);
          }
        }, timeoutMs);
      });
    }
  };
}

describe('MDB003_3A: SSE Subscriptions', () => {

  test('MDB003_3A_T1: stream subscription receives pokes', async () => {
    const t = await setupTest('T1');
    const stream = randomStreamName('account');

    const sub = createSSEConnection(
      'http://localhost:6789',
      { stream, position: '0', token: t.token }
    );

    await sub.ready;
    await t.client.writeMessage(stream, { type: 'AccountOpened', data: { balance: 0 } });

    const pokes = await sub.waitForPokes(1);
    expect(pokes.length).toBeGreaterThanOrEqual(1);
    expect(pokes[0].stream).toBe(stream);
    expect(pokes[0].position).toBe(0);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T2: poke contains correct position', async () => {
    const t = await setupTest('T2');
    const stream = randomStreamName('account');

    await t.client.writeMessage(stream, { type: 'Event0', data: {} });

    const sub = createSSEConnection(
      'http://localhost:6789',
      { stream, position: '1', token: t.token }
    );

    await sub.ready; // Wait for subscription to be ready
    const writeResult = await t.client.writeMessage(stream, { type: 'Event1', data: {} });
    const pokes = await sub.waitForPokes(1);

    expect(pokes.length).toBeGreaterThanOrEqual(1);
    expect(pokes[0].position).toBe(1);
    expect(pokes[0].globalPosition).toBe(writeResult.globalPosition);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T3: multiple pokes for multiple messages', async () => {
    const t = await setupTest('T3');
    const stream = randomStreamName('account');

    const sub = createSSEConnection(
      'http://localhost:6789',
      { stream, position: '0', token: t.token }
    );

    await sub.ready;
    await t.client.writeMessage(stream, { type: 'Event1', data: { n: 1 } });
    await t.client.writeMessage(stream, { type: 'Event2', data: { n: 2 } });
    await t.client.writeMessage(stream, { type: 'Event3', data: { n: 3 } });

    const pokes = await sub.waitForPokes(3);
    expect(pokes.length).toBeGreaterThanOrEqual(3);

    const positions = pokes.map(p => p.position);
    expect(positions).toContain(0);
    expect(positions).toContain(1);
    expect(positions).toContain(2);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T4: subscription from specific position', async () => {
    const t = await setupTest('T4');
    const stream = randomStreamName('account');

    await t.client.writeMessage(stream, { type: 'Event0', data: {} });
    await t.client.writeMessage(stream, { type: 'Event1', data: {} });
    await t.client.writeMessage(stream, { type: 'Event2', data: {} });

    const sub = createSSEConnection(
      'http://localhost:6789',
      { stream, position: '2', token: t.token }
    );

    const pokes = await sub.waitForPokes(1);
    expect(pokes.length).toBeGreaterThanOrEqual(1);
    expect(pokes[0].position).toBe(2);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T5: category subscription', async () => {
    const t = await setupTest('T5');
    const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

    const sub = createSSEConnection(
      'http://localhost:6789',
      { category, position: '0', token: t.token }
    );

    await sub.ready;
    await t.client.writeMessage(`${category}-1`, { type: 'Event', data: { stream: 1 } });
    await t.client.writeMessage(`${category}-2`, { type: 'Event', data: { stream: 2 } });

    const pokes = await sub.waitForPokes(2);
    expect(pokes.length).toBeGreaterThanOrEqual(2);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T6: poke includes stream name for category', async () => {
    const t = await setupTest('T6');
    const category = `cat_${Math.random().toString(36).substring(2, 8)}`;
    const stream1 = `${category}-stream1`;
    const stream2 = `${category}-stream2`;

    const sub = createSSEConnection(
      'http://localhost:6789',
      { category, position: '0', token: t.token }
    );

    await sub.ready;
    await t.client.writeMessage(stream1, { type: 'Event', data: {} });
    await t.client.writeMessage(stream2, { type: 'Event', data: {} });

    const pokes = await sub.waitForPokes(2);
    expect(pokes.length).toBeGreaterThanOrEqual(2);

    const streamNames = pokes.map(p => p.stream);
    expect(streamNames).toContain(stream1);
    expect(streamNames).toContain(stream2);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T7: connection cleanup on close', async () => {
    const t = await setupTest('T7');
    const stream = randomStreamName('account');
    let pokeCount = 0;

    const sub = createSSEConnection(
      'http://localhost:6789',
      { stream, position: '0', token: t.token },
      () => pokeCount++
    );

    await sub.ready;
    await t.client.writeMessage(stream, { type: 'Event1', data: {} });
    await sub.waitForPokes(1);
    const countBeforeClose = pokeCount;

    sub.close();

    await t.client.writeMessage(stream, { type: 'Event2', data: {} });
    
    // Brief wait to ensure no more pokes arrive
    await Bun.sleep(50);
    expect(pokeCount).toBe(countBeforeClose);

    await t.cleanup();
  });

  test('MDB003_3A_T8: subscription with consumer group', async () => {
    const t = await setupTest('T8');
    const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

    const sub = createSSEConnection(
      'http://localhost:6789',
      { category, position: '0', token: t.token, consumer: '0', size: '2' }
    );

    await sub.ready;
    for (let i = 0; i < 6; i++) {
      await t.client.writeMessage(`${category}-${i}`, { type: 'Event', data: { i } });
    }

    const pokes = await sub.waitForPokes(1);
    expect(pokes.length).toBeGreaterThan(0);
    expect(pokes.length).toBeLessThanOrEqual(6);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T9: subscription receives existing messages on connect', async () => {
    const t = await setupTest('T9');
    const stream = randomStreamName('account');

    // Write BEFORE subscribing
    await t.client.writeMessage(stream, { type: 'Event0', data: {} });
    await t.client.writeMessage(stream, { type: 'Event1', data: {} });
    await t.client.writeMessage(stream, { type: 'Event2', data: {} });

    const sub = createSSEConnection(
      'http://localhost:6789',
      { stream, position: '0', token: t.token }
    );

    const pokes = await sub.waitForPokes(3);
    expect(pokes.length).toBeGreaterThanOrEqual(3);
    
    const positions = pokes.slice(0, 3).map(p => p.position).sort((a, b) => a - b);
    expect(positions).toEqual([0, 1, 2]);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T10: subscription ignores messages before start position', async () => {
    const t = await setupTest('T10');
    const stream = randomStreamName('account');

    await t.client.writeMessage(stream, { type: 'Event0', data: {} });
    await t.client.writeMessage(stream, { type: 'Event1', data: {} });

    const sub = createSSEConnection(
      'http://localhost:6789',
      { stream, position: '2', token: t.token }
    );

    // Should get no pokes initially (position 2 doesn't exist yet)
    const initialPokes = await sub.waitForPokes(1, 100);
    expect(initialPokes.length).toBe(0);

    // Write at position 2
    await t.client.writeMessage(stream, { type: 'Event2', data: {} });

    const pokes = await sub.waitForPokes(1);
    expect(pokes.length).toBeGreaterThanOrEqual(1);
    expect(pokes[0].position).toBe(2);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T11: poke globalPosition increases across streams', async () => {
    const t = await setupTest('T11');
    const category = `cat_${Math.random().toString(36).substring(2, 8)}`;

    const sub = createSSEConnection(
      'http://localhost:6789',
      { category, position: '0', token: t.token }
    );

    await sub.ready;
    await t.client.writeMessage(`${category}-a`, { type: 'Event', data: {} });
    await t.client.writeMessage(`${category}-b`, { type: 'Event', data: {} });
    await t.client.writeMessage(`${category}-c`, { type: 'Event', data: {} });

    const pokes = await sub.waitForPokes(3);
    expect(pokes.length).toBeGreaterThanOrEqual(3);

    const globalPositions = pokes.map(p => p.globalPosition);
    const uniquePositions = new Set(globalPositions);
    expect(uniquePositions.size).toBe(pokes.length);

    sub.close();
    await t.cleanup();
  });

  test('MDB003_3A_T12: category subscription filters by category', async () => {
    const t = await setupTest('T12');
    const category1 = `cat1_${Math.random().toString(36).substring(2, 8)}`;
    const category2 = `cat2_${Math.random().toString(36).substring(2, 8)}`;

    const sub = createSSEConnection(
      'http://localhost:6789',
      { category: category1, position: '0', token: t.token }
    );

    await sub.ready;
    await t.client.writeMessage(`${category1}-stream`, { type: 'Event1', data: {} });
    await t.client.writeMessage(`${category2}-stream`, { type: 'Event2', data: {} });
    await t.client.writeMessage(`${category1}-other`, { type: 'Event3', data: {} });

    const pokes = await sub.waitForPokes(2);
    expect(pokes.length).toBeGreaterThanOrEqual(2);
    
    for (const poke of pokes) {
      expect(poke.stream.startsWith(category1)).toBe(true);
    }

    sub.close();
    await t.cleanup();
  });

});
