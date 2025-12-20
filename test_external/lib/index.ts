/**
 * MessageDB External Test Library
 * 
 * Phase MDB003_1A: External Test Client
 */

export { EventoDBClient } from './client';
export type {
  Message,
  WriteOptions,
  GetStreamOptions,
  GetCategoryOptions,
  CreateNamespaceOptions,
  WriteResult,
  NamespaceResult,
  DeleteNamespaceResult,
  NamespaceInfo,
  SubscribeOptions,
  PokeEvent,
  Subscription
} from './client';

export {
  setupTest,
  startTestServer,
  stopSharedServer,
  getSharedServer,
  releaseServer,
  createAdminClient,
  randomStreamName,
  getServerURL,
  SERVER_URL,
  DEFAULT_TOKEN
} from './helpers';
export type { TestServer, TestContext } from './helpers';
