import { request } from './http';
import type { CheckoutData, OrderData, OrdersData } from './types';

/**
 * Opens a checkout: reserves stock, creates a Razorpay order and returns both
 * the internal order and the Razorpay parameters for Checkout.init(). Omit
 * `addressId` to use the shopper's default address. Requires auth.
 */
export async function openCheckout(addressId?: string): Promise<CheckoutData> {
  const body: { addressId?: string } = {};
  if (addressId) body.addressId = addressId;
  return request<CheckoutData>('POST', '/orders', body);
}

/**
 * Confirms payment after Razorpay Checkout returns success on the browser.
 * Verifies the HMAC signature server-side and moves the order to `placed`.
 * Requires auth.
 */
export async function confirmPayment(
  orderId: string,
  razorpayOrderId: string,
  razorpayPaymentId: string,
  razorpaySignature: string,
): Promise<OrderData> {
  return request<OrderData>(
    'POST',
    `/orders/${encodeURIComponent(orderId)}/payment/callback`,
    { razorpayOrderId, razorpayPaymentId, razorpaySignature },
  );
}

/** Fetches a single order. Returns 404 if it does not exist or belongs to another user. Requires auth. */
export async function getOrder(orderId: string): Promise<OrderData> {
  return request<OrderData>('GET', `/orders/${encodeURIComponent(orderId)}`);
}

/** Fetches the signed-in shopper's orders, newest first. Requires auth. */
export async function listOrders(): Promise<OrdersData> {
  return request<OrdersData>('GET', '/orders');
}
