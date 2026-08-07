import { NUTRITION, PDP_BADGES } from '@/lib/shop/data';
import { findProduct, variantOf } from '@/lib/shop/pricing';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

import { PillBig, PriceRow } from './controls';

const THUMBS = [
  'linear-gradient(150deg,#cfe9d5,#6ea77f)',
  'linear-gradient(150deg,#dff0d6,#8bbf5c)',
  'linear-gradient(150deg,#c9e4dd,#3f7f68)',
  'linear-gradient(150deg,#e8f2e5,#b6cdae)',
];

export function PdpScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  const p = findProduct(state.pdp);
  const v = variantOf(p, state.sel);
  const sel = state.sel[p.id];

  return (
    <div className="wrap screen">
      <button
        className="btn btn-ghost"
        type="button"
        style={{ marginBottom: 16 }}
        onClick={() => dispatch({ type: 'go', screen: 'shop' })}
      >
        ← Back to shop
      </button>

      <div className="pdp-grid">
        <div>
          <div className="pdp-art" style={{ background: p.grad }} />
          <div className="pdp-thumbs" aria-hidden="true">
            {THUMBS.map((g) => (
              <div key={g} style={{ background: g }} />
            ))}
          </div>
        </div>

        <div>
          <div className="eyebrow eyebrow-sm">{p.form}</div>
          <h1 className="pdp-title">{p.name}</h1>
          <div className="pdp-rating">
            <span className="stars" aria-hidden="true">
              ★★★★★
            </span>
            <span>4.8 · 312 reviews</span>
          </div>
          <p className="pdp-blurb">{p.blurb}</p>
          <div className="rule" style={{ margin: '22px 0' }} />

          <div className="label-cap" id="size-label">
            Choose your size
          </div>
          <div
            className="variant-row"
            style={{ marginBottom: 20 }}
            role="group"
            aria-labelledby="size-label"
          >
            {p.variants.map((vv, i) => (
              <PillBig
                key={vv.label}
                variant={vv}
                active={i === sel}
                onClick={() => dispatch({ type: 'pickVariant', id: p.id, index: i })}
              />
            ))}
          </div>

          <PriceRow variant={v} className="pdp-price-row" />

          <div className="pdp-buy">
            <div className="qty">
              <button
                className="btn btn-icon"
                type="button"
                onClick={() => dispatch({ type: 'bumpQty', delta: -1 })}
                aria-label="Decrease quantity"
              >
                −
              </button>
              <span className="qty-val" aria-live="polite">
                {state.qty}
              </span>
              <button
                className="btn btn-icon"
                type="button"
                onClick={() => dispatch({ type: 'bumpQty', delta: 1 })}
                aria-label="Increase quantity"
              >
                +
              </button>
            </div>
            <button
              className="btn btn-primary btn-buy"
              type="button"
              onClick={() => dispatch({ type: 'addFromPdp' })}
            >
              Add to cart
            </button>
          </div>

          <div className="pdp-badges">
            {PDP_BADGES.map((b) => (
              <div className="pdp-badge" key={b.t}>
                <b>{b.t}</b>
                <span>{b.s}</span>
              </div>
            ))}
          </div>

          <div className="panel-nutrition">
            <h4>Nutrition per 5 g serving</h4>
            <table className="table">
              <tbody>
                {NUTRITION.map((n) => (
                  <tr key={n.k}>
                    <td>{n.k}</td>
                    <td>{n.v}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}
