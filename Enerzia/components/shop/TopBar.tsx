import Image from 'next/image';

import type { ShopAction } from '@/lib/shop/reducer';

export function TopBar({
  count,
  dispatch,
}: {
  count: number;
  dispatch: (a: ShopAction) => void;
}) {
  return (
    <div className="topbar-wrap">
      <div className="topbar">
        <button
          className="topbar-brand"
          type="button"
          onClick={() => dispatch({ type: 'go', screen: 'shop' })}
          aria-label="Enerzeia Future Farm — go to shop"
        >
          <span className="brand-mark">
            <Image src="/assets/enerzeia-logo.png" alt="" width={30} height={30} />
          </span>
          <span className="brand-text">
            <span className="brand-name">ENERZEIA</span>
            <span className="brand-sub">FUTURE FARM</span>
          </span>
        </button>

        {/* Shop is the only wired link in the design; Benefits and Our Farm are
            placeholders (href="#" upstream) awaiting real routes. */}
        <nav className="topbar-links" aria-label="Primary">
          <button type="button" onClick={() => dispatch({ type: 'go', screen: 'shop' })}>
            Shop
          </button>
          <a href="#">Benefits</a>
          <a href="#">Our Farm</a>
        </nav>

        <button
          className="btn btn-cart"
          type="button"
          onClick={() => dispatch({ type: 'requireAuth', dest: 'cart' })}
        >
          <svg
            width="17"
            height="17"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.75"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <circle cx="8" cy="21" r="1" />
            <circle cx="19" cy="21" r="1" />
            <path d="M2.05 2.05h2l2.66 12.42a2 2 0 0 0 2 1.58h9.78a2 2 0 0 0 1.95-1.57l1.65-7.43H5.12" />
          </svg>
          Cart
          {count > 0 && <span className="cart-badge">{count}</span>}
        </button>
      </div>
    </div>
  );
}
