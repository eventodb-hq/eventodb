/**
 * Base error for MessageDB operations
 */
export class EventoDBError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: Record<string, any>
  ) {
    super(message);
    this.name = 'EventoDBError';
  }

  /**
   * Create error from server response
   */
  static fromResponse(errorData: {
    code: string;
    message: string;
    details?: Record<string, any>;
  }): EventoDBError {
    return new EventoDBError(
      errorData.code,
      errorData.message,
      errorData.details
    );
  }
}

/**
 * Network/connection errors
 */
export class NetworkError extends EventoDBError {
  constructor(message: string, cause?: Error) {
    super('NETWORK_ERROR', message, { cause });
    this.name = 'NetworkError';
  }
}

/**
 * Authentication errors
 */
export class AuthError extends EventoDBError {
  constructor(code: string, message: string) {
    super(code, message);
    this.name = 'AuthError';
  }
}
