import {
  cartKey,
  findProduct,
  totals,
  validateAddress,
  validatePayment,
  variantOf,
} from './pricing';
import type {
  Address,
  CardDetails,
  Filter,
  GatedScreen,
  PayMethod,
  Product,
  ProductId,
  Screen,
  ShopState,
  Variant,
} from './types';

export type ShopAction =
  | { type: 'go'; screen: Screen }
  /** Navigate to a screen that requires a signed-in shopper. */
  | { type: 'requireAuth'; dest: GatedScreen }
  | { type: 'openPdp'; id: ProductId }
  | { type: 'setFilter'; filter: Filter }
  | { type: 'pickVariant'; id: ProductId; index: number }
  | { type: 'addToCart'; product: Product; variant: Variant; qty: number }
  /** Add the PDP's selected variant at the current quantity, then gate to cart. */
  | { type: 'addFromPdp' }
  | { type: 'bumpQty'; delta: number }
  | { type: 'bumpLine'; key: string; delta: number }
  | { type: 'removeLine'; key: string }
  | { type: 'setPhone'; value: string }
  | { type: 'setOtp'; value: string }
  | { type: 'sendOtp' }
  | { type: 'verifyOtp' }
  | { type: 'editPhone' }
  | { type: 'setAddr'; field: keyof Address; value: string }
  | { type: 'goCheckout' }
  | { type: 'setPay'; pay: PayMethod }
  | { type: 'setUpi'; value: string }
  | { type: 'setCard'; field: keyof CardDetails; value: string }
  /** orderId is generated in the event handler so the reducer stays pure. */
  | { type: 'placeOrder'; orderId: string };

const digits = (v: string, max: number) => v.replace(/\D/g, '').slice(0, max);

/** Field-level sanitisers, matching the design's onChange handlers. */
const cardSanitisers: Record<keyof CardDetails, (v: string) => string> = {
  num: (v) => v.replace(/[^\d ]/g, '').slice(0, 19),
  exp: (v) => v.slice(0, 5),
  cvv: (v) => digits(v, 3),
};

export function shopReducer(state: ShopState, action: ShopAction): ShopState {
  switch (action.type) {
    case 'go':
      return { ...state, screen: action.screen };

    case 'requireAuth':
      return state.user
        ? { ...state, screen: action.dest }
        : { ...state, screen: 'login', next: action.dest };

    case 'openPdp':
      return { ...state, screen: 'pdp', pdp: action.id, qty: 1 };

    case 'setFilter':
      return { ...state, filter: action.filter };

    case 'pickVariant':
      return { ...state, sel: { ...state.sel, [action.id]: action.index } };

    case 'addToCart':
      return { ...state, cart: mergeLine(state.cart, action.product, action.variant, action.qty) };

    case 'addFromPdp': {
      const product = findProduct(state.pdp);
      const variant = variantOf(product, state.sel);
      const cart = mergeLine(state.cart, product, variant, state.qty);
      return state.user
        ? { ...state, cart, screen: 'cart' }
        : { ...state, cart, screen: 'login', next: 'cart' };
    }

    case 'bumpQty':
      return { ...state, qty: Math.max(1, state.qty + action.delta) };

    case 'bumpLine':
      return {
        ...state,
        cart: state.cart
          .map((l) => (l.key === action.key ? { ...l, qty: l.qty + action.delta } : l))
          .filter((l) => l.qty > 0),
      };

    case 'removeLine':
      return { ...state, cart: state.cart.filter((l) => l.key !== action.key) };

    case 'setPhone':
      return { ...state, phone: digits(action.value, 10), phoneError: false };

    case 'setOtp':
      return { ...state, otp: digits(action.value, 6), otpError: false };

    case 'sendOtp':
      return state.phone.length === 10
        ? { ...state, otpSent: true }
        : { ...state, phoneError: true };

    case 'verifyOtp':
      return state.otp.length === 6
        ? { ...state, user: state.phone, screen: state.next, otpSent: false, otp: '' }
        : { ...state, otpError: true };

    case 'editPhone':
      return { ...state, otpSent: false, otp: '', otpError: false };

    case 'setAddr': {
      const value = action.field === 'pin' ? digits(action.value, 6) : action.value;
      return { ...state, addr: { ...state.addr, [action.field]: value } };
    }

    case 'goCheckout': {
      const addrError = validateAddress(state.addr);
      return addrError ? { ...state, addrError } : { ...state, addrError: '', screen: 'checkout' };
    }

    case 'setPay':
      return { ...state, pay: action.pay, payError: '' };

    case 'setUpi':
      return { ...state, upi: action.value, payError: '' };

    case 'setCard':
      return {
        ...state,
        card: { ...state.card, [action.field]: cardSanitisers[action.field](action.value) },
        payError: '',
      };

    case 'placeOrder': {
      const payError = validatePayment(state.pay, state.upi, state.card);
      if (payError) return { ...state, payError };
      return {
        ...state,
        payError: '',
        screen: 'done',
        orderId: action.orderId,
        order: { lines: state.cart, total: totals(state.cart).total, pay: state.pay },
        cart: [],
      };
    }
  }
}

function mergeLine(
  cart: ShopState['cart'],
  p: Product,
  v: Variant,
  qty: number
): ShopState['cart'] {
  const key = cartKey(p.id, v.label);
  const i = cart.findIndex((l) => l.key === key);
  if (i >= 0) {
    const next = cart.slice();
    next[i] = { ...next[i]!, qty: next[i]!.qty + qty };
    return next;
  }
  return [
    ...cart,
    {
      key,
      id: p.id,
      name: p.name,
      grad: p.grad,
      variant: v.label,
      sub: v.sub,
      price: v.price,
      mrp: v.mrp,
      qty,
    },
  ];
}

export const newOrderId = (): string =>
  'EFF-' + Math.floor(100000 + Math.random() * 899999);
