/** Razorpay Checkout JS — hand-written type declarations for strict TypeScript. */

interface RazorpayOptions {
  /** Public key ID from POST /orders response (razorpay.keyId). */
  key: string;
  /** Order amount in PAISE — passed through unchanged from the server. Do NOT divide by 100. */
  amount: number;
  currency: string;
  /** Razorpay order ID from POST /orders response (razorpay.razorpayOrderId). */
  order_id: string;
  name?: string;
  description?: string;
  prefill?: {
    name?: string;
    email?: string;
    contact?: string;
  };
  notes?: Record<string, string>;
  theme?: {
    color?: string;
  };
  /** Called by Razorpay outside React's event system on a successful payment. */
  handler?: (response: RazorpayHandlerResponse) => void;
  modal?: {
    /** Called when the shopper closes the modal (success or cancel). */
    ondismiss?: () => void;
  };
}

interface RazorpayHandlerResponse {
  razorpay_payment_id: string;
  razorpay_order_id: string;
  razorpay_signature: string;
}

interface RazorpayPaymentFailedResponse {
  error: {
    code: string;
    description: string;
    source: string;
    step: string;
    reason: string;
    metadata: {
      order_id: string;
      payment_id: string;
    };
  };
}

interface RazorpayInstance {
  open(): void;
  on(event: 'payment.failed', handler: (resp: RazorpayPaymentFailedResponse) => void): void;
}

interface Window {
  /**
   * Optional — Razorpay may be blocked by an ad blocker or fail to load.
   * Always check `typeof window.Razorpay === 'function'` before constructing.
   */
  Razorpay?: new (options: RazorpayOptions) => RazorpayInstance;
}
