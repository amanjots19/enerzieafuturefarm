import type { MetadataRoute } from 'next';

import { SITE_URL, absoluteUrl } from '@/lib/content/site';

/**
 * /robots.txt
 *
 * Deliberately permissive — this is a nine-product storefront, not a site with
 * anything to hide from a crawler. The value here is the `sitemap` line, which
 * is how a crawler finds the URL list without waiting to stumble across every
 * page through internal links.
 *
 * Nothing is disallowed, and that is correct rather than an oversight. The
 * shop's private screens (cart, orders, addresses, checkout) are NOT separate
 * URLs — they are screens inside the client app at /shop, reachable only after
 * an OTP sign-in, so there is no path to exclude. If they ever become real
 * routes, add them here AND check they are noindex'd; a Disallow alone still
 * lets an externally-linked URL be indexed without being crawled.
 */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [{ userAgent: '*', allow: '/' }],
    sitemap: absoluteUrl('/sitemap.xml'),
    host: SITE_URL,
  };
}
