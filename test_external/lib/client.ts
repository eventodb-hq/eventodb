/**
 * MessageDB TypeScript Client
 * 
 * Phase MDB003_1A: External Test Client
 * 
 * Simple HTTP client for testing MessageDB server via RPC API.
 */

export interface Message {
  id?: string;
  type: string;
  data: any;
  metadata?: {
    correlationStreamName?: string;
    causationMessageId?: string;
    [key: string]: any;
  };
}

export interface WriteOptions {
  id?: string;
  expectedVersion?: number;
}

export interface GetStreamOptions {
  position?: number;
  globalPosition?: number;
  batchSize?: number;
}

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

export interface CreateNamespaceOptions {
  token?: string;
  description?: string;
  metadata?: Record<string, any>;
}

export interface WriteResult {
  position: number;
  globalPosition: number;
}

export interface NamespaceResult {
  namespace: string;
  token: string;
  createdAt: string;
}

export interface DeleteNamespaceResult {
  namespace: string;
  deletedAt: string;
  messagesDeleted: number;
}

export interface NamespaceInfo {
  namespace: string;
  description: string;
  createdAt: string;
  messageCount: number;
  streamCount: number;
  lastActivity: string | null;
}

export interface StreamInfo {
  stream: string;
  version: number;
  lastActivity: string;
}

export interface CategoryInfo {
  category: string;
  streamCount: number;
  messageCount: number;
}

export interface ListStreamsOptions {
  prefix?: string;
  limit?: number;
  cursor?: string;
}

export interface SubscribeOptions {
  position?: number;
  consumerGroup?: {
    member: number;
    size: number;
  };
  onPoke?: (poke: PokeEvent) => void;
  onError?: (error: Error) => void;
  onClose?: () => void;
}

export interface PokeEvent {
  stream: string;
  position: number;
  globalPosition: number;
}

export interface Subscription {
  close: () => void;
}

/**
 * MessageDB Client - HTTP client for RPC API
 */
export class EventoDBClient {
  private token?: string;

  constructor(
    private baseURL: string,
    opts: { token?: string } = {}
  ) {
    this.token = opts.token;
  }

  /**
   * Make an RPC call to the server.
   * Request format: ["method", arg1, arg2, ...]
   */
  async rpc(method: string, ...args: any[]): Promise<any> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json'
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    const response = await fetch(`${this.baseURL}/rpc`, {
      method: 'POST',
      headers,
      body: JSON.stringify([method, ...args])
    });

    // Capture token from response header (for test mode auto-creation)
    const newToken = response.headers.get('X-MessageDB-Token');
    if (newToken && !this.token) {
      this.token = newToken;
    }

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error?.code || 'UNKNOWN_ERROR');
    }

    return response.json();
  }

  // ==================
  // Stream Operations
  // ==================

  /**
   * Write a message to a stream.
   */
  async writeMessage(
    streamName: string,
    msg: Message,
    opts?: WriteOptions
  ): Promise<WriteResult> {
    return this.rpc('stream.write', streamName, msg, opts || {});
  }

  /**
   * Get messages from a stream.
   */
  async getStream(
    streamName: string,
    opts?: GetStreamOptions
  ): Promise<any[]> {
    return this.rpc('stream.get', streamName, opts || {});
  }

  /**
   * Get the last message from a stream.
   */
  async getLastMessage(streamName: string, opts?: { type?: string }): Promise<any | null> {
    return this.rpc('stream.last', streamName, opts || {});
  }

  /**
   * Get the current version of a stream (-1 if empty, null if doesn't exist).
   */
  async getStreamVersion(streamName: string): Promise<number | null> {
    return this.rpc('stream.version', streamName);
  }

  // ====================
  // Category Operations
  // ====================

  /**
   * Get messages from all streams in a category.
   */
  async getCategory(
    categoryName: string,
    opts?: GetCategoryOptions
  ): Promise<any[]> {
    return this.rpc('category.get', categoryName, opts || {});
  }

  // =====================
  // Namespace Operations
  // =====================

  /**
   * Create a new namespace.
   */
  async createNamespace(
    namespaceId: string,
    opts?: CreateNamespaceOptions
  ): Promise<NamespaceResult> {
    return this.rpc('ns.create', namespaceId, opts || {});
  }

  /**
   * Delete a namespace and all its data.
   */
  async deleteNamespace(namespaceId: string): Promise<DeleteNamespaceResult> {
    return this.rpc('ns.delete', namespaceId);
  }

  /**
   * List all namespaces.
   */
  async listNamespaces(): Promise<any[]> {
    return this.rpc('ns.list');
  }

  /**
   * Get information about a namespace.
   */
  async getNamespaceInfo(namespaceId: string): Promise<NamespaceInfo> {
    return this.rpc('ns.info', namespaceId);
  }

  /**
   * List streams in the current namespace.
   */
  async listStreams(opts?: ListStreamsOptions): Promise<StreamInfo[]> {
    return this.rpc('ns.streams', opts || {});
  }

  /**
   * List distinct categories in the current namespace with stream and message counts.
   */
  async listCategories(): Promise<CategoryInfo[]> {
    return this.rpc('ns.categories');
  }

  // ===================
  // System Operations
  // ===================

  /**
   * Get server version.
   */
  async getVersion(): Promise<string> {
    return this.rpc('sys.version');
  }

  /**
   * Get server health status.
   */
  async getHealth(): Promise<{ status: string; backend: string; connections: number }> {
    return this.rpc('sys.health');
  }

  // =====================
  // Subscription (SSE)
  // =====================

  /**
   * Subscribe to a stream for real-time updates.
   * Returns a Subscription object with close() method.
   * 
   * Note: Uses Server-Sent Events (SSE).
   */
  subscribeToStream(streamName: string, opts: SubscribeOptions = {}): Subscription {
    const params = new URLSearchParams({
      stream: streamName,
      position: String(opts.position || 0)
    });

    if (this.token) {
      params.set('token', this.token);
    }

    const url = `${this.baseURL}/subscribe?${params}`;
    const eventSource = new EventSource(url);

    eventSource.addEventListener('poke', (event) => {
      if (opts.onPoke) {
        try {
          const data = JSON.parse(event.data);
          opts.onPoke(data);
        } catch (err) {
          if (opts.onError) {
            opts.onError(new Error(`Failed to parse poke: ${err}`));
          }
        }
      }
    });

    eventSource.onerror = (event) => {
      if (opts.onError) {
        opts.onError(new Error('SSE connection error'));
      }
    };

    eventSource.addEventListener('close', () => {
      if (opts.onClose) {
        opts.onClose();
      }
    });

    return {
      close: () => {
        eventSource.close();
      }
    };
  }

  /**
   * Subscribe to a category for real-time updates.
   */
  subscribeToCategory(categoryName: string, opts: SubscribeOptions = {}): Subscription {
    const params = new URLSearchParams({
      category: categoryName,
      position: String(opts.position || 0)
    });

    if (opts.consumerGroup) {
      params.set('consumerGroupMember', String(opts.consumerGroup.member));
      params.set('consumerGroupSize', String(opts.consumerGroup.size));
    }

    if (this.token) {
      params.set('token', this.token);
    }

    const url = `${this.baseURL}/subscribe?${params}`;
    const eventSource = new EventSource(url);

    eventSource.addEventListener('poke', (event) => {
      if (opts.onPoke) {
        try {
          const data = JSON.parse(event.data);
          opts.onPoke(data);
        } catch (err) {
          if (opts.onError) {
            opts.onError(new Error(`Failed to parse poke: ${err}`));
          }
        }
      }
    });

    eventSource.onerror = (event) => {
      if (opts.onError) {
        opts.onError(new Error('SSE connection error'));
      }
    };

    return {
      close: () => {
        eventSource.close();
      }
    };
  }

  // =================
  // Token Management
  // =================

  /**
   * Get the current token.
   */
  getToken(): string | undefined {
    return this.token;
  }

  /**
   * Set the token.
   */
  setToken(token: string): void {
    this.token = token;
  }

  /**
   * Parse namespace from token (for debugging).
   */
  parseNamespaceFromToken(token: string): string | null {
    try {
      // Token format: ns_<base64url-namespace>_<random>
      const parts = token.split('_');
      if (parts.length >= 2 && parts[0] === 'ns') {
        return Buffer.from(parts[1], 'base64url').toString();
      }
    } catch {
      // Invalid token
    }
    return null;
  }
}
