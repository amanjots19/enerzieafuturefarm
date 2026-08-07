import Image from 'next/image';

import { FILTERS, PRODUCTS, TRUST } from '@/lib/shop/data';
import { cartKey, variantOf } from '@/lib/shop/pricing';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

import { Pill, PriceRow } from './controls';

export function ShopScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  const shown = PRODUCTS.filter((p) => state.filter === 'all' || p.form === state.filter);

  return (
    <div className="wrap screen">
      <div className="hero-grid">
        <div>
          <div className="eyebrow">The Enerzeia shop</div>
          <h1 className="hero-title">
            Everyday spirulina,
            <br />
            in the size your
            <br />
            routine needs.
          </h1>
          <p className="lede">
            Grown in clean water on our own farm, sun-dried at low heat, and tested batch by batch.
            Powder for smoothies, tablets for the days you&rsquo;re running.
          </p>
          <div className="tagrow">
            <span className="tag tag-accent-2">FSSAI licensed</span>
            <span className="tag tag-accent">Third-party lab tested</span>
            <span className="tag tag-neutral">60%+ plant protein</span>
          </div>
        </div>
        <div className="hero-art">
          <Image
            src="/assets/products-coming-soon.jpeg"
            alt="Enerzeia Future Farm spirulina packaging"
            width={800}
            height={600}
            priority
          />
        </div>
      </div>

      <div className="section-head">
        <h2>All products</h2>
        <div className="filters" role="group" aria-label="Filter by product type">
          {FILTERS.map((f) => (
            <Pill
              key={f.k}
              label={f.label}
              active={state.filter === f.k}
              onClick={() => dispatch({ type: 'setFilter', filter: f.k })}
            />
          ))}
        </div>
      </div>

      <div className="pgrid">
        {shown.map((p) => {
          const v = variantOf(p, state.sel);
          /* The cart line for the variant on show, if any — its presence swaps
             the add button for a quantity stepper. */
          const line = state.cart.find((l) => l.key === cartKey(p.id, v.label));
          const sel = state.sel[p.id];

          return (
            <article className="pcard" key={p.id}>
              <button
                className="pcard-art"
                type="button"
                style={{ background: p.grad }}
                onClick={() => dispatch({ type: 'openPdp', id: p.id })}
                aria-label={`View ${p.name}`}
              >
                <span className="tag">{p.form}</span>
              </button>

              <div className="pcard-body">
                <div>
                  <h3 className="pcard-name">
                    <button type="button" onClick={() => dispatch({ type: 'openPdp', id: p.id })}>
                      {p.name}
                    </button>
                  </h3>
                  <div className="pcard-stats">
                    <span>{p.stat}</span>
                    <span className="dot-sep">•</span>
                    <span>{p.stat2}</span>
                  </div>
                </div>

                <div className="variant-row" role="group" aria-label={`Choose size for ${p.name}`}>
                  {p.variants.map((vv, i) => (
                    <Pill
                      key={vv.label}
                      label={vv.label}
                      active={i === sel}
                      onClick={() => dispatch({ type: 'pickVariant', id: p.id, index: i })}
                    />
                  ))}
                </div>

                <PriceRow variant={v} className="price-row" />

                {line ? (
                  <div
                    className="card-qty"
                    role="group"
                    aria-label={`Quantity of ${p.name}, ${v.label}`}
                  >
                    <button
                      className="btn btn-icon"
                      type="button"
                      onClick={() => dispatch({ type: 'bumpLine', key: line.key, delta: -1 })}
                      aria-label={
                        line.qty === 1
                          ? `Remove ${p.name} from cart`
                          : `Decrease quantity of ${p.name}`
                      }
                    >
                      −
                    </button>
                    <span className="card-qty-val" aria-live="polite">
                      {line.qty} <span className="card-qty-unit">in cart</span>
                    </span>
                    <button
                      className="btn btn-icon"
                      type="button"
                      onClick={() => dispatch({ type: 'bumpLine', key: line.key, delta: 1 })}
                      aria-label={`Increase quantity of ${p.name}`}
                    >
                      +
                    </button>
                  </div>
                ) : (
                  <button
                    className="btn btn-primary btn-add"
                    type="button"
                    onClick={() => dispatch({ type: 'addToCart', product: p, variant: v, qty: 1 })}
                  >
                    Add to cart
                  </button>
                )}
              </div>
            </article>
          );
        })}
      </div>

      <div className="trust">
        {TRUST.map((t) => (
          <div key={t.big}>
            <div className="trust-big">{t.big}</div>
            <div className="trust-body">{t.body}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
