import type {
  AddressDTO,
  CartDTO,
  CartLineDTO,
  CheckoutData,
  FreeShippingDTO,
  OrderDTO,
  OrderLineDTO,
  OrderTotalsDTO,
  PaymentDTO,
  ProductDetailData,
  ProductDTO,
  TrustTileDTO,
  UserDTO,
} from '@/lib/api/types';

export type { AddressDTO, CartDTO, CartLineDTO, CheckoutData, FreeShippingDTO, OrderDTO, OrderLineDTO, OrderTotalsDTO, PaymentDTO, ProductDetailData, ProductDTO, TrustTileDTO, UserDTO };

export type Screen = 'shop' | 'pdp' | 'login' | 'cart' | 'review' | 'done' | 'orders' | 'addresses' | 'contact';

/** Screens the auth gate can send a shopper to once they sign in. */
export type GatedScreen = Extract<Screen, 'cart' | 'review'>;

/**
 * ProductId is a plain string alias — the catalogue is server data, so a
 * literal union would compile-break on a seed change and give false confidence
 * about ids arriving over the wire.
 */
export type ProductId = string;

export type ProductForm = 'Powder' | 'Tablets' | 'Bundle';

export type Filter = 'all' | ProductForm;

/**
 * Closed union of every in-flight request key. Declaring all keys now avoids
 * churn as later tasks fill them in. `pending` must be a full Record over
 * this union so noUncheckedIndexedAccess yields `boolean`, not `boolean | undefined`.
 */
export type RequestKey =
  | 'products'
  | 'product'
  | 'trust'
  | 'otpRequest'
  | 'otpVerify'
  | 'session'
  | 'cart'
  | 'addresses'
  | 'addressSave'
  | 'addressDelete'
  | 'checkout'
  | 'payment'
  | 'widgetSession'
  | 'orders';

/** Frozen cart line stored in a placed order (7.7 will replace with OrderLineDTO). */
export interface CartLine {
  key: string;
  id: string;
  name: string;
  grad: string;
  variant: string;
  sub: string;
  /** paise */
  price: number;
  /** paise */
  mrp: number;
  qty: number;
}

export interface Address {
  name: string;
  email: string;
  line1: string;
  city: string;
  state: string;
  pin: string;
}

export interface ShopState {
  screen: Screen;
  filter: Filter;
  /** Live server cart. Null before the first load (boot or sign-in). */
  cart: CartDTO | null;
  /**
   * Offline selection buffer. Items added while signed out land here instead of
   * routing to login. Replayed sequentially after a successful OTP verify, then
   * cleared. Persisted to localStorage so a mid-browse reload does not lose the
   * selection. Cleared on sign-out.
   */
  cartBuffer: { productId: string; qty: number }[];
  /** Product id of the currently open PDP, or null. */
  pdp: string | null;
  /** Full product list loaded from GET /products on boot. */
  products: readonly ProductDTO[];
  /** Detail payload from GET /products/{id}. Null while loading or between navigations. */
  pdpDetail: ProductDetailData | null;
  qty: number;
  /** Signed-in shopper, or null. */
  user: UserDTO | null;
  phone: string;
  otp: string;
  otpSent: boolean;
  /** False when clear; server error message when set. */
  phoneError: string | false;
  /** False when clear; server error message when set. */
  otpError: string | false;
  /** Seconds the shopper must wait before they can resend. 0 = no cooldown. */
  resendAfterSeconds: number;
  /**
   * Incremented on every successful OTP request. Components watch this to
   * restart their countdown timer without Date.now in the reducer.
   */
  otpResendNonce: number;
  /** Dev code from the server (non-production only). Absent when not present. */
  otpDevCode: string | undefined;
  /** Saved delivery addresses loaded from GET /me/addresses. Default is first. */
  addresses: AddressDTO[];
  /** Id of the address the shopper has selected for checkout. */
  selectedAddressId: string | null;
  /** Whether the add/edit address form is currently visible. */
  showAddressForm: boolean;
  /** Id being edited, or null when the form is for a new address. */
  editingAddressId: string | null;
  /** Per-field server validation errors for the address form. */
  addrFieldErrors: Partial<Record<keyof Address, string>>;
  addrError: string;
  next: GatedScreen;
  addr: Address;
  /** Full response from POST /orders. Null until openCheckout succeeds. */
  checkout: CheckoutData | null;
  /** Placed order from the payment callback or polling fallback. Drives DoneScreen. */
  placedOrder: OrderDTO | null;
  /**
   * Inline message shown on the review screen after a dismissed or failed payment.
   * Null when no message. Cleared when the next POST /orders succeeds.
   */
  paymentCancelled: string | null;
  /** True until the boot sequence (trust load + products + session restore) settles. */
  booting: boolean;
  /**
   * In-flight flags per request key. Full Record — never Partial — so that
   * noUncheckedIndexedAccess yields `boolean` not `boolean | undefined`.
   */
  pending: Record<RequestKey, boolean>;
  /** Screen-level error shown as a banner. Null when there is nothing to report. */
  banner: string | null;
  /** Trust strip tiles loaded from GET /content/trust on boot. */
  trust: TrustTileDTO[];
  /** Shopper's order history, newest first. Empty before first load on the orders screen. */
  orders: readonly OrderDTO[];
}
