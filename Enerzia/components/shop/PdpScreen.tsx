import { useEffect, useState } from 'react';

import type { ShopActions } from '@/app/shop/useShop';
import { findProduct, rupeeFromPaise } from '@/lib/shop/pricing';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ProductDTO, ShopState } from '@/lib/shop/types';

import { PillBig, PriceRow } from './controls';

const THUMBS = [
  'linear-gradient(150deg,#cfe9d5,#6ea77f)',
  'linear-gradient(150deg,#dff0d6,#8bbf5c)',
  'linear-gradient(150deg,#c9e4dd,#3f7f68)',
  'linear-gradient(150deg,#e8f2e5,#b6cdae)',
];

/** Extracts the size descriptor from a product name ("Base — Size" → "Size"). */
const sizeLabel = (p: ProductDTO): string =>
  p.name.split(' — ').at(-1) ?? p.name;

export function PdpScreen({
  state,
  dispatch,
  actions,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
}) {
  const p = findProduct(state.pdp, state.products);
  const cartPending = state.pending.cart;

  // Must be declared before any early return (hooks rules).
  const [selectedThumb, setSelectedThumb] = useState(0);
  const [failedUrls, setFailedUrls] = useState<Set<string>>(new Set());
  // Reset to the first image whenever the shopper navigates to a different PDP.
  useEffect(() => {
    setSelectedThumb(0);
    setFailedUrls(new Set());
  }, [p?.id]);

  // Products not yet loaded from the server — show a skeleton.
  if (!p) {
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
        <p style={{ color: 'var(--color-muted)' }}>Loading product…</p>
      </div>
    );
  }

  const detail = state.pdpDetail;
  const siblings = state.products.filter((s) => s.family === p.family);
  const firstImage = p.images[0]; // ImageDTO | undefined; undefined when images: []
  // Hero image: prefer selected thumb, fall back to first, fall back to gradient.
  const heroImage = firstImage ? (p.images[selectedThumb] ?? firstImage) : undefined;
  // Signed in: reflect server cart; signed out: reflect the offline buffer.
  const inCart = state.user ? state.cart?.lines.find((l) => l.productId === p.id) : undefined;
  const inBuffer = !state.user ? state.cartBuffer.find((i) => i.productId === p.id) : undefined;
  const existingQty = inCart?.qty ?? inBuffer?.qty ?? 0;

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
          <div
            className="pdp-art"
            style={heroImage && !failedUrls.has(heroImage.url) ? undefined : { background: p.grad }}
          >
            {heroImage && !failedUrls.has(heroImage.url) && (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                className="pdp-art-img"
                src={heroImage.url}
                alt={heroImage.alt || p.name}
                onError={() => setFailedUrls((prev) => new Set([...prev, heroImage.url]))}
              />
            )}
          </div>

          {p.images.length >= 1 ? (
            <div className="pdp-thumbs">
              {p.images.map((img, i) => (
                <button
                  key={img.publicId}
                  type="button"
                  className={`pdp-thumb-btn${i === selectedThumb ? ' pdp-thumb-btn--active' : ''}`}
                  onClick={() => setSelectedThumb(i)}
                  aria-label={img.alt || `${p.name} photo ${i + 1}`}
                >
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img className="pdp-thumb-img" src={img.url} alt="" aria-hidden="true" />
                </button>
              ))}
            </div>
          ) : (
            <div className="pdp-thumbs" aria-hidden="true">
              {THUMBS.map((g) => (
                <div key={g} style={{ background: g }} />
              ))}
            </div>
          )}
        </div>

        <div>
          <div className="eyebrow eyebrow-sm">{p.form}</div>
          <h1 className="pdp-title">{p.name}</h1>
          <div className="pdp-rating">
            <span className="stars" aria-hidden="true">
              ★★★★★
            </span>
            {detail ? (
              <span>
                {detail.rating.score} · {detail.rating.count} reviews
              </span>
            ) : (
              <span style={{ color: 'var(--color-muted)' }}>Loading…</span>
            )}
          </div>
          <p className="pdp-blurb">{p.blurb}</p>
          <div className="rule" style={{ margin: '22px 0' }} />

          {siblings.length > 1 && (
            <>
              <div className="label-cap" id="size-label">
                Choose your size
              </div>
              <div
                className="variant-row"
                style={{ marginBottom: 20 }}
                role="group"
                aria-labelledby="size-label"
              >
                {siblings.map((sib) => (
                  <PillBig
                    key={sib.id}
                    label={sizeLabel(sib)}
                    sub={sib.stat2}
                    active={sib.id === p.id}
                    soldOut={sib.soldOut}
                    onClick={() => dispatch({ type: 'openPdp', id: sib.id })}
                  />
                ))}
              </div>
            </>
          )}

          <PriceRow
            price={p.price}
            mrp={p.mrp}
            discountPercent={p.discountPercent}
            className="pdp-price-row"
          />

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
            {p.soldOut ? (
              <div
                className="btn btn-primary btn-buy"
                style={{ opacity: 0.45, cursor: 'default' }}
              >
                Sold out
              </div>
            ) : (
              <button
                className="btn btn-primary btn-buy"
                type="button"
                disabled={cartPending}
                onClick={() => void actions.addFromPdp()}
              >
                {existingQty > 0
                  ? `Add more (${existingQty} ${inCart ? 'in cart' : 'selected'})`
                  : 'Add to cart'}
              </button>
            )}
          </div>

          <div className="pdp-badges">
            {detail ? (
              detail.badges.map((b) => (
                <div className="pdp-badge" key={b.title}>
                  <b>{b.title}</b>
                  <span>{b.subtitle}</span>
                </div>
              ))
            ) : (
              <div style={{ color: 'var(--color-muted)', fontSize: 13 }}>Loading…</div>
            )}
          </div>

          {detail && (
            <div className="panel-nutrition">
              <h4>Nutrition per {detail.nutrition.servingSize}</h4>
              <table className="table">
                <tbody>
                  {detail.nutrition.rows.map((n) => (
                    <tr key={n.key}>
                      <td>{n.key}</td>
                      <td>{n.value}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
