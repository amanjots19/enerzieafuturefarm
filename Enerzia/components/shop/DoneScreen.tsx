import { addressDTOSummary, rupeeFromPaise } from '@/lib/shop/pricing';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

export function DoneScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  const order = state.placedOrder;
  const placed = order?.status === 'placed';

  return (
    <div className="wrap-xs screen">
      <div className="done-card">
        <div className="done-check" aria-hidden="true">
          <svg
            width="34"
            height="34"
            viewBox="0 0 24 24"
            fill="none"
            stroke="#4c7026"
            strokeWidth="2.75"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M20 6 9 17l-5-5" />
          </svg>
        </div>

        {placed ? (
          <>
            <h1>Order placed</h1>
            <p className="done-lead">Thank you — we&rsquo;re packing it at the farm.</p>
            <p className="done-meta">
              Order <strong>{order.orderId}</strong>
              {order.etaText ? ` · ${order.etaText}` : ' · arriving in 3–5 days'}
            </p>

            <div className="done-panel">
              <div className="label-cap">Shipping to</div>
              <div className="done-addr">
                <strong>{order.shippingAddress.name}</strong>
                <br />
                {addressDTOSummary(order.shippingAddress)}
                <br />
                +91 {state.user?.phone ?? ''}
              </div>
              <div className="rule" style={{ margin: '16px 0' }} />
              <div className="done-lines">
                {order.lines.map((l) => (
                  <div className="mini-line" key={l.productId}>
                    <span>{l.name} × {l.qty}</span>
                    <span>{rupeeFromPaise(l.lineTotal)}</span>
                  </div>
                ))}
                <div className="done-paid">
                  <b>Paid via {order.payment?.label ?? 'Razorpay'}</b>
                  <span className="summary-total">{rupeeFromPaise(order.totals.total)}</span>
                </div>
              </div>
            </div>
          </>
        ) : (
          <>
            {/* Degraded success: payment captured but server not yet confirmed.
                The webhook will place the order momentarily. Never show a failure
                to someone who has just paid. */}
            <h1>Payment received</h1>
            <p className="done-lead">
              We&rsquo;ve received your payment and are confirming it.
            </p>
            {order && (
              <p className="done-meta">Order <strong>{order.orderId}</strong></p>
            )}
          </>
        )}

        <button
          className="btn btn-primary"
          type="button"
          style={{ padding: '13px 26px', marginTop: 22 }}
          onClick={() => dispatch({ type: 'go', screen: 'shop' })}
        >
          Back to shop
        </button>
      </div>
    </div>
  );
}
