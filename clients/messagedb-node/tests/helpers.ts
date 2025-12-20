import { MessageDBClient } from '../src/client.js';

const MESSAGEDB_URL = process.env.MESSAGEDB_URL || 'http://localhost:8080';
const ADMIN_TOKEN = process.env.MESSAGEDB_ADMIN_TOKEN;

/**
 * Test context with isolated namespace
 */
export interface TestContext {
  client: MessageDBClient;
  namespaceId: string;
  token: string;
  cleanup: () => Promise<void>;
}

/**
 * Setup test with isolated namespace
 */
export async function setupTest(testName: string): Promise<TestContext> {
  // Create admin client for namespace management
  const adminClient = new MessageDBClient(MESSAGEDB_URL, { 
    token: ADMIN_TOKEN 
  });

  const namespaceId = `test-${testName}-${uniqueSuffix()}`;
  
  const result = await adminClient.namespaceCreate(namespaceId, {
    description: `Test namespace for ${testName}`
  });

  // Create client with namespace token
  const client = new MessageDBClient(MESSAGEDB_URL, { 
    token: result.token 
  });

  return {
    client,
    namespaceId,
    token: result.token,
    cleanup: async () => {
      await adminClient.namespaceDelete(namespaceId);
    }
  };
}

/**
 * Generate unique stream name
 */
export function randomStreamName(prefix = 'test'): string {
  return `${prefix}-${uniqueSuffix()}`;
}

/**
 * Generate unique suffix
 */
function uniqueSuffix(): string {
  return `${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
}

/**
 * Get MessageDB URL
 */
export function getMessageDBURL(): string {
  return MESSAGEDB_URL;
}
