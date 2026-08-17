/**
 * Admin token store.
 *
 * Admin tokens are stored in sessionStorage, NOT localStorage. A shopper token
 * in localStorage is intentional — the storefront keeps you signed in across
 * browser sessions. An admin token opens write access over the whole catalogue,
 * so it must die when the tab closes. If this were localStorage, an unattended
 * laptop would leave the console open indefinitely.
 *
 * Do NOT change this to localStorage to "match the storefront" — the behaviour
 * difference is the security boundary, not an oversight.
 */

/** undefined = not yet hydrated; null = signed out; string = active token. */
let token: string | null | undefined;

/** Callback invoked when a 401 arrives on a request that carried a token. */
let unauthorizedCb: (() => void) | undefined;

const STORAGE_KEY = 'eff_admin_token';

/**
 * Returns the current bearer token, hydrating lazily from sessionStorage on
 * first call. Safe during SSR — returns null when window is absent.
 * Do NOT call sessionStorage at module scope: it runs during the server render
 * and crashes the build.
 */
export function getToken(): string | null {
  if (token === undefined) {
    if (typeof window === 'undefined') return null;
    token = sessionStorage.getItem(STORAGE_KEY);
  }
  return token;
}

/** Persists a bearer token to memory and sessionStorage. */
export function setToken(t: string): void {
  token = t;
  if (typeof window !== 'undefined') {
    sessionStorage.setItem(STORAGE_KEY, t);
  }
}

/** Clears the token from memory and sessionStorage. */
export function clearToken(): void {
  token = null;
  if (typeof window !== 'undefined') {
    sessionStorage.removeItem(STORAGE_KEY);
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
