import Image from 'next/image';
import { useEffect, useState } from 'react';

import type { ShopActions } from '@/app/shop/useShop';
import type { ImageDTO } from '@/lib/api/types';
import { FILTERS } from '@/lib/shop/data';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

import { Pill, PriceRow } from './controls';

function CardArt({
  image,
  grad,
  name,
  form,
  onClick,
}: {
  image: ImageDTO | undefined;
  grad: string;
  name: string;
  form: string;
  onClick: () => void;
}) {
  const [imgFailed, setImgFailed] = useState(false);
  useEffect(() => { setImgFailed(false); }, [image?.url]);
  const showGrad = !image || imgFailed;
  return (
    <button
      className="pcard-art"
      type="button"
      style={showGrad ? { background: grad } : undefined}
      onClick={onClick}
      aria-label={`View ${name}`}
    >
      {image && !imgFailed && (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          className="pcard-art-img"
          src={image.url}
          alt={image.alt || name}
          onError={() => setImgFailed(true)}
        />
      )}
      <span className="tag">{form}</span>
    </button>
  );
}

export function ShopScreen({
  state,
  dispatch,
  actions,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
}) {
  const shown = state.products.filter(
    (p) => state.filter === 'all' || p.form === state.filter,
  );
  const cartPending = state.pending.cart;

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
            <span className="tag tag-neutral">62%+ plant protein</span>
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
          // Signed in: use the live server cart line.
          // Signed out: use the local offline buffer entry.
          const line = state.user ? state.cart?.lines.find((l) => l.productId === p.id) : undefined;
          const bufferItem = !state.user
            ? state.cartBuffer.find((i) => i.productId === p.id)
            : undefined;

          const primaryImage = p.images[0]; // ImageDTO | undefined; undefined when images: []

          return (
            <article className={`pcard${p.soldOut ? ' pcard--sold-out' : ''}`} key={p.id}>
              <CardArt
                image={primaryImage}
                grad={p.grad}
                name={p.name}
                form={p.form}
                onClick={() => dispatch({ type: 'openPdp', id: p.id })}
              />

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

                <PriceRow
                  price={p.price}
                  mrp={p.mrp}
                  discountPercent={p.discountPercent}
                  className="price-row"
                />

                {p.soldOut ? (
                  <div className="btn btn-primary btn-add" style={{ opacity: 0.45, cursor: 'default' }}>
                    Sold out
                  </div>
                ) : line ? (
                  // Signed-in stepper: drives the server cart.
                  <div
                    className="card-qty"
                    role="group"
                    aria-label={`Quantity of ${p.name}`}
                  >
                    <button
                      className="btn btn-icon"
                      type="button"
                      disabled={cartPending}
                      onClick={() => void actions.setQty(p.id, line.qty - 1)}
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
                      disabled={cartPending || line.qty >= 99}
                      onClick={() => void actions.setQty(p.id, line.qty + 1)}
                      aria-label={`Increase quantity of ${p.name}`}
                    >
                      +
                    </button>
                  </div>
                ) : bufferItem ? (
                  // Signed-out stepper: drives the local offline buffer.
                  <div
                    className="card-qty"
                    role="group"
                    aria-label={`Quantity of ${p.name}`}
                  >
                    <button
                      className="btn btn-icon"
                      type="button"
                      onClick={() => actions.setBufferQty(p.id, bufferItem.qty - 1)}
                      aria-label={
                        bufferItem.qty === 1
                          ? `Remove ${p.name} from selection`
                          : `Decrease quantity of ${p.name}`
                      }
                    >
                      −
                    </button>
                    <span className="card-qty-val" aria-live="polite">
                      {bufferItem.qty} <span className="card-qty-unit">selected</span>
                    </span>
                    <button
                      className="btn btn-icon"
                      type="button"
                      disabled={bufferItem.qty >= 99}
                      onClick={() => actions.setBufferQty(p.id, bufferItem.qty + 1)}
                      aria-label={`Increase quantity of ${p.name}`}
                    >
                      +
                    </button>
                  </div>
                ) : (
                  <button
                    className="btn btn-primary btn-add"
                    type="button"
                    disabled={cartPending}
                    onClick={() => void actions.addItem(p.id, 1)}
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
        {state.trust.map((t) => (
          <div key={t.big}>
            <div className="trust-big">{t.big}</div>
            <div className="trust-body">{t.body}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
