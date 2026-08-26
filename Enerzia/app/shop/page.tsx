import type { Metadata } from 'next';

import { JsonLd } from '@/components/JsonLd';
import { getProductsForRender } from '@/lib/api/catalogue.server';
import type { ProductDTO } from '@/lib/api/types';
import { HOME_PATH, SITE_NAME, SITE_URL, absoluteUrl } from '@/lib/content/site';

import { ShopClient } from './ShopClient';
import { pageMetadata } from '@/lib/seo';

export const metadata: Metadata = pageMetadata({
  path: '/shop',
  title: 'Buy spirulina powder & tablets online | Enerzeia Future Farm',
  description: 'Farm-grown spirulina powder and tablets, sun-dried at low heat and third-party lab tested batch by batch. FSSAI licensed, shipped across India. Free shipping over ₹499.',
  socialTitle: 'Buy spirulina powder & tablets | Enerzeia Future Farm',
});

/**
 * The shop is now rendered from the live catalogue rather than fetched
 * entirely in the browser, so this page revalidates instead of being static.
 *
 * IT MUST BE A LITERAL. Next reads segment config by static analysis at build
 * time, not by evaluating the module, so `export const revalidate = SOME_IMPORT`
 * is rejected outright — "Invalid segment configuration export detected".
 *
 * This is the ONLY place the window is set. `getProductsForRender` deliberately
 * does not pass its own `next.revalidate`, so the fetch inherits this segment's
 * value and there is no second number to keep in step. The reasoning for the
 * length is in lib/api/catalogue.server.ts.
 */
export const revalidate = 300;

/**
 * Schema.org `Product` for one catalogue entry.
 *
 * ---------------------------------------------------------------------------
 * EVERY FIELD HERE IS A CLAIM GOOGLE WILL PRINT IN A SEARCH RESULT, so each
 * one is taken straight from the catalogue and nothing is inferred:
 *
 *  - `price` is `p.price / 100`. Catalogue money is int64 PAISE; emitting the
 *    paise figure would advertise every product at a hundred times its price.
 *    Rendered with toFixed(2) because schema.org wants a plain decimal, not a
 *    localised string - "₹1,548" is invalid here and silently drops the offer.
 *  - `availability` follows `soldOut`, which is the same flag the buy button
 *    uses. Advertising stock we do not have is what earns a merchant listing
 *    suspension, so these two must never diverge.
 *  - `image` is only set when a real Cloudinary photograph exists. The `grad`
 *    fallback is a CSS gradient, not an image, and there is no honest way to
 *    put it in this field.
 *  - No `aggregateRating`. GET /products does not return one, and a rating is
 *    the single most abused field in product structured data - inventing one
 *    is grounds for a manual action against the whole domain.
 * ---------------------------------------------------------------------------
 */
function productSchema(p: ProductDTO): Record<string, unknown> {
  const url = absoluteUrl(`${HOME_PATH}?product=${encodeURIComponent(p.id)}`);
  const image = p.images?.[0]?.url;

  return {
    '@type': 'Product',
    '@id': `${url}#product`,
    name: p.name,
    description: p.blurb,
    category: p.form,
    ...(image ? { image } : {}),
    brand: { '@type': 'Brand', name: SITE_NAME },
    offers: {
      '@type': 'Offer',
      url,
      priceCurrency: 'INR',
      price: (p.price / 100).toFixed(2),
      availability: p.soldOut
        ? 'https://schema.org/OutOfStock'
        : 'https://schema.org/InStock',
      seller: { '@type': 'Organization', name: SITE_NAME, '@id': `${SITE_URL}/#organization` },
    },
  };
}

/**
 * The shop, and — since `/` 308-redirects here — the site's home page.
 *
 * An async SERVER component that reads the catalogue and hands it to the
 * client app as `initialProducts`. Before this, the whole page was a
 * `'use client'` shell: the HTML a crawler received carried 797 characters and
 * not one product name, price or description, because everything arrived by
 * fetch after load. That is a poor page to hang a domain's ranking on, and it
 * became this domain's most important page the moment the root started
 * redirecting here.
 *
 * Two things make this safe to do on the render path:
 *
 *  1. `getProductsForRender` never throws and never blocks for long. If the Go
 *     backend is unreachable — including during `next build` in CI — it
 *     returns null, this renders exactly the shell it always did, and the
 *     client boot fetch behaves as before, banner and all.
 *  2. `ShopClient` seeds its reducer with the same array through
 *     `useReducer`'s init argument, so the first client render matches the
 *     server's and hydration has nothing to reconcile. It then refetches on
 *     boot, so a shopper never acts on a stale price or stock figure.
 *
 * The JSON-LD is emitted only when the catalogue was actually read. Structured
 * data describing an empty shop is worse than none.
 */
export default async function ShopPage() {
  const products = await getProductsForRender();

  return (
    <>
      {products && (
        <JsonLd
          data={{
            '@context': 'https://schema.org',
            '@type': 'CollectionPage',
            '@id': `${absoluteUrl(HOME_PATH)}#collection`,
            name: 'Enerzeia spirulina — powder, tablets and bundles',
            url: absoluteUrl(HOME_PATH),
            mainEntity: {
              '@type': 'ItemList',
              numberOfItems: products.length,
              itemListElement: products.map((p, i) => ({
                '@type': 'ListItem',
                position: i + 1,
                item: productSchema(p),
              })),
            },
          }}
        />
      )}
      <ShopClient initialProducts={products ?? undefined} />
    </>
  );
}
