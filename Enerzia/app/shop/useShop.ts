'use client';

import { useCallback, useEffect, useReducer, useRef } from 'react';

import { exchangeSession, getMe, requestOtp, verifyOtp as apiVerifyOtp } from '@/lib/api/auth';
import {
  confirmPayment as apiConfirmPayment,
  getOrder as apiGetOrder,
  listOrders,
  openCheckout as apiOpenCheckout,
} from '@/lib/api/orders';
import {
  addAddress as apiAddAddress,
  listAddresses,
  removeAddress as apiRemoveAddress,
  updateAddress as apiUpdateAddress,
} from '@/lib/api/addresses';
import {
  addItem as apiAddItem,
  clearCart as apiClearCart,
  getCart,
  removeItem as apiRemoveItem,
  setQty as apiSetQty,
} from '@/lib/api/cart';
import { getProduct, getProducts, getTrust } from '@/lib/api/catalogue';
import { fieldError, isApiError, isNetworkError } from '@/lib/api/errors';
import { clearToken, getToken, onUnauthorized, setToken } from '@/lib/api/token';
import { initialState } from '@/lib/shop/pricing';
import { shopReducer } from '@/lib/shop/reducer';
import type { ShopAction } from '@/lib/shop/reducer';
import type { Address, CartDTO, RequestKey, ShopState } from '@/lib/shop/types';

// ── offline cart buffer helpers ──────────────────────────────────────────────

const BUFFER_KEY = 'enerzia:cart-buffer';
type BufferItem = { productId: string; qty: number };

function loadBuffer(): BufferItem[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = localStorage.getItem(BUFFER_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed as BufferItem[];
  } catch {
    return [];
  }
}

function saveBuffer(buf: BufferItem[]): void {
  if (typeof window === 'undefined') return;
  if (buf.length === 0) {
    localStorage.removeItem(BUFFER_KEY);
  } else {
    localStorage.setItem(BUFFER_KEY, JSON.stringify(buf));
  }
}

/**
 * Returns a new buffer with productId's qty incremented by qty (capped at 99).
 * Appends a new entry when the product is not yet in the buffer.
 */
function applyBufferAdd(buf: BufferItem[], productId: string, qty: number): BufferItem[] {
  const existing = buf.find((i) => i.productId === productId);
  if (existing) {
    return buf.map((i) =>
      i.productId === productId ? { ...i, qty: Math.min(99, i.qty + qty) } : i,
    );
  }
  return [...buf, { productId, qty: Math.min(99, qty) }];
}

/**
 * Returns a new buffer with productId's qty set to qty.
 * Removes the entry when qty <= 0.
 */
function applyBufferSetQty(buf: BufferItem[], productId: string, qty: number): BufferItem[] {
  if (qty <= 0) return buf.filter((i) => i.productId !== productId);
  return buf.map((i) => (i.productId === productId ? { ...i, qty: Math.min(99, qty) } : i));
}

// ── public interface ─────────────────────────────────────────────────────────

/**
 * Async command functions exposed to components. This interface grows with
 * each later task; pure navigation still goes through dispatch directly.
 */
export interface ShopActions {
  /** Calls POST /auth/otp/request with the current phone in state. */
  sendOtp: () => Promise<void>;
  /** Calls POST /auth/otp/verify with the current phone + otp in state. */
  verifyOtp: () => Promise<void>;
  /** Calls GET /products/{id} for the detail payload. Aborts any prior load. */
  loadProduct: (id: string) => Promise<void>;
  /** Loads the cart from the server. No-op if a cart write is in flight. */
  loadCart: () => Promise<void>;
  /**
   * Adds a product to the cart.
   * - Signed in: POST /cart/items immediately.
   * - Signed out: appends/increments the offline buffer and stays on the
   *   current screen (no auth gate, no navigation).
   */
  addItem: (productId: string, qty: number) => Promise<void>;
  /**
   * PDP-specific add: sources productId and qty from state.
   * - Signed in: POST /cart/items then navigates to the cart.
   * - Signed out: appends/increments the offline buffer, stays on the PDP.
   */
  addFromPdp: () => Promise<void>;
  /** Sets the absolute quantity of a cart line. qty=0 removes the line. */
  setQty: (productId: string, qty: number) => Promise<void>;
  /** Removes a line from the cart entirely. */
  removeItem: (productId: string) => Promise<void>;
  /** Empties the cart. */
  clearCart: () => Promise<void>;
  /**
   * Updates the qty of a buffered item while signed out.
   * qty=0 removes the item. Persists to localStorage immediately.
   */
  setBufferQty: (productId: string, qty: number) => void;
  /** Loads the saved address list from GET /me/addresses. No-op if already in flight. */
  loadAddresses: () => Promise<void>;
  /**
   * POSTs (new) or PUTs (edit) an address. Client-side validates first, then
   * maps any server field errors back onto the form inputs.
   */
  saveAddress: (addr: Address) => Promise<void>;
  /** Deletes an address and refreshes the list. */
  deleteAddress: (id: string) => Promise<void>;
  /**
   * Exchanges an MSG91 widget access token for our session JWT, then runs the
   * same post-sign-in sequence as verifyOtp: load cart + addresses, replay the
   * offline cart buffer, then navigate to state.next.
   */
  signInWithWidget: (accessToken: string) => Promise<void>;
  /**
   * POSTs to /orders with the currently selected addressId. Handles each
   * distinct server error with a specific message. The Pay button must be
   * disabled while state.pending.checkout is true.
   */
  openCheckout: () => Promise<void>;
  /** Fetches the shopper's order list from GET /orders. No-op if already in flight. */
  loadOrders: () => Promise<void>;
  /**
   * Signs the shopper out: clears the token, wipes the offline cart buffer from
   * localStorage, clears all user state, and navigates to 'shop'. Use this for
   * manual sign-out. The global-401 handler also calls this same function so
   * both paths stay in sync.
   */
  signOut: () => void;
}

/**
 * The single source of truth for the shop. Owns the reducer and all async
 * commands. Components receive { state, dispatch, actions } and stay dumb —
 * no fetch inside any component, ever.
 *
 * Every network command follows: guard → reqStart → await api → loaded | reqFail.
 * The reducer keeps ZERO I/O, no Date.now, no Math.random.
 */
export function useShop(): {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
} {
  const [state, dispatch] = useReducer(shopReducer, initialState);

  /**
   * Tracks which keys have a request currently in flight. MUST be checked
   * synchronously before the first await in every command — reading
   * state.pending inside a callback closure gives a stale render snapshot,
   * so two clicks in the same tick would both see pending.x === false and
   * both fire.
   */
  const inflight = useRef(new Set<RequestKey>());

  /**
   * Holds the AbortController for the most recent request per key. Starting
   * a new request for a key aborts the previous one, preventing stale
   * responses from painting over fresh data.
   */
  const controllers = useRef(new Map<RequestKey, AbortController>());

  /**
   * StrictMode guard. React 19 double-invokes effects in development;
   * this ref survives the remount so exactly one boot request fires.
   */
  const booted = useRef(false);

  /** Loads the shopper's order list from GET /orders. No-op if already in flight. */
  const loadOrders = useCallback(async () => {
    const key: RequestKey = 'orders';
    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await listOrders();
      dispatch({ type: 'ordersLoaded', orders: data.orders });
    } catch (e) {
      dispatch({
        type: 'reqFail',
        key,
        message: isNetworkError(e)
          ? 'Unable to reach the server. Please check your connection.'
          : isApiError(e)
          ? e.message
          : e instanceof Error
          ? e.message
          : 'Something went wrong.',
      });
    } finally {
      inflight.current.delete(key);
    }
  }, []);

  // Single sign-out path shared by manual sign-out and the global-401 handler.
  const performSignOut = useCallback(() => {
    clearToken();
    saveBuffer([]);
    dispatch({ type: 'signOut' });
  }, []);

  // Register the global sign-out hook once. The token module fires this
  // when a 401 arrives on a request that carried an Authorization header.
  useEffect(() => {
    onUnauthorized(performSignOut);
  }, [performSignOut]);

  /**
   * Shared helper for every cart mutation. Runs fn() under the 'cart' inflight
   * guard so only one write is in flight at a time. Returns null if a write is
   * already running; callers should treat null as "request skipped".
   */
  const cartMutation = useCallback(
    async (fn: () => Promise<CartDTO>): Promise<CartDTO | null> => {
      const key: RequestKey = 'cart';
      if (inflight.current.has(key)) return null;
      inflight.current.add(key);

      dispatch({ type: 'reqStart', key });
      try {
        const cart = await fn();
        dispatch({ type: 'cartLoaded', cart });
        return cart;
      } catch (e) {
        dispatch({
          type: 'reqFail',
          key,
          message: isNetworkError(e)
            ? 'Unable to reach the server. Please check your connection.'
            : isApiError(e)
            ? e.message
            : e instanceof Error
            ? e.message
            : 'Something went wrong.',
        });
        return null;
      } finally {
        inflight.current.delete(key);
      }
    },
    [],
  );

  const loadTrust = useCallback(async () => {
    const key: RequestKey = 'trust';

    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    controllers.current.get(key)?.abort();
    const controller = new AbortController();
    controllers.current.set(key, controller);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await getTrust(controller.signal);
      if (controller.signal.aborted) return;
      dispatch({ type: 'trustLoaded', tiles: data.tiles });
    } catch (e) {
      if (controller.signal.aborted) return;
      dispatch({
        type: 'reqFail',
        key,
        message: isNetworkError(e)
          ? 'Unable to reach the server. Please check your connection.'
          : e instanceof Error
          ? e.message
          : 'Something went wrong.',
      });
    } finally {
      inflight.current.delete(key);
    }
  }, []);

  /** Loads the full product list from GET /products on boot. */
  const loadProducts = useCallback(async () => {
    const key: RequestKey = 'products';

    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    controllers.current.get(key)?.abort();
    const controller = new AbortController();
    controllers.current.set(key, controller);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await getProducts();
      if (controller.signal.aborted) return;
      dispatch({ type: 'productsLoaded', products: data.products });
    } catch (e) {
      if (controller.signal.aborted) return;
      dispatch({
        type: 'reqFail',
        key,
        message: isNetworkError(e)
          ? 'Unable to reach the server. Please check your connection.'
          : e instanceof Error
          ? e.message
          : 'Something went wrong.',
      });
    } finally {
      inflight.current.delete(key);
    }
  }, []);

  /**
   * Loads a PDP's detail payload from GET /products/{id}.
   * Aborts any prior product load so fast navigation settles on the last click.
   */
  const loadProduct = useCallback(async (id: string) => {
    const key: RequestKey = 'product';

    // Abort the previous product load; unlike the grid we replace rather than gate.
    controllers.current.get(key)?.abort();
    const controller = new AbortController();
    controllers.current.set(key, controller);
    inflight.current.delete(key);
    inflight.current.add(key);

    dispatch({ type: 'reqStart', key });
    try {
      const detail = await getProduct(id);
      if (controller.signal.aborted) return;
      dispatch({ type: 'productLoaded', detail });
    } catch (e) {
      if (controller.signal.aborted) return;
      dispatch({
        type: 'reqFail',
        key,
        message: isNetworkError(e)
          ? 'Unable to reach the server. Please check your connection.'
          : e instanceof Error
          ? e.message
          : 'Something went wrong.',
      });
    } finally {
      inflight.current.delete(key);
    }
  }, []);

  /** Loads the shopper's cart from GET /cart. */
  const loadCart = useCallback(async () => {
    await cartMutation(getCart);
  }, [cartMutation]);

  /** Loads saved addresses from GET /me/addresses. No-op if already in flight. */
  const loadAddresses = useCallback(async () => {
    const key: RequestKey = 'addresses';
    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await listAddresses();
      dispatch({ type: 'addressesLoaded', addresses: data.addresses });
    } catch (e) {
      dispatch({
        type: 'reqFail',
        key,
        message: isNetworkError(e)
          ? 'Unable to reach the server. Please check your connection.'
          : isApiError(e)
          ? e.message
          : e instanceof Error
          ? e.message
          : 'Something went wrong.',
      });
    } finally {
      inflight.current.delete(key);
    }
  }, []);

  /**
   * Restores the session on boot. Reads the token from localStorage:
   * - absent → stays signed out, no banner.
   * - 401 → stale token; clear it silently, no banner.
   * - network error → keep the token, show banner. A flaky connection
   *   must never silently sign someone out.
   *
   * When a session is found, loads cart and addresses in parallel.
   */
  const restoreSession = useCallback(async () => {
    const token = getToken();
    if (!token) return;

    try {
      const data = await getMe();
      dispatch({ type: 'sessionRestored', user: data.user });
      await Promise.all([loadCart(), loadAddresses()]);
    } catch (e) {
      if (isApiError(e) && e.status === 401) {
        clearToken();
      } else {
        dispatch({
          type: 'reqFail',
          key: 'session',
          message: isNetworkError(e)
            ? 'Unable to reach the server. Please check your connection.'
            : e instanceof Error
            ? e.message
            : 'Something went wrong.',
        });
      }
    }
  }, [loadCart, loadAddresses]);

  // Boot effect: load trust tiles, full product list, and restore any existing
  // session in parallel. Also loads the offline cart buffer from localStorage.
  // The booted ref ensures exactly one boot even under React StrictMode's
  // double-invoke.
  useEffect(() => {
    if (booted.current) return;
    booted.current = true;

    // Restore any buffer the shopper left in a previous browse session.
    const storedBuffer = loadBuffer();
    if (storedBuffer.length > 0) {
      dispatch({ type: 'setCartBuffer', buffer: storedBuffer });
    }

    void Promise.all([loadTrust(), loadProducts(), restoreSession()]).finally(() =>
      dispatch({ type: 'bootDone' }),
    );
  }, [loadTrust, loadProducts, restoreSession]);

  /** POST /auth/otp/request with the phone currently in state. */
  const sendOtp = useCallback(async () => {
    const phone = state.phone;

    if (phone.length !== 10) {
      dispatch({ type: 'otpRequestFailed', phoneError: 'Enter a valid 10-digit mobile number' });
      return;
    }

    const key: RequestKey = 'otpRequest';
    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await requestOtp(phone);
      dispatch({
        type: 'otpRequested',
        resendAfterSeconds: data.resendAfterSeconds,
        devCode: data.devCode,
      });
    } catch (e) {
      if (isApiError(e) && e.status === 422) {
        const msg = fieldError(e, 'phone') ?? e.message;
        dispatch({ type: 'otpRequestFailed', phoneError: msg });
      } else if (isApiError(e) && e.status === 429) {
        dispatch({ type: 'reqFail', key, message: e.message });
      } else {
        dispatch({
          type: 'reqFail',
          key,
          message: isNetworkError(e)
            ? 'Unable to reach the server. Please check your connection.'
            : e instanceof Error
            ? e.message
            : 'Something went wrong.',
        });
      }
    } finally {
      inflight.current.delete(key);
    }
  }, [state.phone]);

  /**
   * POST /auth/otp/verify with the phone + otp currently in state.
   *
   * After a successful verify:
   * 1. Set the JWT in localStorage.
   * 2. Load the cart and addresses in parallel.
   * 3. Replay every buffered add-to-cart intent in order, collecting failures.
   * 4. Clear the buffer from state and localStorage.
   * 5. If any item failed (e.g. sold out), show a specific banner.
   * 6. Dispatch otpVerified (which navigates to state.next).
   */
  const verifyOtp = useCallback(async () => {
    const phone = state.phone;
    const otp = state.otp;
    const cartBuffer = state.cartBuffer;
    const products = state.products;

    if (otp.length !== 6) {
      dispatch({ type: 'otpVerifyFailed', otpError: 'Enter the 6-digit code' });
      return;
    }

    const key: RequestKey = 'otpVerify';
    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await apiVerifyOtp(phone, otp);
      setToken(data.token);

      // Load cart and addresses in parallel before navigating.
      await Promise.all([loadCart(), loadAddresses()]);

      // Replay buffered Add intents from the signed-out browse session.
      if (cartBuffer.length > 0) {
        const failedNames: string[] = [];
        for (const item of cartBuffer) {
          try {
            const cart = await apiAddItem(item.productId, item.qty);
            dispatch({ type: 'cartLoaded', cart });
          } catch (e) {
            // Best effort: continue with the rest of the buffer regardless.
            if (isApiError(e) && (e.status === 422 || e.status === 409)) {
              const product = products.find((p) => p.id === item.productId);
              failedNames.push(product?.name ?? item.productId);
            }
          }
        }
        // Clear buffer regardless of individual failures — partial replay is
        // still progress, and retrying would double-add successful items.
        saveBuffer([]);
        dispatch({ type: 'clearCartBuffer' });

        if (failedNames.length > 0) {
          const names = failedNames.join(', ');
          dispatch({
            type: 'reqFail',
            key: 'cart',
            message:
              failedNames.length === 1
                ? `"${names}" sold out — it could not be added to your cart.`
                : `These items sold out and could not be added: ${names}.`,
          });
        }
      }

      dispatch({ type: 'otpVerified', user: data.user });
    } catch (e) {
      if (isApiError(e) && e.status === 401) {
        dispatch({ type: 'otpVerifyFailed', otpError: e.message });
      } else if (isApiError(e) && e.status === 422) {
        const msg = fieldError(e, 'code') ?? e.message;
        dispatch({ type: 'otpVerifyFailed', otpError: msg });
      } else if (isApiError(e) && e.status === 429) {
        dispatch({ type: 'reqFail', key, message: e.message });
      } else {
        dispatch({
          type: 'reqFail',
          key,
          message: isNetworkError(e)
            ? 'Unable to reach the server. Please check your connection.'
            : e instanceof Error
            ? e.message
            : 'Something went wrong.',
        });
      }
    } finally {
      inflight.current.delete(key);
    }
  }, [state.phone, state.otp, state.cartBuffer, state.products, loadCart, loadAddresses]);

  /**
   * Exchanges the MSG91 widget access token for our session JWT, then hydrates
   * the session: loads cart + addresses in parallel, replays the offline cart
   * buffer, and navigates to state.next. Errors surface as the screen banner.
   *
   * Error mapping:
   *   422 → "Please sign in again." (empty/invalid accessToken)
   *   401 → server message ("We could not verify that sign-in…")
   *   502 → provider unreachable; invite retry, not a credentials failure
   */
  const signInWithWidget = useCallback(async (accessToken: string) => {
    const cartBuffer = state.cartBuffer;
    const products = state.products;

    const key: RequestKey = 'widgetSession';
    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await exchangeSession(accessToken);
      setToken(data.token);

      await Promise.all([loadCart(), loadAddresses()]);

      if (cartBuffer.length > 0) {
        const failedNames: string[] = [];
        for (const item of cartBuffer) {
          try {
            const cart = await apiAddItem(item.productId, item.qty);
            dispatch({ type: 'cartLoaded', cart });
          } catch (e) {
            if (isApiError(e) && (e.status === 422 || e.status === 409)) {
              const product = products.find((p) => p.id === item.productId);
              failedNames.push(product?.name ?? item.productId);
            }
          }
        }
        saveBuffer([]);
        dispatch({ type: 'clearCartBuffer' });

        if (failedNames.length > 0) {
          const names = failedNames.join(', ');
          dispatch({
            type: 'reqFail',
            key: 'cart',
            message:
              failedNames.length === 1
                ? `"${names}" sold out — it could not be added to your cart.`
                : `These items sold out and could not be added: ${names}.`,
          });
        }
      }

      dispatch({ type: 'otpVerified', user: data.user });
    } catch (e) {
      if (isApiError(e) && e.status === 422) {
        dispatch({ type: 'reqFail', key, message: e.message });
      } else if (isApiError(e) && e.status === 401) {
        dispatch({ type: 'reqFail', key, message: e.message });
      } else if (isApiError(e) && e.status === 502) {
        dispatch({
          type: 'reqFail',
          key,
          message:
            'Sign-in provider is temporarily unreachable. Please try again in a moment.',
        });
      } else {
        dispatch({
          type: 'reqFail',
          key,
          message: isNetworkError(e)
            ? 'Unable to reach the server. Please check your connection.'
            : isApiError(e)
            ? e.message
            : e instanceof Error
            ? e.message
            : 'Something went wrong.',
        });
      }
    } finally {
      inflight.current.delete(key);
    }
  }, [state.cartBuffer, state.products, loadCart, loadAddresses]);

  /**
   * Adds a product to the cart.
   * - Signed out: appends/increments the offline buffer, stays on the current
   *   screen. No auth gate. The prompt guides the shopper to sign in when ready.
   * - Signed in: POST /cart/items immediately.
   */
  const addItem = useCallback(
    async (productId: string, qty: number) => {
      if (!state.user) {
        const product = state.products.find((p) => p.id === productId);
        if (!product || product.soldOut) return;
        const newBuffer = applyBufferAdd(state.cartBuffer, productId, qty);
        saveBuffer(newBuffer);
        dispatch({ type: 'setCartBuffer', buffer: newBuffer });
        return;
      }
      await cartMutation(() => apiAddItem(productId, qty));
    },
    [state.user, state.products, state.cartBuffer, cartMutation],
  );

  /**
   * PDP-specific add: sources productId and qty from state.
   * - Signed out: appends/increments the offline buffer, stays on the PDP.
   * - Signed in: POST /cart/items then navigates to the cart.
   */
  const addFromPdp = useCallback(async () => {
    const p = state.products.find((pr) => pr.id === state.pdp);
    if (!p || p.soldOut) return;

    if (!state.user) {
      const qty = Math.min(99, state.qty);
      const newBuffer = applyBufferAdd(state.cartBuffer, p.id, qty);
      saveBuffer(newBuffer);
      dispatch({ type: 'setCartBuffer', buffer: newBuffer });
      return;
    }

    const result = await cartMutation(() => apiAddItem(p.id, state.qty));
    if (result !== null) {
      dispatch({ type: 'go', screen: 'cart' });
    }
  }, [state.products, state.pdp, state.qty, state.user, state.cartBuffer, cartMutation]);

  /** Sets the absolute quantity on a cart line (qty=0 removes it). */
  const setQty = useCallback(
    async (productId: string, qty: number) => {
      await cartMutation(() => apiSetQty(productId, qty));
    },
    [cartMutation],
  );

  /** Removes a line from the cart. */
  const removeItem = useCallback(
    async (productId: string) => {
      await cartMutation(() => apiRemoveItem(productId));
    },
    [cartMutation],
  );

  /** Empties the cart. */
  const clearCart = useCallback(async () => {
    await cartMutation(apiClearCart);
  }, [cartMutation]);

  /**
   * Sets the qty of a product in the offline buffer (signed-out only).
   * qty=0 removes the product. Persists to localStorage immediately.
   */
  const setBufferQty = useCallback(
    (productId: string, qty: number) => {
      const newBuffer = applyBufferSetQty(state.cartBuffer, productId, qty);
      saveBuffer(newBuffer);
      dispatch({ type: 'setCartBuffer', buffer: newBuffer });
    },
    [state.cartBuffer],
  );

  /**
   * POSTs a new address or PUTs an existing one. Client validates first.
   * Server field errors are mapped back onto the form inputs via setAddrFieldError.
   */
  const saveAddress = useCallback(
    async (addr: Address) => {
      const key: RequestKey = 'addressSave';
      if (inflight.current.has(key)) return;
      inflight.current.add(key);

      const editingId = state.editingAddressId;

      dispatch({ type: 'reqStart', key });
      try {
        const body = {
          name: addr.name,
          email: addr.email,
          line1: addr.line1,
          city: addr.city,
          state: addr.state,
          pin: addr.pin,
          isDefault: false,
        };
        const data = editingId
          ? await apiUpdateAddress(editingId, body)
          : await apiAddAddress(body);
        dispatch({ type: 'addressSaved', address: data.address });
        // Reload the full list so ordering and isDefault flag are fresh.
        await loadAddresses();
      } catch (e) {
        if (isApiError(e) && (e.status === 422 || e.status === 400)) {
          // Map per-field server errors onto the form.
          if (e.fields.length > 0) {
            for (const fe of e.fields) {
              const field = fe.field as keyof Address;
              dispatch({ type: 'setAddrFieldError', field, message: fe.message });
            }
          } else {
            dispatch({ type: 'reqFail', key, message: e.message });
          }
        } else {
          dispatch({
            type: 'reqFail',
            key,
            message: isNetworkError(e)
              ? 'Unable to reach the server. Please check your connection.'
              : isApiError(e)
              ? e.message
              : e instanceof Error
              ? e.message
              : 'Something went wrong.',
          });
        }
      } finally {
        inflight.current.delete(key);
      }
    },
    [state.editingAddressId, loadAddresses],
  );

  /** Deletes an address; server returns the remaining list. */
  const deleteAddress = useCallback(
    async (id: string) => {
      const key: RequestKey = 'addressDelete';
      if (inflight.current.has(key)) return;
      inflight.current.add(key);

      dispatch({ type: 'reqStart', key });
      try {
        const data = await apiRemoveAddress(id);
        dispatch({ type: 'addressDeleted', addresses: data.addresses });
      } catch (e) {
        dispatch({
          type: 'reqFail',
          key,
          message: isNetworkError(e)
            ? 'Unable to reach the server. Please check your connection.'
            : isApiError(e)
            ? e.message
            : e instanceof Error
            ? e.message
            : 'Something went wrong.',
        });
      } finally {
        inflight.current.delete(key);
      }
    },
    [],
  );

  /**
   * POST /orders → open the Razorpay Checkout modal.
   *
   * The Pay button is gated on razorpayReady (Script.onReady) in ReviewScreen;
   * this command also re-checks window.Razorpay at call time to handle ad-blocker
   * scenarios where neither onReady nor onError fires.
   *
   * Distinct POST /orders error handling:
   *   409 empty cart → route to shop
   *   409 other      → surface server message (blocking line, concurrent conflict)
   *   422            → route to cart (no address)
   *   404            → reload addresses + route to cart
   *   502            → transient gateway failure, invite retry
   *
   * Payment callbacks (defined here, capture all needed values in closure):
   *   handler        → confirmed payment; retry/poll resilience ladder; paymentConfirmed
   *   ondismiss      → modal closed; if settled ignore (handler already fired); else paymentCancelled
   *   payment.failed → record reason for ondismiss to surface; Razorpay keeps modal open for retry
   */
  const openCheckout = useCallback(async () => {
    const addressId = state.selectedAddressId;
    const user = state.user;
    const selectedAddr = state.addresses.find((a) => a.id === addressId);

    const key: RequestKey = 'checkout';
    if (inflight.current.has(key)) return;
    inflight.current.add(key);

    dispatch({ type: 'reqStart', key });
    try {
      const data = await apiOpenCheckout(addressId ?? undefined);
      dispatch({ type: 'checkoutOpened', data });

      // Ad-blocker guard: if the script was blocked silently, neither onReady nor
      // onError fires. Detect here so the shopper gets an actionable message.
      if (typeof window.Razorpay !== 'function') {
        dispatch({
          type: 'reqFail',
          key,
          message: 'Payment provider failed to load. Please disable any ad blockers and refresh.',
        });
        return;
      }

      // These two closed-over variables are shared between handler and ondismiss.
      // `settled` prevents ondismiss from overwriting a completed payment.
      // `paymentFailReason` carries the last failure reason into ondismiss.
      let settled = false;
      let paymentFailReason: string | null = null;

      const orderId = data.order.orderId;

      // Resilience ladder: payment was captured (Razorpay called handler) but our
      // callback POST may fail transiently. Never tell a paying shopper it failed.
      const confirmWithFallback = async (
        razorpayOrderId: string,
        razorpayPaymentId: string,
        razorpaySignature: string,
      ) => {
        const delay = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

        // Attempt 1
        try {
          const result = await apiConfirmPayment(orderId, razorpayOrderId, razorpayPaymentId, razorpaySignature);
          dispatch({ type: 'paymentConfirmed', order: result.order });
          inflight.current.delete('payment');
          await loadCart();
          return;
        } catch {
          // fall through to attempt 2
        }

        // Attempt 2 after ~1 s
        await delay(1000);
        try {
          const result = await apiConfirmPayment(orderId, razorpayOrderId, razorpayPaymentId, razorpaySignature);
          dispatch({ type: 'paymentConfirmed', order: result.order });
          inflight.current.delete('payment');
          await loadCart();
          return;
        } catch {
          // fall through to polling
        }

        // Poll GET /orders up to 3 times looking for status === 'placed'
        for (let i = 0; i < 3; i++) {
          await delay(i === 0 ? 1500 : 2000);
          try {
            const result = await apiGetOrder(orderId);
            if (result.order.status === 'placed') {
              dispatch({ type: 'paymentConfirmed', order: result.order });
              inflight.current.delete('payment');
              await loadCart();
              return;
            }
          } catch {
            // continue polling
          }
        }

        // Degraded success: payment was captured (Razorpay called handler), but
        // we couldn't confirm server-side. The webhook will place the order shortly.
        // Show a reassuring screen rather than a failure.
        dispatch({ type: 'paymentConfirmed', order: data.order });
        inflight.current.delete('payment');
        await loadCart();
      };

      const options: RazorpayOptions = {
        key: data.razorpay.keyId,
        amount: data.razorpay.amount, // paise — passed through UNCHANGED; do not divide
        currency: data.razorpay.currency,
        order_id: data.razorpay.razorpayOrderId,
        name: 'Enerzeia Future Farm',
        description: orderId,
        prefill: {
          contact: '+91' + (user?.phone ?? ''),
          name: selectedAddr?.name,
          email: selectedAddr?.email,
        },
        notes: { orderId },
        theme: { color: '#0e6b52' },
        handler: (response) => {
          // MUST be first — ondismiss fires AFTER handler when the sheet closes.
          settled = true;
          void confirmWithFallback(
            response.razorpay_order_id,
            response.razorpay_payment_id,
            response.razorpay_signature,
          );
        },
        modal: {
          ondismiss: () => {
            // If handler already fired, the payment succeeded — do not overwrite.
            if (settled) return;
            inflight.current.delete('payment');
            const message = paymentFailReason
              ? `That payment didn't go through — you can try again.`
              : 'Payment cancelled — your cart is still here.';
            dispatch({ type: 'paymentCancelled', message });
          },
        },
      };

      const rzp = new window.Razorpay(options);
      rzp.on('payment.failed', (resp) => {
        // Razorpay keeps the modal open for a retry — do NOT navigate.
        paymentFailReason = resp.error.description;
      });

      dispatch({ type: 'reqStart', key: 'payment' });
      inflight.current.add('payment');
      rzp.open();
    } catch (e) {
      if (isApiError(e) && e.status === 409) {
        if (e.message === 'Your cart is empty.') {
          dispatch({ type: 'go', screen: 'shop' });
          dispatch({ type: 'reqFail', key, message: 'Your cart is empty. Browse products to add items.' });
        } else {
          dispatch({ type: 'reqFail', key, message: e.message });
        }
      } else if (isApiError(e) && e.status === 422) {
        dispatch({ type: 'go', screen: 'cart' });
        dispatch({ type: 'reqFail', key, message: 'Please add a delivery address before continuing.' });
      } else if (isApiError(e) && e.status === 404) {
        dispatch({ type: 'go', screen: 'cart' });
        await loadAddresses();
        dispatch({ type: 'reqFail', key, message: 'The selected address was not found. Please choose another.' });
      } else if (isApiError(e) && e.status === 502) {
        dispatch({
          type: 'reqFail',
          key,
          message: 'The payment provider is temporarily unreachable. Please try again in a moment.',
        });
      } else {
        dispatch({
          type: 'reqFail',
          key,
          message: isNetworkError(e)
            ? 'Unable to reach the server. Please check your connection.'
            : isApiError(e)
            ? e.message
            : e instanceof Error
            ? e.message
            : 'Something went wrong.',
        });
      }
    } finally {
      // Release 'checkout' — the modal is now open; 'payment' key guards from here.
      inflight.current.delete(key);
    }
  }, [state.selectedAddressId, state.user, state.addresses, loadAddresses, loadCart]);

  const actions: ShopActions = {
    sendOtp,
    verifyOtp,
    signInWithWidget,
    loadProduct,
    loadCart,
    addItem,
    addFromPdp,
    setQty,
    removeItem,
    clearCart,
    setBufferQty,
    loadAddresses,
    saveAddress,
    deleteAddress,
    openCheckout,
    loadOrders,
    signOut: performSignOut,
  };
  return { state, dispatch, actions };
}
