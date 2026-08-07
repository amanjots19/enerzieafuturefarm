import { PAY_LABELS } from '@/lib/shop/data';
import { addressSummary, lineShort, rupee } from '@/lib/shop/pricing';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

export function DoneScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  const order = state.order;

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

        <h1>Order placed</h1>
        <p className="done-lead">Thank you — we&rsquo;re packing it at the farm.</p>
        <p className="done-meta">
          Order <strong>{state.orderId}</strong> · arriving in 3–5 days
        </p>

        <div className="done-panel">
          <div className="label-cap">Shipping to</div>
          <div className="done-addr">{addressSummary(state.addr, state.user)}</div>
          <div className="rule" style={{ margin: '16px 0' }} />
          <div className="done-lines">
            {order?.lines.map((l) => (
              <div className="mini-line" key={l.key}>
                <span>{lineShort(l)}</span>
                <span>{rupee(l.price * l.qty)}</span>
              </div>
            ))}
            <div className="done-paid">
              <b>Paid via {order ? PAY_LABELS[order.pay] : ''}</b>
              <span className="summary-total">{order ? rupee(order.total) : ''}</span>
            </div>
          </div>
        </div>

        <button
          className="btn btn-primary"
          type="button"
          style={{ padding: '13px 26px', marginTop: 22 }}
          onClick={() => dispatch({ type: 'go', screen: 'shop' })}
        >
          Continue shopping
        </button>
      </div>
    </div>
  );
}
