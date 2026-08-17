import { freeShipHint, rupeeFromPaise } from '@/lib/shop/pricing';
import type { ShopActions } from '@/app/shop/useShop';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

import { ErrorText } from './controls';
import { AddressCard, AddressForm } from './address';

export function CartScreen({
  state,
  dispatch,
  actions,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
}) {
  const cart = state.cart;
  const cartPending = state.pending.cart;
  const addrDeleting = state.pending.addressDelete;
  const hasAddresses = state.addresses.length > 0;

  if (!cart || cart.lines.length === 0) {
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

  const ct = cart.totals;
  const hint = freeShipHint(cart.freeShipping);

  return (
    <div className="wrap-md screen">
      <h1 className="page-title">Your cart</h1>
      <div className="two-col">
        <div className="col-stack">
          <div className="panel-lines">
            {cart.lines.map((l) => {
              const qtyExceedsStock = !l.soldOut && !l.unavailable && l.qty > l.stock;
              const hasIssue = l.soldOut || l.unavailable || qtyExceedsStock;

              return (
                <div className={`line${hasIssue ? ' line--issue' : ''}`} key={l.productId}>
                  <div className="line-thumb" style={{ background: l.grad }} />
                  <div className="line-main">
                    <div className="line-name">{l.name}</div>
                    <div className="line-variant">{l.stat2}</div>
                    <div className="line-price">
                      <b>{rupeeFromPaise(l.unitPrice)}</b>
                      <span className="mrp">{rupeeFromPaise(l.unitMrp)}</span>
                    </div>
                    {l.soldOut && (
                      <div className="line-warn">Sold out — please remove this item.</div>
                    )}
                    {l.unavailable && !l.soldOut && (
                      <div className="line-warn">No longer available — please remove this item.</div>
                    )}
                    {qtyExceedsStock && (
                      <div className="line-warn">
                        Only {l.stock} in stock — reduce quantity to continue.
                      </div>
                    )}
                  </div>
                  <div className="line-qty">
                    <button
                      className="btn btn-icon"
                      type="button"
                      disabled={cartPending}
                      onClick={() => void actions.setQty(l.productId, l.qty - 1)}
                      aria-label={
                        l.qty === 1
                          ? `Remove ${l.name} from cart`
                          : `Decrease quantity of ${l.name}`
                      }
                    >
                      −
                    </button>
                    <span aria-live="polite">{l.qty}</span>
                    <button
                      className="btn btn-icon"
                      type="button"
                      disabled={cartPending || l.qty >= 99}
                      onClick={() => void actions.setQty(l.productId, l.qty + 1)}
                      aria-label={`Increase quantity of ${l.name}`}
                    >
                      +
                    </button>
                  </div>
                  <div className="line-total">{rupeeFromPaise(l.lineTotal)}</div>
                  <button
                    className="btn btn-icon line-remove"
                    type="button"
                    disabled={cartPending}
                    onClick={() => void actions.removeItem(l.productId)}
                    aria-label={`Remove ${l.name}`}
                  >
                    ✕
                  </button>
                </div>
              );
            })}
          </div>

          <div className="panel">
            <h3>Delivery address</h3>
            <p className="panel-sub">Signed in as +91 {state.user?.phone ?? ''}</p>

            {/* Saved address radio list */}
            {hasAddresses && !state.showAddressForm && (
              <>
                <div
                  className="addr-list"
                  role="radiogroup"
                  aria-label="Saved delivery addresses"
                >
                  {state.addresses.map((addr) => (
                    <AddressCard
                      key={addr.id}
                      mode="cart"
                      addr={addr}
                      selected={state.selectedAddressId === addr.id}
                      addrDeleting={addrDeleting}
                      onSelect={() => dispatch({ type: 'selectAddress', id: addr.id })}
                      onEdit={() => dispatch({ type: 'showAddressForm', editing: addr.id })}
                      onDelete={() => void actions.deleteAddress(addr.id)}
                    />
                  ))}
                </div>
                <button
                  className="btn btn-ghost addr-add-btn"
                  type="button"
                  onClick={() => dispatch({ type: 'showAddressForm', editing: null })}
                >
                  + Add new address
                </button>
                {state.addrError && (
                  <div style={{ marginTop: 8 }}>
                    <ErrorText>{state.addrError}</ErrorText>
                  </div>
                )}
              </>
            )}

            {/* Add/edit form — shown when no saved addresses or form is open */}
            {(!hasAddresses || state.showAddressForm) && (
              <AddressForm
                state={state}
                dispatch={dispatch}
                actions={actions}
                hasExisting={hasAddresses}
              />
            )}
          </div>
        </div>

        <div className="panel aside">
          <h3>Order summary</h3>
          <div className="summary">
            <div className="summary-row">
              <span className="muted">MRP total</span>
              <span className="strike">{rupeeFromPaise(ct.mrpTotal)}</span>
            </div>
            <div className="summary-row">
              <span className="muted">Discount</span>
              <span className="save">− {rupeeFromPaise(ct.savings)}</span>
            </div>
            <div className="summary-row">
              <span className="muted">Delivery</span>
              <span style={{ fontWeight: 600 }}>
                {ct.shipping === 0 ? 'Free' : rupeeFromPaise(ct.shipping)}
              </span>
            </div>
            <div className="rule" style={{ margin: '4px 0' }} />
            <div className="summary-row total">
              <span>To pay</span>
              <span className="summary-total">{rupeeFromPaise(ct.total)}</span>
            </div>
          </div>

          {hint && <div className="ship-hint">{hint}</div>}

          {cart.hasBlockingIssues && (
            <div className="line-warn" style={{ marginBottom: 10 }}>
              Some items are unavailable or out of stock. Please review them above before continuing.
            </div>
          )}

          <button
            className="btn btn-primary btn-block btn-cta"
            type="button"
            disabled={cart.hasBlockingIssues || cartPending || !state.selectedAddressId}
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
