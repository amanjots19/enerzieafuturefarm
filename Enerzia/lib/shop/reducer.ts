import {
  validateAddress,
} from './pricing';
import type {
  Address,
  AddressDTO,
  CartDTO,
  CheckoutData,
  Filter,
  GatedScreen,
  OrderDTO,
  ProductDetailData,
  ProductDTO,
  RequestKey,
  Screen,
  ShopState,
  TrustTileDTO,
  UserDTO,
} from './types';

const emptyAddr: Address = {
  name: '',
  email: '',
  phone: '',
  line1: '',
  city: '',
  state: '',
  pin: '',
};

export type ShopAction =
  | { type: 'go'; screen: Screen }
  /** Navigate to a screen that requires a signed-in shopper. */
  | { type: 'requireAuth'; dest: GatedScreen }
  | { type: 'openPdp'; id: string }
  | { type: 'setFilter'; filter: Filter }
  | { type: 'bumpQty'; delta: number }
  | { type: 'setPhone'; value: string }
  | { type: 'setOtp'; value: string }
  | { type: 'editPhone' }
  | { type: 'setAddr'; field: keyof Address; value: string }
  | { type: 'goCheckout' }
  /* ── async seam ── */
  /** Marks a request as in-flight; sets pending[key] = true. */
  | { type: 'reqStart'; key: RequestKey }
  /** Marks a request as settled and records an error banner. */
  | { type: 'reqFail'; key: RequestKey; message: string }
  /** Stores loaded trust tiles and clears pending.trust. */
  | { type: 'trustLoaded'; tiles: TrustTileDTO[] }
  /** Stores the product list and clears pending.products. */
  | { type: 'productsLoaded'; products: readonly ProductDTO[] }
  /** Stores the PDP detail payload and clears pending.product. */
  | { type: 'productLoaded'; detail: ProductDetailData }
  /** Replaces the whole cart with the server's response and clears pending.cart. */
  | { type: 'cartLoaded'; cart: CartDTO }
  /** Stores the POST /orders response; also clears any prior paymentCancelled message. */
  | { type: 'checkoutOpened'; data: CheckoutData }
  /**
   * Payment was confirmed (callback or polling). Stores the placed order,
   * clears pending.payment, and navigates to 'done'.
   */
  | { type: 'paymentConfirmed'; order: OrderDTO }
  /**
   * Shopper dismissed the Razorpay modal without completing payment.
   * Clears pending.payment and checkout; shows an inline message on the review screen.
   */
  | { type: 'paymentCancelled'; message: string }
  /** Signals that the boot sequence has settled. */
  | { type: 'bootDone' }
  /** Re-arms the booting state before a retry; clears the stale error banner. */
  | { type: 'bootStart' }
  /** Clears the signed-in user; fired by the global 401 hook. */
  | { type: 'signOut' }
  /** Replaces the orders list after GET /orders. */
  | { type: 'ordersLoaded'; orders: readonly OrderDTO[] }
  /** Dismisses the error banner. */
  | { type: 'clearBanner' }
  /* ── auth async results ── */
  | { type: 'otpRequested'; resendAfterSeconds: number; devCode?: string }
  | { type: 'otpRequestFailed'; phoneError: string }
  | { type: 'otpVerified'; user: UserDTO }
  | { type: 'otpVerifyFailed'; otpError: string }
  | { type: 'sessionRestored'; user: UserDTO }
  /* ── offline cart buffer ── */
  /** Replaces the whole buffer (used when loading from localStorage or after a batch update). */
  | { type: 'setCartBuffer'; buffer: { productId: string; qty: number }[] }
  /** Clears the buffer after successful replay on sign-in. */
  | { type: 'clearCartBuffer' }
  /* ── addresses ── */
  /** Replaces the full address list; auto-selects the default when no valid selection exists. */
  | { type: 'addressesLoaded'; addresses: AddressDTO[] }
  /** Signals a successful POST or PUT; closes the form. The list is reloaded separately. */
  | { type: 'addressSaved'; address: AddressDTO }
  /** Replaces the address list after a DELETE (server returns the remaining list). */
  | { type: 'addressDeleted'; addresses: AddressDTO[] }
  /** Sets the selected address for checkout. */
  | { type: 'selectAddress'; id: string }
  /** Opens the add/edit form. editing=null → new address; editing=id → edit that address. */
  | { type: 'showAddressForm'; editing: string | null }
  /** Closes the add/edit form and resets it. */
  | { type: 'hideAddressForm' }
  /** Records a per-field server validation error on the address form. */
  | { type: 'setAddrFieldError'; field: keyof Address; message: string };

const digits = (v: string, max: number) => v.replace(/\D/g, '').slice(0, max);

/** Auto-selects the default address, or the first, unless the current selection is still valid. */
function resolveSelection(addresses: AddressDTO[], currentId: string | null): string | null {
  if (currentId && addresses.some((a) => a.id === currentId)) return currentId;
  return addresses.find((a) => a.isDefault)?.id ?? addresses[0]?.id ?? null;
}

export function shopReducer(state: ShopState, action: ShopAction): ShopState {
  switch (action.type) {
    case 'go':
      return { ...state, screen: action.screen };

    case 'requireAuth':
      return state.user
        ? { ...state, screen: action.dest }
        : { ...state, screen: 'login', next: action.dest };

    case 'openPdp':
      return { ...state, screen: 'pdp', pdp: action.id, qty: 1, pdpDetail: null };

    case 'setFilter':
      return { ...state, filter: action.filter };

    case 'bumpQty':
      return { ...state, qty: Math.max(1, state.qty + action.delta) };

    case 'setPhone':
      return { ...state, phone: digits(action.value, 10), phoneError: false };

    case 'setOtp':
      return { ...state, otp: digits(action.value, 6), otpError: false };

    case 'editPhone':
      return {
        ...state,
        otpSent: false,
        otp: '',
        otpError: false,
        otpDevCode: undefined,
        resendAfterSeconds: 0,
      };

    case 'setAddr': {
      // Numeric fields are stripped and capped as typed.
      //
      // The phone cap is 15, E.164's ceiling, not 10 — the delivery contact may
      // be a foreign number since 2026-08-24 (product.md §3.4). At 10 a pasted
      // "+919876543210" silently lost its last two digits, which looked like a
      // typo the shopper had made.
      let value = action.value;
      if (action.field === 'pin') value = digits(action.value, 6);
      if (action.field === 'phone') value = digits(action.value, 15);
      const addrFieldErrors = { ...state.addrFieldErrors };
      delete addrFieldErrors[action.field];
      return { ...state, addr: { ...state.addr, [action.field]: value }, addrFieldErrors };
    }

    case 'goCheckout': {
      if (!state.selectedAddressId) {
        return { ...state, addrError: 'Please select a delivery address to continue.' };
      }
      return { ...state, addrError: '', screen: 'review' };
    }

    case 'reqStart': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending[action.key] = true;
      return { ...state, pending };
    }

    case 'reqFail': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending[action.key] = false;
      return { ...state, pending, banner: action.message };
    }

    case 'trustLoaded': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['trust'] = false;
      return { ...state, pending, trust: action.tiles };
    }

    case 'productsLoaded': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['products'] = false;
      return { ...state, pending, products: action.products };
    }

    case 'productLoaded': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['product'] = false;
      return { ...state, pending, pdpDetail: action.detail };
    }

    case 'cartLoaded': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['cart'] = false;
      return { ...state, pending, cart: action.cart };
    }

    case 'checkoutOpened': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['checkout'] = false;
      // Clear any prior cancellation message so it doesn't linger if the shopper retries.
      return { ...state, pending, checkout: action.data, paymentCancelled: null };
    }

    case 'paymentConfirmed': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['payment'] = false;
      return {
        ...state,
        pending,
        placedOrder: action.order,
        checkout: null,
        paymentCancelled: null,
        screen: 'done',
      };
    }

    case 'paymentCancelled': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['payment'] = false;
      // Discard checkout so pressing Pay next time issues a fresh POST /orders.
      return { ...state, pending, checkout: null, paymentCancelled: action.message };
    }

    case 'bootDone':
      return { ...state, booting: false };

    case 'bootStart':
      return { ...state, booting: true, banner: null };

    case 'signOut':
      return {
        ...state,
        user: null,
        screen: 'shop',
        cart: null,
        cartBuffer: [],
        addresses: [],
        orders: [],
        selectedAddressId: null,
        checkout: null,
        placedOrder: null,
        paymentCancelled: null,
      };

    case 'ordersLoaded': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['orders'] = false;
      return { ...state, pending, orders: action.orders };
    }

    case 'clearBanner':
      return { ...state, banner: null };

    case 'otpRequested': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['otpRequest'] = false;
      return {
        ...state,
        pending,
        otpSent: true,
        phoneError: false,
        resendAfterSeconds: action.resendAfterSeconds,
        otpResendNonce: state.otpResendNonce + 1,
        otpDevCode: action.devCode,
      };
    }

    case 'otpRequestFailed': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['otpRequest'] = false;
      return { ...state, pending, phoneError: action.phoneError };
    }

    case 'otpVerified': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['otpVerify'] = false;
      return {
        ...state,
        pending,
        user: action.user,
        screen: state.next,
        otpSent: false,
        otp: '',
        otpError: false,
        otpDevCode: undefined,
      };
    }

    case 'otpVerifyFailed': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['otpVerify'] = false;
      return { ...state, pending, otpError: action.otpError };
    }

    case 'sessionRestored':
      return { ...state, user: action.user };

    case 'setCartBuffer':
      return { ...state, cartBuffer: action.buffer };

    case 'clearCartBuffer':
      return { ...state, cartBuffer: [] };

    case 'addressesLoaded': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['addresses'] = false;
      const selectedAddressId = resolveSelection(action.addresses, state.selectedAddressId);
      return { ...state, pending, addresses: action.addresses, selectedAddressId };
    }

    case 'addressSaved': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['addressSave'] = false;
      return {
        ...state,
        pending,
        selectedAddressId: action.address.id,
        showAddressForm: false,
        editingAddressId: null,
        addr: emptyAddr,
        addrError: '',
        addrFieldErrors: {},
      };
    }

    case 'addressDeleted': {
      const pending: Record<RequestKey, boolean> = { ...state.pending };
      pending['addressDelete'] = false;
      const selectedAddressId = resolveSelection(action.addresses, state.selectedAddressId);
      return { ...state, pending, addresses: action.addresses, selectedAddressId };
    }

    case 'selectAddress':
      return { ...state, selectedAddressId: action.id, addrError: '' };

    case 'showAddressForm': {
      if (action.editing) {
        const existing = state.addresses.find((a) => a.id === action.editing);
        if (!existing) return state;
        return {
          ...state,
          showAddressForm: true,
          editingAddressId: action.editing,
          addr: {
            name: existing.name,
            email: existing.email,
            // Absent on addresses saved before the field existed. Editing one
            // is exactly when the shopper supplies it.
            phone: existing.phone ?? '',
            line1: existing.line1,
            city: existing.city,
            state: existing.state,
            pin: existing.pin,
          },
          addrError: '',
          addrFieldErrors: {},
        };
      }
      return {
        ...state,
        showAddressForm: true,
        editingAddressId: null,
        addr: emptyAddr,
        addrError: '',
        addrFieldErrors: {},
      };
    }

    case 'hideAddressForm':
      return {
        ...state,
        showAddressForm: false,
        editingAddressId: null,
        addr: emptyAddr,
        addrError: '',
        addrFieldErrors: {},
      };

    case 'setAddrFieldError':
      return {
        ...state,
        addrFieldErrors: { ...state.addrFieldErrors, [action.field]: action.message },
      };
  }
}
