import { freeShipHint, lineVariantLabel, rupee, totals } from '@/lib/shop/pricing';
import type { ShopAction } from '@/lib/shop/reducer';
import type { Address, ShopState } from '@/lib/shop/types';

import { ErrorText, Field } from './controls';

export function CartScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  if (state.cart.length === 0) {
    return (
      <div className="wrap-md screen">
        <h1 className="page-title">Your cart</h1>
        <div className="empty">
          <p>Nothing here yet.</p>
          <button
            className="btn btn-primary"
            type="button"
            style={{ padding: '12px 24px' }}
            onClick={() => dispatch({ type: 'go', screen: 'shop' })}
          >
            Browse products
          </button>
        </div>
      </div>
    );
  }

  const t = totals(state.cart);
  const hint = freeShipHint(t);

  const addrField = (field: keyof Address, label: string, placeholder: string) => (
    <Field
      id={`f-addr-${field}`}
      label={label}
      value={state.addr[field]}
      placeholder={placeholder}
      inputMode={field === 'pin' ? 'numeric' : field === 'email' ? 'email' : 'text'}
      onChange={(value) => dispatch({ type: 'setAddr', field, value })}
    />
  );

  return (
    <div className="wrap-md screen">
      <h1 className="page-title">Your cart</h1>
      <div className="two-col">
        <div className="col-stack">
          <div className="panel-lines">
            {state.cart.map((l) => (
              <div className="line" key={l.key}>
                <div className="line-thumb" style={{ background: l.grad }} />
                <div className="line-main">
                  <div className="line-name">{l.name}</div>
                  <div className="line-variant">{lineVariantLabel(l)}</div>
                  <div className="line-price">
                    <b>{rupee(l.price)}</b>
                    <span className="mrp">{rupee(l.mrp)}</span>
                  </div>
                </div>
                <div className="line-qty">
                  <button
                    className="btn btn-icon"
                    type="button"
                    onClick={() => dispatch({ type: 'bumpLine', key: l.key, delta: -1 })}
                    aria-label={`Decrease quantity of ${l.name}`}
                  >
                    −
                  </button>
                  <span aria-live="polite">{l.qty}</span>
                  <button
                    className="btn btn-icon"
                    type="button"
                    onClick={() => dispatch({ type: 'bumpLine', key: l.key, delta: 1 })}
                    aria-label={`Increase quantity of ${l.name}`}
                  >
                    +
                  </button>
                </div>
                <div className="line-total">{rupee(l.price * l.qty)}</div>
                <button
                  className="btn btn-icon line-remove"
                  type="button"
                  onClick={() => dispatch({ type: 'removeLine', key: l.key })}
                  aria-label={`Remove ${l.name}`}
                >
                  ✕
                </button>
              </div>
            ))}
          </div>

          <div className="panel">
            <h3>Delivery address</h3>
            <p className="panel-sub">Signed in as +91 {state.user ?? ''}</p>
            <div className="field-grid-2">
              {addrField('name', 'Full name', 'Ananya Sharma')}
              {addrField('email', 'Email for order updates', 'you@email.com')}
            </div>
            <div style={{ marginTop: 14 }}>
              {addrField('line1', 'Flat, house no., building, street', '12, Anand Residency, MG Road')}
            </div>
            <div className="field-grid-3" style={{ marginTop: 14 }}>
              {addrField('city', 'City', 'Pune')}
              {addrField('state', 'State', 'Maharashtra')}
              {addrField('pin', 'PIN code', '411001')}
            </div>
            {state.addrError && (
              <div style={{ marginTop: 12 }}>
                <ErrorText>{state.addrError}</ErrorText>
              </div>
            )}
          </div>
        </div>

        <div className="panel aside">
          <h3>Order summary</h3>
          <div className="summary">
            <div className="summary-row">
              <span className="muted">MRP total</span>
              <span className="strike">{rupee(t.mrp)}</span>
            </div>
            <div className="summary-row">
              <span className="muted">Discount</span>
              <span className="save">− {rupee(t.save)}</span>
            </div>
            <div className="summary-row">
              <span className="muted">Delivery</span>
              <span style={{ fontWeight: 600 }}>{t.ship === 0 ? 'Free' : rupee(t.ship)}</span>
            </div>
            <div className="rule" style={{ margin: '4px 0' }} />
            <div className="summary-row total">
              <span>To pay</span>
              <span className="summary-total">{rupee(t.total)}</span>
            </div>
          </div>

          {hint && <div className="ship-hint">{hint}</div>}

          <button
            className="btn btn-primary btn-block btn-cta"
            type="button"
            onClick={() => dispatch({ type: 'goCheckout' })}
          >
            Proceed to checkout
          </button>
          <button
            className="btn btn-ghost btn-block"
            type="button"
            onClick={() => dispatch({ type: 'go', screen: 'shop' })}
          >
            Add more products
          </button>
        </div>
      </div>
    </div>
  );
}
