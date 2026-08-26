import { API_BASE } from './config';
import type { ProductDTO, ProductListData } from './types';

/**
 * Server-side catalogue read, for rendering the shop's HTML.
 *
 * SERVER ONLY. Do not import this from a `'use client'` module — the client
 * has `lib/api/catalogue.ts` for the same endpoint, and that one carries the
 * bearer token and the shared error envelope handling this deliberately skips.
 *
 * ---------------------------------------------------------------------------
 * WHY THIS EXISTS RATHER THAN REUSING `request()` FROM ./http.ts
 *
 * `request()` is the shopper's client: it attaches the bearer token, fires the
 * global 401 hook, and throws typed errors that the reducer turns into a
 * banner. None of that is wanted here. This runs with no shopper, during a
 * render that must not fail, and the only two outcomes it needs are "here are
 * the products" and "carry on without them".
 *
 * ---------------------------------------------------------------------------
 * IT RETURNS null RATHER THAN THROWING, AND THAT IS THE WHOLE POINT
 *
 * The catalogue lives in the Go backend, which is a separate process that can
 * be down, slow, or simply not running during `next build` in CI. Before this
 * existed, /shop rendered a shell and fetched everything in the browser, so an
 * unreachable API cost a banner and nothing else. That must stay true: a
 * marketing page for a spirulina farm should not 500 because an API call timed
 * out.
 *
 * So every failure - timeout, refused connection, non-2xx, malformed body -
 * lands on `null`, the page renders exactly the shell it rendered before, and
 * the client boot fetch shows its usual "unable to reach the server" banner.
 * The ONLY thing lost is the server-rendered product markup.
 * ---------------------------------------------------------------------------
 */

/**
 * Hard ceiling on the API call. A page render blocks on this, so it is short:
 * a slow catalogue must degrade to the client-fetched shell rather than hold
 * the whole document open.
 */
const TIMEOUT_MS = 3_000;

/**
 * HOW LONG A RENDERED CATALOGUE IS REUSED — set by the page, not here.
 *
 * The window lives in `export const revalidate` in app/shop/page.tsx, because
 * Next requires that to be a static literal and will not accept an imported
 * constant. This fetch therefore passes NO `next.revalidate` of its own and
 * inherits the segment's, which keeps a single number in a single place rather
 * than two that can drift apart.
 *
 * What that number means: the catalogue is MANAGER-OWNED — prices, stock and
 * copy are edited in the admin, not in this repo — so it is the delay between
 * an owner's edit and the public HTML showing it. It trades a little staleness
 * for not calling the backend on every crawl of every product.
 *
 * The client boot fetch is NOT cached and runs on every load, so a shopper
 * always sees live stock and prices within a moment of the page appearing.
 * This window only affects what a crawler reads.
 *
 * `tags` is kept so the admin can purge this on demand with
 * `revalidateTag('catalogue')` the moment an edit is published, rather than
 * waiting the window out.
 */

/** The API's success envelope. Mirrors `DataEnvelope` in ./http.ts. */
interface Envelope {
  data?: ProductListData;
}

/**
 * Fetches the product list for server rendering.
 *
 * @returns the products, or `null` if the catalogue could not be read for any
 *          reason. Never throws.
 */
export async function getProductsForRender(): Promise<ProductDTO[] | null> {
  try {
    const res = await fetch(`${API_BASE}/products`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(TIMEOUT_MS),
      next: { tags: ['catalogue'] },
    });

    if (!res.ok) return null;

    const body: unknown = await res.json();
    const products = (body as Envelope | null)?.data?.products;

    // An empty array is a real answer ("the shop is empty"), but it is
    // indistinguishable from a misconfigured API here, and rendering an empty
    // grid into the HTML is worse than rendering the shell. Treat it as absent
    // and let the client fetch settle it.
    return Array.isArray(products) && products.length > 0 ? products : null;
  } catch {
    return null;
  }
}
