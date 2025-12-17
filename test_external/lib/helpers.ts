/**
 * Test Helpers
 * 
 * Each test gets its own unique namespace based on test name.
 * No coordination, no shared state, no contention.
 */

import { MessageDBClient } from './client';

// Server URL
export const SERVER_URL = process.env.MESSAGEDB_URL || 'http://localhost:6789';

// Admin token for creating namespaces
export const DEFAULT_TOKEN = 'ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000';

/**
 * Create admin client (for namespace management only)
 */
export function createAdminClient(): MessageDBClient {
  return new MessageDBClient(SERVER_URL, { token: DEFAULT_TOKEN });
}

/**
 * Counter for unique namespace IDs
 */
let namespaceCounter = 0;

/**
 * Generate a unique namespace ID from test name
 */
function namespaceFromTestName(testName: string): string {
  // Use counter + timestamp + random for guaranteed uniqueness
  const counter = (namespaceCounter++).toString(36);
  const timestamp = Date.now().toString(36);
  const random = Math.random().toString(36).substring(2, 6);
  return `t_${counter}_${timestamp}_${random}`;
}

/**
 * Generate a token for a namespace
 */
function tokenForNamespace(ns: string): string {
  // Base64 encode namespace for token format
  const nsEncoded = Buffer.from(ns).toString('base64url');
  // Use zeros for the random part (deterministic for tests)
  return `ns_${nsEncoded}_${'0'.repeat(64)}`;
}

/**
 * Test context - holds namespace and client for a test
 */
export interface TestContext {
  namespace: string;
  token: string;
  client: MessageDBClient;
  cleanup: () => Promise<void>;
}

/**
 * Setup a unique namespace for a test.
 * Returns a client ready to use.
 */
export async function setupTest(testName: string): Promise<TestContext> {
  const namespace = namespaceFromTestName(testName);
  const token = tokenForNamespace(namespace);
  
  const admin = createAdminClient();
  await admin.createNamespace(namespace, { token });
  
  const client = new MessageDBClient(SERVER_URL, { token });
  
  return {
    namespace,
    token,
    client,
    cleanup: async () => {
      try {
        await admin.deleteNamespace(namespace);
      } catch {
        // Ignore cleanup errors
      }
    }
  };
}

/**
 * Generate a random stream name
 */
export function randomStreamName(category: string = 'test'): string {
  const id = Math.random().toString(36).substring(2, 15);
  return `${category}-${id}`;
}
