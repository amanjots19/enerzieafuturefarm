export type Screen = 'shop' | 'pdp' | 'login' | 'cart' | 'checkout' | 'done';

/** Screens the auth gate can send a shopper to once they sign in. */
export type GatedScreen = Extract<Screen, 'cart' | 'checkout'>;

export type ProductId = 'powder' | 'tablets' | 'refill' | 'bundle';

export type ProductForm = 'Powder' | 'Tablets' | 'Bundle';

export type Filter = 'all' | ProductForm;

export type PayMethod = 'upi' | 'card' | 'cod';

export interface Variant {
  readonly label: string;
  readonly sub: string;
  readonly mrp: number;
  readonly price: number;
}

export interface Product {
  readonly id: ProductId;
  readonly form: ProductForm;
  readonly name: string;
  readonly stat: string;
  readonly stat2: string;
  readonly blurb: string;
  /** CSS gradient standing in for the product photograph. */
  readonly grad: string;
  readonly variants: readonly [Variant, ...Variant[]];
}

export interface CartLine {
  /** `${productId}|${variantLabel}` — identity for merging and quantity edits. */
  key: string;
  id: ProductId;
  name: string;
  grad: string;
  variant: string;
  sub: string;
  price: number;
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

export interface CardDetails {
  num: string;
  exp: string;
  cvv: string;
}

export interface Order {
  lines: readonly CartLine[];
  total: number;
  pay: PayMethod;
}

export interface Totals {
  mrp: number;
  sub: number;
  ship: number;
  total: number;
  save: number;
}

export interface ShopState {
  screen: Screen;
  filter: Filter;
  /** Selected variant index per product, shared by the grid and the PDP. */
  sel: Record<ProductId, number>;
  cart: CartLine[];
  pdp: ProductId | null;
  qty: number;
  /** Signed-in phone number, or null. */
  user: string | null;
  phone: string;
  otp: string;
  otpSent: boolean;
  phoneError: boolean;
  otpError: boolean;
  addrError: string;
  payError: string;
  next: GatedScreen;
  addr: Address;
  pay: PayMethod;
  upi: string;
  card: CardDetails;
  orderId: string | null;
  order: Order | null;
}
