/** undefined = not yet hydrated; null = signed out; string = active token. */
let token: string | null | undefined;

/** Callback invoked when a 401 arrives on a request that carried a token. */
let unauthorizedCb: (() => void) | undefined;

const STORAGE_KEY = 'eff_token';

/**
 * Returns the current bearer token, hydrating lazily from localStorage on
 * first call. Safe during SSR — returns null when window is absent.
 * Do NOT call localStorage at module scope: it runs during the server render
 * of /shop and crashes the build.
 */
export function getToken(): string | null {
  if (token === undefined) {
    if (typeof window === 'undefined') return null;
    token = localStorage.getItem(STORAGE_KEY);
  }
  return token;
}

/** Persists a bearer token to memory and localStorage. */
export function setToken(t: string): void {
  token = t;
  if (typeof window !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, t);
  }
}

/** Clears the token from memory and localStorage. */
export function clearToken(): void {
  token = null;
  if (typeof window !== 'undefined') {
    localStorage.removeItem(STORAGE_KEY);
  }
}

/**
 * Registers the callback to fire when a 401 response arrives on a request
 * that carried an Authorization header. Only one callback is active; each
 * call replaces the previous registration.
 */
export function onUnauthorized(cb: () => void): void {
  unauthorizedCb = cb;
}

/** Called by http.ts when a 401 arrives on an authenticated request. */
export function fireUnauthorized(): void {
  unauthorizedCb?.();
}
