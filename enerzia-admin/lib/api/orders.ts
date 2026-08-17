import { API_BASE } from './config';
import { ApiError, type ErrorCode } from './errors';
import { request } from './http';
import { fireUnauthorized, getToken } from './token';
import type { AdminOrder, AdminOrderListData } from './types';

export interface ListOrdersParams {
  /** One status, a comma-separated set, or "all". Omit to use the server default. */
  status?: string;
  /** "none", "packed", "in_transit", "shipped", or a comma-separated set. */
  fulfilment?: string;
  /** 1–200, default 50. */
  limit?: number;
  /** RFC3339 createdAt cursor; returns orders strictly older than this. */
  before?: string;
}

/**
 * Lists admin orders. Returns the full response envelope including nextBefore
 * and count so the caller can drive cursor-based pagination.
 */
export async function listOrders(params: ListOrdersParams = {}): Promise<AdminOrderListData> {
  const qs = new URLSearchParams();
  if (params.status !== undefined && params.status !== '') qs.set('status', params.status);
  if (params.fulfilment !== undefined && params.fulfilment !== '') qs.set('fulfilment', params.fulfilment);
  if (params.limit !== undefined) qs.set('limit', String(params.limit));
  if (params.before !== undefined) qs.set('before', params.before);
  const query = qs.toString();
  return request<AdminOrderListData>('GET', '/admin/orders' + (query ? '?' + query : ''));
}

/** One order, by its customer-facing id. Throws ApiError(404) if it does not exist. */
export async function getOrder(orderId: string): Promise<AdminOrder> {
  const data = await request<{ order: AdminOrder }>(
    'GET',
    `/admin/orders/${encodeURIComponent(orderId)}`,
  );
  return data.order;
}

/**
 * Advances an order's fulfilment by exactly one step and returns the updated
 * order.
 *
 * The server refuses anything else: backwards, skipping a step, re-sending the
 * current state, or an order nobody paid for all answer 409 with a message that
 * names what IS allowed. Show that message rather than inventing one — and do
 * not pre-compute the next step client-side as though it were authoritative.
 */
export async function setFulfilment(orderId: string, fulfilment: string): Promise<AdminOrder> {
  const data = await request<{ order: AdminOrder }>(
    'PATCH',
    `/admin/orders/${encodeURIComponent(orderId)}/fulfilment`,
    { fulfilment },
  );
  return data.order;
}

/**
 * Fetches a rendered 4×6 shipping label as HTML.
 *
 * Not routed through request(): that helper parses the JSON envelope, and this
 * endpoint returns a whole HTML document. Errors still arrive as the envelope,
 * so a failure is decoded here to surface the server's own message — 409 for an
 * unpaid order, 503 when SHIP_FROM_* is unset.
 *
 * The bearer token is why this cannot be a plain link: window.open on the
 * endpoint would send no Authorization header and get a 401.
 */
export async function fetchLabelHTML(orderId: string): Promise<string> {
  const token = getToken();
  const res = await fetch(`${API_BASE}/admin/orders/${encodeURIComponent(orderId)}/label`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });

  if (!res.ok) {
    if (res.status === 401) fireUnauthorized();
    let message = 'Could not generate the label.';
    let code: ErrorCode = 'INTERNAL';
    try {
      const body = (await res.json()) as { error?: { message?: string; code?: ErrorCode } };
      if (body.error?.message) message = body.error.message;
      if (body.error?.code) code = body.error.code;
    } catch {
      // A non-JSON error body is not worth surfacing raw.
    }
    throw new ApiError(code, message, res.status);
  }
  return res.text();
}
