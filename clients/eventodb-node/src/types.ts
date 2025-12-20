/**
 * Message to write to a stream
 */
export interface Message {
  type: string;
  data: Record<string, any>;
  metadata?: Record<string, any> | null;
}

/**
 * Options for writing messages
 */
export interface WriteOptions {
  id?: string;
  expectedVersion?: number;
}

/**
 * Result from writing a message
 */
export interface WriteResult {
  position: number;
  globalPosition: number;
}

/**
 * Options for reading from a stream
 */
export interface GetStreamOptions {
  position?: number;
  globalPosition?: number;
  batchSize?: number;
}

/**
 * Options for reading from a category
 */
export interface GetCategoryOptions {
  position?: number;
  globalPosition?: number;
  batchSize?: number;
  correlation?: string;
  consumerGroup?: {
    member: number;
    size: number;
  };
}

/**
 * Options for getting last message
 */
export interface GetLastOptions {
  type?: string;
}

/**
 * Options for creating a namespace
 */
export interface CreateNamespaceOptions {
  token?: string;
  description?: string;
  metadata?: Record<string, any>;
}

/**
 * Result from creating a namespace
 */
export interface NamespaceResult {
  namespace: string;
  token: string;
  createdAt: string;
}

/**
 * Result from deleting a namespace
 */
export interface DeleteNamespaceResult {
  namespace: string;
  deletedAt: string;
  messagesDeleted: number;
}

/**
 * Namespace information
 */
export interface NamespaceInfo {
  namespace: string;
  description: string;
  createdAt: string;
  messageCount: number;
  streamCount: number;
  lastActivity: string | null;
}

/**
 * Stream message format: 
 * [id, type, position, globalPosition, data, metadata, time]
 */
export type StreamMessage = [
  string,                    // id
  string,                    // type
  number,                    // position
  number,                    // globalPosition
  Record<string, any>,       // data
  Record<string, any> | null, // metadata
  string                     // time (ISO 8601)
];

/**
 * Category message format:
 * [id, streamName, type, position, globalPosition, data, metadata, time]
 */
export type CategoryMessage = [
  string,                    // id
  string,                    // streamName
  string,                    // type
  number,                    // position
  number,                    // globalPosition
  Record<string, any>,       // data
  Record<string, any> | null, // metadata
  string                     // time (ISO 8601)
];

/**
 * Options for subscribing to stream/category updates
 */
export interface SubscribeOptions {
  position?: number;
  correlation?: string;
  consumerGroup?: {
    member: number;
    size: number;
  };
}

/**
 * Poke event from SSE subscription
 */
export interface PokeEvent {
  stream?: string;
  category?: string;
  position: number;
  globalPosition: number;
}

/**
 * Subscription handle for managing SSE connections
 */
export interface Subscription {
  close(): void;
  on(event: 'poke', handler: (poke: PokeEvent) => void): void;
  on(event: 'error', handler: (error: Error) => void): void;
  on(event: 'end', handler: () => void): void;
}
