import type { Metadata } from 'next';

import { OG_IMAGE, SITE_NAME, absoluteUrl } from './content/site';

/**
 * Builds a page's `metadata` export: title, description, canonical, and
 * matching OpenGraph and Twitter cards.
 *
 * ---------------------------------------------------------------------------
 * WHY THIS EXISTS RATHER THAN EACH PAGE WRITING ITS OWN OBJECT
 *
 * Next merges metadata from the layout SHALLOWLY, one key at a time. That is
 * easy to misread, and it cost this site both of these before the helper
 * landed:
 *
 *  1. A page setting `openGraph: { url }` REPLACES the layout's entire
 *     `openGraph` object — it does not merge into it. The shared `images`
 *     disappeared, so every page except the shop shipped with no `og:image`
 *     and rendered as a bare text link when shared.
 *  2. `twitter` is a separate key and is NOT derived from `openGraph`. A page
 *     that set only `openGraph` inherited the LAYOUT's twitter title and
 *     description, so the card advertised the site tagline no matter which
 *     page was pasted.
 *
 * Both are silent: nothing errors, the page looks fine, and you find out when
 * somebody shares a link. Building all three from one set of arguments means
 * they cannot drift.
 * ---------------------------------------------------------------------------
 */
export function pageMetadata({
  path,
  title,
  description,
  /** Card title, when the `<title>` is too long or too keyword-shaped to share well. */
  socialTitle,
  /** Page-specific card image. Defaults to the site's. */
  image = OG_IMAGE,
}: {
  path: string;
  title: string;
  description: string;
  socialTitle?: string;
  image?: typeof OG_IMAGE;
}): Metadata {
  const url = absoluteUrl(path);
  const cardTitle = socialTitle ?? title;

  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: {
      type: 'website',
      siteName: SITE_NAME,
      locale: 'en_IN',
      url,
      title: cardTitle,
      description,
      images: [image],
    },
    twitter: {
      card: 'summary_large_image',
      title: cardTitle,
      description,
      images: [image.url],
    },
  };
}
