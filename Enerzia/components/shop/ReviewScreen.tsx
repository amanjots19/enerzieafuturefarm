import type { ShopActions } from '@/app/shop/useShop';
import { formatAccountPhone } from '@/lib/shop/phone';
import { addressDTOSummary, rupeeFromPaise } from '@/lib/shop/pricing';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

export function ReviewScreen({
  state,
  dispatch,
  actions,
  razorpayReady,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
  razorpayReady: boolean;
}) {
  const ct = state.cart?.totals;
  const lines = state.cart?.lines ?? [];
  const itemCount = state.cart?.itemCount ?? 0;
  const checkoutPending = state.pending.checkout;
  const paymentPending = state.pending.payment;

  const selectedAddr = state.addresses.find((a) => a.id === state.selectedAddressId);

  const payDisabled = !razorpayReady || checkoutPending || paymentPending || !selectedAddr;

  const payLabel = !razorpayReady
    ? 'Loading payment…'
    : checkoutPending
    ? 'Opening checkout…'
    : paymentPending
    ? 'Completing payment…'
    : `Pay ${rupeeFromPaise(ct?.total ?? 0)}`;

  return (
    <div className="wrap-md screen">
      <button
        className="btn btn-ghost"
        type="button"
        style={{ marginBottom: 12 }}
        onClick={() => dispatch({ type: 'go', screen: 'cart' })}
      >
        ← Back to cart
      </button>
      <h1 className="page-title">Review order</h1>

      <div className="two-col">
        <div className="col-stack">
          <div className="panel">
            <div className="panel-head">
              <h3>Delivery address</h3>
              <button
                className="btn btn-ghost"
                type="button"
                style={{ fontSize: 13 }}
                onClick={() => dispatch({ type: 'go', screen: 'cart' })}
              >
                Edit
              </button>
            </div>
            {selectedAddr ? (
              <div className="addr-summary">
                <strong>{selectedAddr.name}</strong>
                <br />
                {addressDTOSummary(selectedAddr)}
                <br />
                {formatAccountPhone(state.user?.phone ?? '')}
              </div>
            ) : (
              <div className="addr-summary" style={{ color: 'var(--color-muted)' }}>
                No address selected
              </div>
            )}
          </div>
        </div>

        <div className="panel aside">
          <h3 style={{ fontSize: 20, marginBottom: 14 }}>
            {itemCount} {itemCount === 1 ? 'item' : 'items'}
          </h3>
          <div className="mini-lines">
            {lines.map((l) => (
              <div className="mini-line" key={l.productId}>
                <span>{l.name} · {l.stat2} × {l.qty}</span>
                <span>{rupeeFromPaise(l.lineTotal)}</span>
              </div>
            ))}
          </div>
          <div className="rule" style={{ marginBottom: 12 }} />
          <div className="summary">
            <div className="summary-row">
              <span className="muted">You save</span>
              <span className="save">{rupeeFromPaise(ct?.savings ?? 0)}</span>
            </div>
            <div className="summary-row">
              <span className="muted">Delivery</span>
              <span style={{ fontWeight: 600 }}>
                {(ct?.shipping ?? 0) === 0 ? 'Free' : rupeeFromPaise(ct?.shipping ?? 0)}
              </span>
            </div>
            <div className="summary-row total">
              <span>To pay</span>
              <span className="summary-total">{rupeeFromPaise(ct?.total ?? 0)}</span>
            </div>
          </div>

          <button
            className="btn btn-primary btn-block btn-cta"
            type="button"
            disabled={payDisabled}
            onClick={() => void actions.openCheckout()}
          >
            {payLabel}
          </button>

          {/* Calm inline message after a dismissed or failed-then-dismissed payment.
              Not a banner — nothing went wrong, the shopper just cancelled. */}
          {state.paymentCancelled && (
            <p style={{ fontSize: 13, color: 'var(--color-neutral-600)', marginTop: 10, textAlign: 'center' }}>
              {state.paymentCancelled}
            </p>
          )}

          <div className="secure-note">Secure payment · 7-day replacement</div>
        </div>
      </div>
    </div>
  );
}
