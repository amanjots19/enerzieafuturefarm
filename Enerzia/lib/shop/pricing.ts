import type {
  Address,
  AddressDTO,
  CartLine,
  FreeShippingDTO,
  ProductDTO,
  RequestKey,
  ShopState,
  UserDTO,
} from './types';

export const rupee = (n: number): string => '₹' + n.toLocaleString('en-IN');

/**
 * Formats a paise integer as a rupee string. When the amount is a whole
 * rupee the result is `rupee(p/100)` (uses toLocaleString); otherwise it is
 * `₹X.XX` with exactly two decimal places. Never use Math.round — it would
 * render a wrong number confidently.
 */
export const rupeeFromPaise = (paise: number): string => {
  // Both branches go through toLocaleString so grouping is applied either way.
  // toFixed(2) does not group, so ₹1,548 sat next to ₹1247.90 in the same
  // summary — the same number formatted two different ways.
  const hasPaise = paise % 100 !== 0;
  return (
    '₹' +
    (paise / 100).toLocaleString('en-IN', {
      minimumFractionDigits: hasPaise ? 2 : 0,
      maximumFractionDigits: hasPaise ? 2 : 0,
    })
  );
};

/** Looks up a product by id in the loaded list. Returns undefined if not found. */
export const findProduct = (
  id: string | null,
  products: readonly ProductDTO[],
): ProductDTO | undefined => products.find((p) => p.id === id);

export const lineShort = (l: CartLine): string =>
  `${l.name} · ${l.variant} × ${l.qty}`;

/**
 * One-line summary of a saved AddressDTO for display in the address radio list.
 * Does not include the shopper's phone — that is shown separately on the panel.
 */
export function addressDTOSummary(addr: AddressDTO): string {
  return [
    addr.line1,
    [addr.city, addr.state, addr.pin].filter(Boolean).join(', '),
  ]
    .filter((x) => x && x.trim())
    .join(', ');
}

export function addressSummary(addr: Address, user: UserDTO | null): string {
  return [
    addr.name,
    addr.line1,
    [addr.city, addr.state, addr.pin].filter(Boolean).join(', '),
    '+91 ' + (user?.phone ?? ''),
    addr.email,
  ]
    .filter((x) => x && x.trim() && x !== '+91 ')
    .join(' · ');
}

/** Returns the first failing field + message, or null when the address is complete. Messages match the design. */
export function validateAddress(
  a: Address,
): { field: keyof Address; message: string } | null {
  if (!a.name.trim())
    return { field: 'name', message: 'Please enter the name for delivery.' };
  if (!/^\S+@\S+\.\S+$/.test(a.email))
    return { field: 'email', message: 'Please enter a valid email for order updates.' };
  // Order and wording mirror the API exactly (product.md §3.4) — a client that
  // validated in a different order would show a different message than the
  // server would for the same input.
  if (!/^\d{10}$/.test(a.phone))
    return {
      field: 'phone',
      message: 'Please enter a valid 10-digit mobile number for delivery.',
    };
  if (a.line1.trim().length < 6)
    return { field: 'line1', message: 'Please enter your full street address.' };
  if (!a.city.trim() || !a.state.trim())
    return { field: 'city', message: 'Please enter your city and state.' };
  if (!/^\d{6}$/.test(a.pin))
    return { field: 'pin', message: 'PIN code must be 6 digits.' };
  return null;
}

/** Free-shipping hint text from server-provided freeShipping data. Returns '' when none needed. */
export const freeShipHint = (fs: FreeShippingDTO | null | undefined): string => {
  if (!fs || fs.qualified || fs.remainingAmount <= 0) return '';
  return `Add ${rupeeFromPaise(fs.remainingAmount)} more for free delivery.`;
};

const allPendingFalse: Record<RequestKey, boolean> = {
  products: false,
  product: false,
  trust: false,
  otpRequest: false,
  otpVerify: false,
  session: false,
  cart: false,
  addresses: false,
  addressSave: false,
  addressDelete: false,
  checkout: false,
  payment: false,
  widgetSession: false,
  orders: false,
};

export const initialState: ShopState = {
  screen: 'shop',
  filter: 'all',
  cart: null,
  cartBuffer: [],
  pdp: null,
  products: [],
  pdpDetail: null,
  qty: 1,
  user: null,
  phone: '',
  otp: '',
  otpSent: false,
  phoneError: false,
  otpError: false,
  resendAfterSeconds: 0,
  otpResendNonce: 0,
  otpDevCode: undefined,
  addresses: [],
  selectedAddressId: null,
  showAddressForm: false,
  editingAddressId: null,
  addrFieldErrors: {},
  addrError: '',
  next: 'cart',
  addr: { name: '', email: '', phone: '', line1: '', city: '', state: '', pin: '' },
  checkout: null,
  placedOrder: null,
  paymentCancelled: null,
  booting: true,
  pending: allPendingFalse,
  banner: null,
  trust: [],
  orders: [],
};
