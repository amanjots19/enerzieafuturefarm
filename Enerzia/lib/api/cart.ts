import { request } from './http';
import type { CartDTO } from './types';

/** Returns the shopper's cart. Empty cart is 200 with `lines: []`. Requires auth. */
export async function getCart(): Promise<CartDTO> {
  return request<CartDTO>('GET', '/cart');
}

/**
 * Adds a product to the cart. If the product is already present the quantity
 * is incremented. Defaults to qty 1 when omitted. Requires auth.
 */
export async function addItem(productId: string, qty?: number): Promise<CartDTO> {
  return request<CartDTO>('POST', '/cart/items', { productId, qty });
}

/**
 * Sets the absolute quantity of a cart line. `qty: 0` removes the line.
 * Requires auth.
 */
export async function setQty(productId: string, qty: number): Promise<CartDTO> {
  return request<CartDTO>('PATCH', `/cart/items/${encodeURIComponent(productId)}`, { qty });
}

/** Removes a line from the cart entirely. Requires auth. */
export async function removeItem(productId: string): Promise<CartDTO> {
  return request<CartDTO>('DELETE', `/cart/items/${encodeURIComponent(productId)}`);
}

/** Empties the cart. Requires auth. */
export async function clearCart(): Promise<CartDTO> {
  return request<CartDTO>('DELETE', '/cart');
}
