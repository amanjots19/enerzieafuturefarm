import type { MetadataRoute } from 'next';

import { HOME_PATH, absoluteUrl } from '@/lib/content/site';

/**
 * /sitemap.xml
 *
 * ---------------------------------------------------------------------------
 * `/` IS DELIBERATELY ABSENT.
 *
 * It 308-redirects to /shop (see next.config.ts). Listing a URL that redirects
 * tells a crawler to fetch it, follow the hop and discard it — every time —
 * and mixing a redirecting URL into a sitemap is a common way to muddy which
 * page a site actually wants ranked. HOME_PATH is the destination and the
 * destination is what is listed.
 * ---------------------------------------------------------------------------
 *
 * `priority` is a hint about RELATIVE importance within this site only; it
 * says nothing to a crawler about other domains and does not raise rankings.
 * The ordering below is the honest one: the shop is what the business is for,
 * the three content pages support it, the legal pages exist because they must.
 *
 * `changeFrequency` is likewise advisory. The shop is 'daily' because stock
 * and prices move; the legal documents are 'yearly' because they genuinely do
 * not change between revisions.
 *
 * NO PER-PRODUCT URLS. The shop is a single-page app — a product opens a PDP
 * screen inside /shop rather than navigating to its own route — so there is no
 * distinct URL to list. Listing invented ones would fill Search Console with
 * 404s. If products ever get real routes (/shop/[id]), add them here by
 * reading the catalogue, and give each one its own canonical.
 */
export default function sitemap(): MetadataRoute.Sitemap {
  const lastModified = new Date();

  return [
    { url: absoluteUrl(HOME_PATH), lastModified, changeFrequency: 'daily', priority: 1 },
    { url: absoluteUrl('/benefits'), lastModified, changeFrequency: 'monthly', priority: 0.9 },
    { url: absoluteUrl('/farm'), lastModified, changeFrequency: 'monthly', priority: 0.8 },
    { url: absoluteUrl('/about'), lastModified, changeFrequency: 'monthly', priority: 0.7 },
    { url: absoluteUrl('/faq'), lastModified, changeFrequency: 'monthly', priority: 0.6 },
    { url: absoluteUrl('/contact'), lastModified, changeFrequency: 'yearly', priority: 0.5 },
    { url: absoluteUrl('/terms'), lastModified, changeFrequency: 'yearly', priority: 0.2 },
    { url: absoluteUrl('/privacy'), lastModified, changeFrequency: 'yearly', priority: 0.2 },
  ];
}
