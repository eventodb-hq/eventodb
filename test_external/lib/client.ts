/**
 * MessageDB TypeScript Client
 * 
 * Simple HTTP client for testing MessageDB server.
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
  expectedVersion?: number;
}

export interface GetStreamOptions {
  position?: number;
  batchSize?: number;
}

export interface GetCategoryOptions {
  position?: number;
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
}

/**
 * MessageDB Client
 */
export class MessageDBClient {
  private token?: string;

  constructor(
    private baseURL: string,
    opts: { token?: string } = {}
  ) {
    this.token = opts.token;
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

    const response = await fetch(`${this.baseURL}/rpc`, {
      method: 'POST',
      headers,
      body: JSON.stringify([method, ...args])
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error?.code || 'UNKNOWN_ERROR');
    }

    const result = await response.json();
    return result;
  }

  /**
   * Write a message to a stream
   */
  async writeMessage(
    streamName: string,
    msg: Message,
    opts?: WriteOptions
  ): Promise<{ position: number; globalPosition: number }> {
    return this.rpc('stream.write', streamName, msg, opts || {});
  }

  /**
   * Get messages from a stream
   */
  async getStream(
    streamName: string,
    opts?: GetStreamOptions
  ): Promise<any[]> {
    return this.rpc('stream.get', streamName, opts || {});
  }

  /**
   * Get the last message from a stream
   */
  async getLastMessage(streamName: string): Promise<any | null> {
    return this.rpc('stream.last', streamName);
  }

  /**
   * Get the current version of a stream (-1 if empty)
   */
  async getStreamVersion(streamName: string): Promise<number | null> {
    return this.rpc('stream.version', streamName);
  }

  /**
   * Get messages from a category
   */
  async getCategory(
    categoryName: string,
    opts?: GetCategoryOptions
  ): Promise<any[]> {
    return this.rpc('category.get', categoryName, opts || {});
  }

  /**
   * Create a new namespace
   */
  async createNamespace(
    namespaceId: string,
    opts?: CreateNamespaceOptions
  ): Promise<{ namespace: string; token: string; createdAt: string }> {
    return this.rpc('ns.create', namespaceId, opts || {});
  }

  /**
   * Delete a namespace
   */
  async deleteNamespace(namespaceId: string): Promise<void> {
    await this.rpc('ns.delete', namespaceId);
  }

  /**
   * Get the current token
   */
  getToken(): string | undefined {
    return this.token;
  }
}
