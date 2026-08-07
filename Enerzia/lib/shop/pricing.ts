import { FREE_SHIP_OVER, PRODUCTS, SHIP_FEE } from './data';
import type {
  Address,
  CardDetails,
  CartLine,
  PayMethod,
  Product,
  ProductId,
  ShopState,
  Totals,
  Variant,
} from './types';

export const rupee = (n: number): string => '₹' + n.toLocaleString('en-IN');

export const offLabel = (v: Variant): string =>
  Math.round((1 - v.price / v.mrp) * 100) + '% off';

export const cartKey = (id: ProductId, variantLabel: string): string =>
  `${id}|${variantLabel}`;

export const findProduct = (id: ProductId | null): Product =>
  PRODUCTS.find((p) => p.id === id) ?? PRODUCTS[0]!;

/** The selected variant, clamped the way the design's variantOf() does. */
export const variantOf = (p: Product, sel: Record<ProductId, number>): Variant =>
  p.variants[Math.min(sel[p.id] ?? 0, p.variants.length - 1)]!;

export function totals(cart: readonly CartLine[]): Totals {
  const mrp = cart.reduce((a, l) => a + l.mrp * l.qty, 0);
  const sub = cart.reduce((a, l) => a + l.price * l.qty, 0);
  const ship = sub === 0 || sub >= FREE_SHIP_OVER ? 0 : SHIP_FEE;
  return { mrp, sub, ship, total: sub + ship, save: mrp - sub };
}

export const cartCount = (cart: readonly CartLine[]): number =>
  cart.reduce((n, l) => n + l.qty, 0);

export const lineVariantLabel = (l: CartLine): string =>
  l.sub ? `${l.variant} · ${l.sub}` : l.variant;

export const lineShort = (l: CartLine): string =>
  `${l.name} · ${l.variant} × ${l.qty}`;

export function addressSummary(addr: Address, user: string | null): string {
  return [
    addr.name,
    addr.line1,
    [addr.city, addr.state, addr.pin].filter(Boolean).join(', '),
    '+91 ' + (user ?? ''),
    addr.email,
  ]
    .filter((x) => x && x.trim() && x !== '+91 ')
    .join(' · ');
}

/** Returns '' when the address is complete. Messages match the design. */
export function validateAddress(a: Address): string {
  if (!a.name.trim()) return 'Please enter the name for delivery.';
  if (!/^\S+@\S+\.\S+$/.test(a.email)) return 'Please enter a valid email for order updates.';
  if (a.line1.trim().length < 6) return 'Please enter your full street address.';
  if (!a.city.trim() || !a.state.trim()) return 'Please enter your city and state.';
  if (!/^\d{6}$/.test(a.pin)) return 'PIN code must be 6 digits.';
  return '';
}

/** Returns '' when payment details are usable. Messages match the design. */
export function validatePayment(pay: PayMethod, upi: string, card: CardDetails): string {
  if (pay === 'upi' && !/^[\w.\-]{2,}@[a-zA-Z]{2,}$/.test(upi))
    return 'Enter a valid UPI ID, e.g. name@bank.';
  if (pay === 'card') {
    if (card.num.replace(/\s/g, '').length !== 16) return 'Card number must be 16 digits.';
    if (!/^\d{2}\/\d{2}$/.test(card.exp)) return 'Expiry must be MM/YY.';
    if (!/^\d{3}$/.test(card.cvv)) return 'CVV must be 3 digits.';
  }
  return '';
}

export const freeShipHint = (t: Totals): string =>
  t.sub > 0 && t.sub < FREE_SHIP_OVER
    ? `Add ${rupee(FREE_SHIP_OVER - t.sub)} more for free delivery.`
    : '';

export const initialState: ShopState = {
  screen: 'shop',
  filter: 'all',
  sel: { powder: 0, tablets: 1, refill: 0, bundle: 0 },
  cart: [],
  pdp: null,
  qty: 1,
  user: null,
  phone: '',
  otp: '',
  otpSent: false,
  phoneError: false,
  otpError: false,
  addrError: '',
  payError: '',
  next: 'cart',
  addr: { name: '', email: '', line1: '', city: '', state: '', pin: '' },
  pay: 'upi',
  upi: '',
  card: { num: '', exp: '', cvv: '' },
  orderId: null,
  order: null,
};
