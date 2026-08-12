/** Machine-readable error classifier from the API error envelope. */
export type ErrorCode =
  | 'VALIDATION_ERROR'
  | 'BAD_REQUEST'
  | 'UNAUTHORIZED'
  | 'NOT_FOUND'
  | 'CONFLICT'
  | 'RATE_LIMITED'
  | 'INTERNAL'
  | 'GATEWAY_ERROR'
  | 'METHOD_NOT_ALLOWED';

/** One field-level validation failure from the error envelope details. */
export interface FieldError {
  field: string;
  message: string;
}

/** Thrown when the server returns an error envelope. */
export class ApiError extends Error {
  readonly code: ErrorCode;
  readonly status: number;
  readonly fields: readonly FieldError[];

  constructor(code: ErrorCode, message: string, status: number, fields: FieldError[] = []) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.fields = fields;
  }
}

/** Thrown when fetch rejects or the response body is not parseable JSON. */
export class NetworkError extends Error {
  constructor(message: string, readonly cause?: unknown) {
    super(message);
    this.name = 'NetworkError';
  }
}

/** Type guard for ApiError. */
export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

/** Type guard for NetworkError. */
export function isNetworkError(e: unknown): e is NetworkError {
  return e instanceof NetworkError;
}

/**
 * Returns the first field-error message from a VALIDATION_ERROR, or undefined.
 * Useful when a form shows one validation message at a time.
 */
export function firstFieldError(e: unknown): string | undefined {
  if (isApiError(e) && e.fields.length > 0) {
    return e.fields[0]?.message;
  }
  return undefined;
}

/** Returns the validation message for a named field, or undefined. */
export function fieldError(e: unknown, field: string): string | undefined {
  if (!isApiError(e)) return undefined;
  return e.fields.find((f) => f.field === field)?.message;
}
