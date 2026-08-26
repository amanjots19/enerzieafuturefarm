import { SiteHeader, type NavItem } from '@/components/SiteHeader';
import { AccountControl } from '@/components/shop/AccountControl';
import type { ShopAction } from '@/lib/shop/reducer';
import type { UserDTO } from '@/lib/shop/types';

/**
 * The shop's header. A thin configuration of the shared SiteHeader rather than
 * a second header — the two used to be a full-width bar and a rounded pill,
 * which read as two different sites.
 *
 * What is shop-specific lives here: the "Home" entry (absent on the homepage,
 * which IS home), a "Shop" entry that dispatches instead of navigating because
 * this is a SPA, and the account control and cart button in the right slot.
 */
export function TopBar({
  count,
  booting,
  user,
  signOut,
  dispatch,
}: {
  count: number;
  booting: boolean;
  user: UserDTO | null;
  signOut: () => void;
  dispatch: (a: ShopAction) => void;
}) {
  // Three entries out of the shop: the landing page, labelled "About us" rather
  // than "Home" because the label describes what is there, the benefits page
  // and the farm page. Moving between the shop's own screens is handled by the
  // in-screen "← Back to shop" links, not the header.
  //
  // This is the only header carrying a right slot as well, so it is the one
  // that runs out of room first — below 560px SiteHeader's burger takes all
  // three labels, and above it they fit inline beside the account and cart.
  const nav: NavItem[] = [
    { label: 'About us', href: '/about' },
    { label: 'Benefits', href: '/benefits' },
    { label: 'Our Farm', href: '/farm' },
  ];

  return (
    <SiteHeader
      nav={nav}
      right={
        <>
          <AccountControl user={user} dispatch={dispatch} signOut={signOut} />
          <button
            className="btn btn-cart"
            type="button"
            disabled={booting}
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
            <span className="btn-cart-label">Cart</span>
            {!booting && count > 0 && <span className="cart-badge">{count}</span>}
          </button>
        </>
      }
    />
  );
}
