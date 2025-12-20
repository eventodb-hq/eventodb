import { describe, test, expect } from 'vitest';
import { getEventoDBURL } from './helpers.js';
import { MessageDBClient } from '../src/client.js';

const ADMIN_TOKEN = process.env.EVENTODB_ADMIN_TOKEN;

describe('SYS Tests', () => {
  test('SYS-001: Get server version', async () => {
    // System endpoints may require authentication in production mode
    const client = new MessageDBClient(getEventoDBURL(), {
      token: ADMIN_TOKEN
    });
    
    const version = await client.systemVersion();

    expect(typeof version).toBe('string');
    // Should match semver pattern (e.g., "1.3.0")
    expect(version).toMatch(/^\d+\.\d+\.\d+/);
  });

  test('SYS-002: Get server health', async () => {
    // System endpoints may require authentication in production mode
    const client = new MessageDBClient(getEventoDBURL(), {
      token: ADMIN_TOKEN
    });
    
    const health = await client.systemHealth();

    expect(health).toBeDefined();
    expect(health.status).toBeDefined();
    expect(typeof health.status).toBe('string');
  });
});
