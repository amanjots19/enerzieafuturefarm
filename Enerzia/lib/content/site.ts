/**
 * Site-level identity: the canonical origin, and the facts search engines and
 * social cards read.
 *
 * ---------------------------------------------------------------------------
 * WHY THE ORIGIN IS HARDCODED WITH AN ENV OVERRIDE
 *
 * `metadataBase`, the sitemap, robots.txt, every canonical link and every
 * JSON-LD `@id` need an absolute origin, and Next has no way to infer one at
 * build time. A wrong origin here is not a cosmetic bug: it points canonicals
 * and structured data at a domain we do not own, which is how a site
 * de-indexes itself.
 *
 * `NEXT_PUBLIC_SITE_URL` overrides it so a preview deployment does not claim
 * to be production. Set it on the preview environment; leave it unset in
 * production so the value below is used.
 * ---------------------------------------------------------------------------
 */

/** Canonical origin, no trailing slash. */
export const SITE_URL = (
  process.env.NEXT_PUBLIC_SITE_URL ?? 'https://enerzeiafuturefarm.com'
).replace(/\/$/, '');

/** Absolute URL for a site-relative path. */
export function absoluteUrl(path: string): string {
  return `${SITE_URL}${path.startsWith('/') ? path : `/${path}`}`;
}

/**
 * The commercial home page.
 *
 * `/` 308-redirects here (see next.config.ts), so this — not `/` — is the URL
 * that carries the domain's ranking, the one the sitemap lists first and the
 * one the brand lockup links to. If that redirect is ever removed, change this
 * back to '/' and the sitemap, canonicals and lockup follow automatically.
 */
export const HOME_PATH = '/shop';

export const SITE_NAME = 'Enerzeia Future Farm';

/**
 * Used as the OpenGraph/Twitter description wherever a page does not set its
 * own, and as the `description` of the Organization JSON-LD.
 */
export const SITE_TAGLINE =
  'Spirulina grown, harvested, dried and packed on our own farm in Haryana. FSSAI licensed and third-party lab tested, batch by batch.';

/**
 * Default social card image, 1200x630.
 *
 * Cropped from the benefits band photograph, which is the only landscape
 * source in the repo wide enough for the 1.91:1 ratio without upscaling. It
 * shows a pack — see the images note in lib/content/benefits.ts for why that
 * is the owner's standing decision rather than something to "fix" here.
 */
export const OG_IMAGE = {
  url: '/assets/og-default.jpg',
  width: 1200,
  height: 630,
  alt: 'A glass of spirulina being stirred at a sunlit kitchen counter beside an Enerzeia Spirulina jar.',
};

/**
 * Registered business address. Mirrors POSTAL_ADDRESS in ./social.ts, split
 * into the fields schema.org's PostalAddress expects — that structure is what
 * makes it eligible for a knowledge panel, which a single string is not.
 *
 * KEEP THESE TWO IN STEP. ./social.ts stays the human-readable source; this is
 * the same address parsed, not a second address.
 */
export const POSTAL = {
  streetAddress: 'Village Saimpal, Kalanaur',
  addressLocality: 'Rohtak',
  addressRegion: 'Haryana',
  postalCode: '124113',
  addressCountry: 'IN',
};
