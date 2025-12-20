import { EventoDBError, NetworkError } from './errors.js';
import type {
  Message,
  WriteOptions,
  WriteResult,
  GetStreamOptions,
  GetCategoryOptions,
  GetLastOptions,
  CreateNamespaceOptions,
  NamespaceResult,
  DeleteNamespaceResult,
  NamespaceInfo,
  StreamMessage,
  CategoryMessage,
  SubscribeOptions,
  PokeEvent,
  Subscription
} from './types.js';

/**
 * MessageDB Client
 * 
 * Core client for interacting with MessageDB via RPC API.
 */
export class EventoDBClient {
  private token?: string;

  constructor(
    private readonly baseURL: string,
    options: { token?: string } = {}
  ) {
    this.token = options.token;
  }

  /**
   * Make an RPC call to the server
   */
  private async rpc(method: string, ...args: any[]): Promise<any> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json'
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    let response: Response;
    try {
      response = await fetch(`${this.baseURL}/rpc`, {
        method: 'POST',
        headers,
        body: JSON.stringify([method, ...args])
      });
    } catch (error) {
      throw new NetworkError(
        `Failed to connect to ${this.baseURL}`,
        error as Error
      );
    }

    // Capture token from response header (auto-creation in test mode)
    const newToken = response.headers.get('X-MessageDB-Token');
    if (newToken && !this.token) {
      this.token = newToken;
    }

    if (!response.ok) {
      const errorData = await response.json() as any;
      if (errorData.error) {
        throw EventoDBError.fromResponse(errorData.error);
      }
      throw new EventoDBError(
        'UNKNOWN_ERROR',
        `HTTP ${response.status}: ${response.statusText}`
      );
    }

    return response.json();
  }

  // ==================
  // Stream Operations
  // ==================

  /**
   * Write a message to a stream
   */
  async streamWrite(
    streamName: string,
    message: Message,
    options: WriteOptions = {}
  ): Promise<WriteResult> {
    return this.rpc('stream.write', streamName, message, options);
  }

  /**
   * Get messages from a stream
   */
  async streamGet(
    streamName: string,
    options: GetStreamOptions = {}
  ): Promise<StreamMessage[]> {
    return this.rpc('stream.get', streamName, options);
  }

  /**
   * Get the last message from a stream
   */
  async streamLast(
    streamName: string,
    options: GetLastOptions = {}
  ): Promise<StreamMessage | null> {
    return this.rpc('stream.last', streamName, options);
  }

  /**
   * Get the version of a stream
   */
  async streamVersion(streamName: string): Promise<number | null> {
    return this.rpc('stream.version', streamName);
  }

  // ====================
  // Category Operations
  // ====================

  /**
   * Get messages from a category
   */
  async categoryGet(
    categoryName: string,
    options: GetCategoryOptions = {}
  ): Promise<CategoryMessage[]> {
    return this.rpc('category.get', categoryName, options);
  }

  // =====================
  // Namespace Operations
  // =====================

  /**
   * Create a new namespace
   */
  async namespaceCreate(
    namespaceId: string,
    options: CreateNamespaceOptions = {}
  ): Promise<NamespaceResult> {
    return this.rpc('ns.create', namespaceId, options);
  }

  /**
   * Delete a namespace
   */
  async namespaceDelete(namespaceId: string): Promise<DeleteNamespaceResult> {
    return this.rpc('ns.delete', namespaceId);
  }

  /**
   * List all namespaces
   */
  async namespaceList(): Promise<NamespaceInfo[]> {
    return this.rpc('ns.list');
  }

  /**
   * Get namespace information
   */
  async namespaceInfo(namespaceId: string): Promise<NamespaceInfo> {
    return this.rpc('ns.info', namespaceId);
  }

  // ===================
  // System Operations
  // ===================

  /**
   * Get server version
   */
  async systemVersion(): Promise<string> {
    return this.rpc('sys.version');
  }

  /**
   * Get server health status
   */
  async systemHealth(): Promise<{ status: string }> {
    return this.rpc('sys.health');
  }

  /**
   * Get current authentication token
   */
  getToken(): string | undefined {
    return this.token;
  }

  // ===================
  // SSE Subscriptions
  // ===================

  /**
   * Subscribe to stream updates via Server-Sent Events
   * 
   * Note: Node.js doesn't have native EventSource. This is a placeholder
   * implementation. For production use, consider using the 'eventsource' package
   * or implement custom fetch-based streaming.
   */
  streamSubscribe(
    streamName: string,
    options: SubscribeOptions = {}
  ): Subscription {
    return this.createSubscription('stream', streamName, options);
  }

  /**
   * Subscribe to category updates via Server-Sent Events
   */
  categorySubscribe(
    categoryName: string,
    options: SubscribeOptions = {}
  ): Subscription {
    return this.createSubscription('category', categoryName, options);
  }

  /**
   * Create an SSE subscription
   * 
   * Note: This is a minimal implementation. Node.js doesn't have EventSource built-in.
   * For full SSE support, you would need to:
   * 1. Use the 'eventsource' npm package, OR
   * 2. Implement custom fetch-based streaming with ReadableStream
   * 
   * This placeholder provides the API shape for testing purposes.
   */
  private createSubscription(
    type: 'stream' | 'category',
    name: string,
    options: SubscribeOptions
  ): Subscription {
    const handlers: {
      poke: Array<(event: PokeEvent) => void>;
      error: Array<(error: Error) => void>;
      end: Array<() => void>;
    } = {
      poke: [],
      error: [],
      end: []
    };

    let closed = false;

    // Build URL with query parameters
    const params = new URLSearchParams();
    if (this.token) {
      params.set('token', this.token);
    }
    if (options.position !== undefined) {
      params.set('position', options.position.toString());
    }
    if (options.correlation) {
      params.set('correlation', options.correlation);
    }
    if (options.consumerGroup) {
      params.set('consumerGroupMember', options.consumerGroup.member.toString());
      params.set('consumerGroupSize', options.consumerGroup.size.toString());
    }

    const endpoint = type === 'stream' 
      ? `${this.baseURL}/subscribe/stream/${encodeURIComponent(name)}`
      : `${this.baseURL}/subscribe/category/${encodeURIComponent(name)}`;
    
    const url = `${endpoint}?${params.toString()}`;

    // Note: This is a simplified implementation
    // In production, you'd use EventSource or custom streaming
    const subscription: Subscription = {
      close() {
        closed = true;
        handlers.end.forEach(handler => handler());
      },
      on(event: string, handler: any) {
        if (event === 'poke') {
          handlers.poke.push(handler);
        } else if (event === 'error') {
          handlers.error.push(handler);
        } else if (event === 'end') {
          handlers.end.push(handler);
        }
      }
    };

    // Emit error to indicate SSE not fully implemented
    setTimeout(() => {
      if (!closed) {
        const error = new Error(
          'SSE subscriptions require EventSource polyfill in Node.js. ' +
          'Install and use the "eventsource" package for full SSE support. ' +
          `Would connect to: ${url}`
        );
        handlers.error.forEach(handler => handler(error));
      }
    }, 0);

    return subscription;
  }
}
