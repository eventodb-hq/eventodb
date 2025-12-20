/**
 * Test Helpers for MessageDB External Tests
 * 
 * Phase MDB003_1A: External Test Client
 * 
 * Features:
 * - Auto-start/stop test server
 * - Unique namespace per test for isolation
 * - Server health detection
 */

import { MessageDBClient } from './client';

// Configuration
const DEFAULT_PORT = 6789;
const SERVER_BIN = process.env.EVENTODB_BIN || '../dist/eventodb';
const STARTUP_TIMEOUT_MS = 10000;
const HEALTH_CHECK_INTERVAL_MS = 100;

// Default token for default namespace (used by tests)
export const DEFAULT_TOKEN = 'ns_ZGVmYXVsdA_0000000000000000000000000000000000000000000000000000000000000000';

/**
 * Server instance returned by startTestServer
 */
export interface TestServer {
  url: string;
  port: number;
  close: () => Promise<void>;
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

// Global server instance (shared across all tests in a file)
let globalServer: TestServer | null = null;
let serverRefCount = 0;

/**
 * Start the MessageDB test server.
 * Returns a server instance with URL and close function.
 * 
 * Uses port 0 to get a random available port.
 */
export async function startTestServer(opts: { port?: number } = {}): Promise<TestServer> {
  const port = opts.port || DEFAULT_PORT;
  
  // Build the command
  const args = [SERVER_BIN, '--test-mode', `--port=${port}`, `--token=${DEFAULT_TOKEN}`];
  
  // Spawn the server process
  const proc = Bun.spawn(args, {
    stdout: 'pipe',
    stderr: 'pipe',
    env: { ...process.env },
  });

  // Wait for server to be healthy
  const serverUrl = `http://localhost:${port}`;
  
  try {
    await waitForHealthy(serverUrl, STARTUP_TIMEOUT_MS);
  } catch (err) {
    proc.kill();
    throw new Error(`Failed to start server: ${err}`);
  }

  return {
    url: serverUrl,
    port,
    close: async () => {
      proc.kill();
      // Wait a bit for process to terminate
      await Bun.sleep(100);
    }
  };
}

/**
 * Wait for the server to become healthy by polling /health endpoint.
 */
async function waitForHealthy(url: string, timeoutMs: number): Promise<void> {
  const healthUrl = `${url}/health`;
  const startTime = Date.now();
  
  while (Date.now() - startTime < timeoutMs) {
    try {
      const response = await fetch(healthUrl, { 
        method: 'GET',
        signal: AbortSignal.timeout(1000)
      });
      
      if (response.ok) {
        return;
      }
    } catch {
      // Connection failed, keep trying
    }
    
    await Bun.sleep(HEALTH_CHECK_INTERVAL_MS);
  }
  
  throw new Error(`Server did not become healthy within ${timeoutMs}ms`);
}

/**
 * Get the shared test server.
 * Starts the server if not already running.
 * Must be paired with releaseServer().
 */
export async function getSharedServer(): Promise<TestServer> {
  if (!globalServer) {
    globalServer = await startTestServer();
  }
  serverRefCount++;
  return globalServer;
}

/**
 * Release a reference to the shared server.
 * Does NOT stop the server (kept running for performance).
 */
export function releaseServer(): void {
  if (serverRefCount > 0) {
    serverRefCount--;
  }
}

/**
 * Stop the shared server if running.
 * Call this in afterAll() to clean up.
 */
export async function stopSharedServer(): Promise<void> {
  if (globalServer) {
    await globalServer.close();
    globalServer = null;
    serverRefCount = 0;
  }
}

/**
 * Create admin client for namespace management.
 * Uses the default token.
 */
export function createAdminClient(serverUrl: string): MessageDBClient {
  return new MessageDBClient(serverUrl, { token: DEFAULT_TOKEN });
}

/**
 * Counter for unique namespace IDs
 */
let namespaceCounter = 0;

/**
 * Generate a unique namespace ID from test name
 */
function generateNamespaceId(testName: string): string {
  const counter = (namespaceCounter++).toString(36);
  const timestamp = Date.now().toString(36);
  const random = Math.random().toString(36).substring(2, 6);
  return `t_${counter}_${timestamp}_${random}`;
}

/**
 * Generate a deterministic token for a namespace (for testing)
 */
function tokenForNamespace(ns: string): string {
  // Base64url encode namespace for token format
  const nsEncoded = Buffer.from(ns).toString('base64url');
  // Use zeros for the random part (deterministic for tests)
  return `ns_${nsEncoded}_${'0'.repeat(64)}`;
}

/**
 * Setup a unique namespace for a test.
 * Automatically handles server lifecycle.
 * Returns a TestContext with client ready to use.
 * 
 * Usage:
 * ```typescript
 * const t = await setupTest('my test name');
 * try {
 *   // use t.client
 * } finally {
 *   await t.cleanup();
 * }
 * ```
 */
export async function setupTest(testName: string): Promise<TestContext> {
  // Get or start the shared server
  const server = await getSharedServer();
  
  // Create unique namespace
  const namespace = generateNamespaceId(testName);
  const token = tokenForNamespace(namespace);
  
  // Create namespace using admin client
  const admin = createAdminClient(server.url);
  await admin.createNamespace(namespace, { token });
  
  // Create client for this namespace
  const client = new MessageDBClient(server.url, { token });
  
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
      releaseServer();
    }
  };
}

/**
 * Generate a random stream name with optional category prefix.
 */
export function randomStreamName(category: string = 'test'): string {
  const id = Math.random().toString(36).substring(2, 15);
  return `${category}-${id}`;
}

/**
 * Get the server URL for manual testing.
 */
export function getServerURL(): string {
  if (globalServer) {
    return globalServer.url;
  }
  return process.env.EVENTODB_URL || `http://localhost:${DEFAULT_PORT}`;
}

/**
 * Export SERVER_URL for backward compatibility
 */
export const SERVER_URL = `http://localhost:${DEFAULT_PORT}`;
