import { API_BASE } from './config';
import { ApiError, NetworkError } from './errors';
import type { ErrorCode, FieldError } from './errors';
import { fireUnauthorized, getToken } from './token';

/* ----------------------------------------------------------------- envelope */

interface ApiErrorBody {
  code: string;
  message: string;
  details?: { fields: FieldError[] };
}

interface ErrorEnvelope {
  error: ApiErrorBody;
}

interface DataEnvelope<T> {
  data: T;
}

/** Narrows ONLY the envelope shape — does not validate the inner payload. */
function isErrorEnvelope(body: unknown): body is ErrorEnvelope {
  return (
    typeof body === 'object' &&
    body !== null &&
    'error' in body &&
    typeof (body as Record<string, unknown>)['error'] === 'object' &&
    (body as Record<string, unknown>)['error'] !== null
  );
}

/* ----------------------------------------------------------------- request */

/**
 * Makes an API request and returns the unwrapped `data` value on success.
 * Throws ApiError for a server error envelope, NetworkError for a fetch
 * rejection or a body that cannot be parsed as JSON.
 *
 * The global 401 hook fires ONLY when the request carried an Authorization
 * header. POST /admin/login also returns 401 (wrong credentials), so a
 * header-gated check is the only way to avoid clearing the token on a
 * failed sign-in attempt.
 */
export async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  let sentAuth = false;
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
    sentAuth = true;
  }

  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      // Never 'include' — the server deliberately omits Allow-Credentials.
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    });
  } catch (e) {
    throw new NetworkError('Network request failed', e);
  }

  let parsed: unknown;
  try {
    parsed = await res.json();
  } catch (e) {
    throw new NetworkError('Response body is not valid JSON', e);
  }

  if (!res.ok) {
    if (isErrorEnvelope(parsed)) {
      const { code, message, details } = parsed.error;
      const fields = details?.fields ?? [];
      const err = new ApiError(code as ErrorCode, message, res.status, fields);
      if (res.status === 401 && sentAuth) {
        fireUnauthorized();
      }
      throw err;
    }
    throw new NetworkError(`HTTP ${res.status}`);
  }

  // Single trust boundary: the server owns the shape of data; we cast once here.
  return (parsed as DataEnvelope<T>).data; // trust boundary
}
