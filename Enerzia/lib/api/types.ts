/**
 * Wire DTOs — these mirror the JSON tags on the Go handler structs exactly.
 * Money fields are int64 paise; never convert them inside this module.
 */

/** One product from GET /products or GET /products/{id}. */
export interface ProductDTO {
  id: string;
  family: string;
  form: string;
  name: string;
  stat: string;
  stat2: string;
  blurb: string;
  /** CSS gradient standing in for a product photograph. */
  grad: string;
  /** paise */
  mrp: number;
  /** paise */
  price: number;
  discountPercent: number;
  stock: number;
  soldOut: boolean;
}

/** Sibling cross-link entry from GET /products/{id}. */
export interface SiblingDTO {
  id: string;
  name: string;
  stat2: string;
  /** paise */
  mrp: number;
  /** paise */
  price: number;
  soldOut: boolean;
}

/** Aggregate review score. */
export interface RatingDTO {
  score: number;
  count: number;
}

/** Reassurance tile under the buy button. */
export interface BadgeDTO {
  title: string;
  subtitle: string;
}

/** One row of the nutrition table. */
export interface NutritionRowDTO {
  key: string;
  value: string;
}

/** Per-serving nutrition breakdown. */
export interface NutritionDTO {
  servingSize: string;
  rows: NutritionRowDTO[];
}

/** One trust-strip tile from GET /content/trust. */
export interface TrustTileDTO {
  big: string;
  body: string;
}

/** Response data of GET /products. */
export interface ProductListData {
  products: ProductDTO[];
}

/** Response data of GET /products/{id}. */
export interface ProductDetailData {
  product: ProductDTO;
  siblings: SiblingDTO[];
  rating: RatingDTO;
  badges: BadgeDTO[];
  nutrition: NutritionDTO;
}

/** Response data of GET /content/trust. */
export interface TrustData {
  tiles: TrustTileDTO[];
}

/** Response data of POST /auth/otp/request. */
export interface OtpRequestData {
  expiresInSeconds: number;
  resendAfterSeconds: number;
  /**
   * Only present outside production (APP_ENV != "production"). Key is ABSENT
   * in production responses — not null, just gone.
   */
  devCode?: string;
}

/** Authenticated shopper. */
export interface UserDTO {
  id: string;
  phone: string;
  /** ISO 8601 timestamp. */
  createdAt: string;
}

/** Response data of POST /auth/otp/verify. */
export interface OtpVerifyData {
  token: string;
  /** ISO 8601 timestamp. */
  expiresAt: string;
  user: UserDTO;
}

/** Response data of GET /auth/me. */
export interface MeData {
  user: UserDTO;
}

/** Response data of POST /auth/session (same shape as OtpVerifyData). */
export interface SessionData {
  token: string;
  /** ISO 8601 timestamp. */
  expiresAt: string;
  user: UserDTO;
}

/** A saved delivery address. */
export interface AddressDTO {
  id: string;
  /** Optional shopper-supplied label (omitempty — absent when empty). */
  label?: string;
  name: string;
  email: string;
  line1: string;
  city: string;
  state: string;
  pin: string;
  isDefault: boolean;
}

/** Request body for POST /me/addresses and PUT /me/addresses/{id}. */
export type AddressBody = Omit<AddressDTO, 'id'>;

/** Response data of GET /me/addresses and DELETE /me/addresses/{id}. */
export interface AddressListData {
  addresses: AddressDTO[];
}

/** Response data of POST /me/addresses and PUT /me/addresses/{id}. */
export interface AddressData {
  address: AddressDTO;
}

/** One line in the live cart. */
export interface CartLineDTO {
  productId: string;
  name: string;
  form: string;
  grad: string;
  stat2: string;
  /** paise */
  unitPrice: number;
  /** paise */
  unitMrp: number;
  qty: number;
  /** paise */
  lineTotal: number;
  stock: number;
  soldOut: boolean;
  /** Absent when false (omitempty) — product retired from the catalogue. */
  unavailable?: boolean;
}

/** Money summary of the cart. All fields are paise. */
export interface CartTotalsDTO {
  mrpTotal: number;
  subtotal: number;
  savings: number;
  shipping: number;
  total: number;
}

/** Free-shipping progress bar data. */
export interface FreeShippingDTO {
  /** paise */
  thresholdAmount: number;
  qualified: boolean;
  /** paise */
  remainingAmount: number;
}

/** Full cart view returned by every cart endpoint. */
export interface CartDTO {
  lines: CartLineDTO[];
  totals: CartTotalsDTO;
  freeShipping: FreeShippingDTO;
  itemCount: number;
  /** True when any line is sold out or exceeds available stock. */
  hasBlockingIssues: boolean;
}

/** One frozen line in a placed order. */
export interface OrderLineDTO {
  productId: string;
  name: string;
  form: string;
  grad: string;
  /** paise */
  unitPrice: number;
  /** paise */
  unitMrp: number;
  qty: number;
  /** paise */
  lineTotal: number;
}

/** Money summary of an order. All fields are paise. */
export interface OrderTotalsDTO {
  mrpTotal: number;
  subtotal: number;
  savings: number;
  shipping: number;
  total: number;
}

/**
 * Payment sub-object on a placed order. All five fields are string | null —
 * no omitempty. null means that method does not carry that detail
 * (e.g. `last4` is null for a UPI payment). Use `?? 'Paid online'` on label.
 */
export interface PaymentDTO {
  method: string | null;
  label: string | null;
  last4: string | null;
  network: string | null;
  vpa: string | null;
}

/**
 * An order from openCheckout or GET /orders/{orderId}. Fields `placedAt`,
 * `etaText`, and `payment` are absent (not null) until status === 'placed'.
 */
export interface OrderDTO {
  orderId: string;
  status: string;
  /** ISO 8601 timestamp. */
  createdAt: string;
  /** ISO 8601 timestamp. */
  expiresAt: string;
  /** Absent unless status === 'placed'. */
  placedAt?: string;
  /** Absent unless status === 'placed'. */
  etaText?: string;
  lines: OrderLineDTO[];
  totals: OrderTotalsDTO;
  /** Snapshot of the address at time of order. */
  shippingAddress: AddressDTO;
  /** Absent unless status === 'placed'. */
  payment?: PaymentDTO;
}

/** Razorpay checkout parameters from POST /orders. */
export interface RazorpayDTO {
  keyId: string;
  razorpayOrderId: string;
  /** paise — equals totals.total, computed server-side */
  amount: number;
  currency: string;
}

/** Response data of POST /orders. */
export interface CheckoutData {
  order: OrderDTO;
  razorpay: RazorpayDTO;
}

/** Response data of GET /orders/{orderId} and POST /orders/{orderId}/payment/callback. */
export interface OrderData {
  order: OrderDTO;
}

/** Response data of GET /orders (the list). */
export interface OrdersData {
  orders: OrderDTO[];
}
