import type { NextConfig } from 'next';

/*
 * Content-Security-Policy note (do not add a CSP without reading this):
 *
 * Razorpay Checkout loads dynamically and requires several relaxed directives.
 * If a CSP header is ever added, it must include AT MINIMUM:
 *
 *   script-src  https://checkout.razorpay.com 'unsafe-inline' 'unsafe-eval'
 *               (Razorpay's bundle uses eval; 'unsafe-inline' is also required)
 *   frame-src   https://*.razorpay.com
 *   connect-src https://*.razorpay.com
 *
 * Without these, Razorpay Checkout breaks silently in production — the modal
 * never opens and the payment attempt is lost. This failure mode is invisible
 * in development because dev servers rarely enforce CSP.
 */

const nextConfig: NextConfig = {
  reactStrictMode: true,

  /**
   * The bare domain lands on the shop.
   *
   * -------------------------------------------------------------------------
   * WHY 308 AND NOT 307
   *
   * `permanent: true` emits a 308, which is what passes the root URL's
   * accumulated ranking to /shop — a 307 tells search engines the move is
   * temporary and deliberately withholds that. Since this is the intended
   * end state, 308 is correct.
   *
   * THE COST, WHICH IS REAL: browsers cache a 308 indefinitely and ignore the
   * server on later visits. Undoing this is therefore NOT just deleting these
   * lines — anyone who has hit the domain since it shipped keeps redirecting
   * until they clear their cache. If there is any doubt about the destination,
   * change `permanent` to false FIRST, confirm the destination is right, and
   * only then make it permanent.
   *
   * The former homepage now lives at /about; the shop's own header links to it
   * under that label. If this redirect is removed, HOME_PATH in
   * lib/content/site.ts must go back to '/' in the same change — canonicals,
   * the sitemap and the brand lockup all read it.
   * -------------------------------------------------------------------------
   */
  async redirects() {
    return [
      {
        source: '/',
        destination: '/shop',
        permanent: true,
      },
    ];
  },
};

export default nextConfig;
