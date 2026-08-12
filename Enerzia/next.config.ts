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
};

export default nextConfig;
