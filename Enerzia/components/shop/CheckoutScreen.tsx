import { PAY_OPTIONS } from '@/lib/shop/data';
import { addressSummary, lineShort, rupee, totals } from '@/lib/shop/pricing';
import { newOrderId, type ShopAction } from '@/lib/shop/reducer';
import type { CardDetails, ShopState } from '@/lib/shop/types';

import { ErrorText, Field } from './controls';

export function CheckoutScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  const t = totals(state.cart);

  const cardField = (field: keyof CardDetails, label: string, placeholder: string) => (
    <Field
      id={`f-card-${field}`}
      label={label}
      value={state.card[field]}
      placeholder={placeholder}
      inputMode="numeric"
      onChange={(value) => dispatch({ type: 'setCard', field, value })}
    />
  );

  return (
    <div className="wrap-md screen">
      <button
        className="btn btn-ghost"
        type="button"
        style={{ marginBottom: 12 }}
        onClick={() => dispatch({ type: 'requireAuth', dest: 'cart' })}
      >
        ← Back to cart
      </button>
      <h1 className="page-title">Checkout</h1>

      <div className="two-col">
        <div className="col-stack">
          <div className="panel">
            <div className="panel-head">
              <h3>Contact &amp; delivery</h3>
              <button
                className="btn btn-ghost"
                type="button"
                style={{ fontSize: 13 }}
                onClick={() => dispatch({ type: 'requireAuth', dest: 'cart' })}
              >
                Edit
              </button>
            </div>
            <div className="addr-summary">{addressSummary(state.addr, state.user)}</div>
          </div>

          <div className="panel">
            <h3>Payment</h3>
            <div className="pay-list" role="radiogroup" aria-label="Payment method">
              {PAY_OPTIONS.map((o) => (
                <button
                  className="pay-opt"
                  type="button"
                  role="radio"
                  aria-checked={state.pay === o.k}
                  key={o.k}
                  onClick={() => dispatch({ type: 'setPay', pay: o.k })}
                >
                  <span className="pay-dot" aria-hidden="true" />
                  <span style={{ flex: 1 }}>
                    <span className="pay-label">{o.label}</span>
                    <br />
                    <span className="pay-sub">{o.sub}</span>
                  </span>
                </button>
              ))}
            </div>

            {state.pay === 'upi' && (
              <div style={{ marginTop: 16 }}>
                <Field
                  id="f-upi"
                  label="UPI ID"
                  value={state.upi}
                  placeholder="name@bank"
                  onChange={(value) => dispatch({ type: 'setUpi', value })}
                />
              </div>
            )}

            {state.pay === 'card' && (
              <div className="field-grid-3" style={{ marginTop: 16 }}>
                <div style={{ gridColumn: '1/-1' }}>
                  {cardField('num', 'Card number', '4111 1111 1111 1111')}
                </div>
                {cardField('exp', 'Expiry', 'MM/YY')}
                {cardField('cvv', 'CVV', '123')}
              </div>
            )}

            {state.payError && (
              <div style={{ marginTop: 14 }}>
                <ErrorText>{state.payError}</ErrorText>
              </div>
            )}
          </div>
        </div>

        <div className="panel aside">
          <h3 style={{ fontSize: 20, marginBottom: 14 }}>
            {state.cart.length} {state.cart.length === 1 ? 'item' : 'items'}
          </h3>
          <div className="mini-lines">
            {state.cart.map((l) => (
              <div className="mini-line" key={l.key}>
                <span>{lineShort(l)}</span>
                <span>{rupee(l.price * l.qty)}</span>
              </div>
            ))}
          </div>
          <div className="rule" style={{ marginBottom: 12 }} />
          <div className="summary">
            <div className="summary-row">
              <span className="muted">You save</span>
              <span className="save">{rupee(t.save)}</span>
            </div>
            <div className="summary-row">
              <span className="muted">Delivery</span>
              <span style={{ fontWeight: 600 }}>{t.ship === 0 ? 'Free' : rupee(t.ship)}</span>
            </div>
            <div className="summary-row total">
              <span>To pay</span>
              <span className="summary-total">{rupee(t.total)}</span>
            </div>
          </div>
          <button
            className="btn btn-primary btn-block btn-cta"
            type="button"
            onClick={() => dispatch({ type: 'placeOrder', orderId: newOrderId() })}
          >
            {state.pay === 'cod' ? 'Place order' : `Pay ${rupee(t.total)}`}
          </button>
          <div className="secure-note">Secure payment · 7-day replacement</div>
        </div>
      </div>
    </div>
  );
}
