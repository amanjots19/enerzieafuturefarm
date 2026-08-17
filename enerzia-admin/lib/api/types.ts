/**
 * Wire DTOs — these mirror the JSON tags on the Go handler structs exactly.
 * Money fields are int64 paise; never convert them inside this module.
 */

/** Response body from POST /admin/login. */
export interface LoginData {
  token: string;
  expiresAt: string;
  email: string;
}

/** Response body from GET /admin/me. */
export interface MeData {
  email: string;
}

/** Response body from POST /admin/uploads/signature. */
export interface UploadSignatureData {
  cloudName: string;
  apiKey: string;
  timestamp: number;
  folder: string;
  signature: string;
}

/** One product image. */
export interface AdminImage {
  url: string;
  publicId: string;
  alt: string;
}

/** Aggregate review score. */
export interface AdminRating {
  score: number;
  count: number;
}

/** Reassurance tile shown alongside a product. */
export interface AdminBadge {
  title: string;
  subtitle: string;
}

/** One row of the nutrition breakdown table. */
export interface NutritionRow {
  key: string;
  value: string;
}

/** Per-serving nutrition information. */
export interface AdminNutrition {
  servingSize: string;
  rows: NutritionRow[];
}

/**
 * Full admin product object returned by every admin product endpoint.
 * Includes fields the shopper view omits (active, position, rating, badges,
 * nutrition) because the console edits them.
 *
 * discountPercent and soldOut are computed on the way out and rejected on the
 * way in — never send them in a write body.
 */
export interface AdminProduct {
  id: string;
  family: string;
  form: string;
  name: string;
  stat: string;
  stat2: string;
  blurb: string;
  grad: string;
  position: number;
  images: AdminImage[];
  /** paise */
  mrp: number;
  /** paise */
  price: number;
  /** computed; never send on write */
  discountPercent: number;
  stock: number;
  /** computed; never send on write */
  soldOut: boolean;
  active: boolean;
  rating: AdminRating;
  badges: AdminBadge[];
  nutrition: AdminNutrition | null;
}

/** Response body from GET /admin/products. */
export interface AdminProductListData {
  products: AdminProduct[];
}

/* ------------------------------------------------------------------ orders */

export interface AdminOrderLine {
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

export interface AdminOrderTotals {
  /** paise */
  mrpTotal: number;
  /** paise */
  subtotal: number;
  /** paise */
  savings: number;
  /** paise */
  shipping: number;
  /** paise */
  total: number;
}

export interface AdminShippingAddress {
  name: string;
  email: string;
  phone?: string;
  line1: string;
  city: string;
  state: string;
  pin: string;
}

/** null means payment not yet recorded on this order (e.g. pending_payment). */
export interface AdminPayment {
  method: string | null;
  label: string | null;
  last4: string | null;
  network: string | null;
  vpa: string | null;
}

export interface AdminCustomer {
  userId: string;
  /** null on orders placed before the field existed (task 11.10). */
  phone: string | null;
}

export interface AdminOrder {
  orderId: string;
  status: string;
  statusLabel: string;
  /** null means fulfilment has not started. */
  fulfilment: string | null;
  fulfilmentLabel: string;
  createdAt: string;
  /** null on orders not yet placed (pending_payment). */
  placedAt: string | null;
  lines: AdminOrderLine[];
  totals: AdminOrderTotals;
  shippingAddress: AdminShippingAddress;
  /** null while order is pending_payment; method may itself be null even when present. */
  payment: AdminPayment | null;
  customer: AdminCustomer;
  razorpayOrderId: string | null;
  razorpayPaymentId: string | null;
}

export interface AdminOrderListData {
  orders: AdminOrder[];
  /** RFC3339 cursor for the next page; absent on the last page. */
  nextBefore: string | undefined;
  /** Number of orders on this page, NOT a total. */
  count: number;
}
