/**
 * Base error for MessageDB operations
 */
export class MessageDBError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: Record<string, any>
  ) {
    super(message);
    this.name = 'MessageDBError';
  }

  /**
   * Create error from server response
   */
  static fromResponse(errorData: {
    code: string;
    message: string;
    details?: Record<string, any>;
  }): MessageDBError {
    return new MessageDBError(
      errorData.code,
      errorData.message,
      errorData.details
    );
  }
}

/**
 * Network/connection errors
 */
export class NetworkError extends MessageDBError {
  constructor(message: string, cause?: Error) {
    super('NETWORK_ERROR', message, { cause });
    this.name = 'NetworkError';
  }
}

/**
 * Authentication errors
 */
export class AuthError extends MessageDBError {
  constructor(code: string, message: string) {
    super(code, message);
    this.name = 'AuthError';
  }
}
