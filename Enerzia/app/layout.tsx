import type { Metadata } from 'next';
import { Figtree, Playfair_Display, Rubik } from 'next/font/google';

import { Footer } from '@/components/Footer';
import { JsonLd } from '@/components/JsonLd';
import { AnnouncementBar } from '@/components/home/AnnouncementBar';
import {
  HOME_PATH,
  OG_IMAGE,
  POSTAL,
  SITE_NAME,
  SITE_TAGLINE,
  SITE_URL,
  absoluteUrl,
} from '@/lib/content/site';
import { SOCIAL_LINKS, SUPPORT_EMAIL, SUPPORT_WHATSAPP } from '@/lib/content/social';
import '@/styles/ds-organic.css';
import '@/styles/shop.css';

/* styles/shop.css reads these two variables for --font-heading / --font-body. */
const playfair = Playfair_Display({
  subsets: ['latin'],
  weight: ['500', '600', '700'],
  variable: '--font-playfair',
  display: 'swap',
});

const figtree = Figtree({
  subsets: ['latin'],
  weight: ['400', '500', '600', '700'],
  variable: '--font-figtree',
  display: 'swap',
});

/* Brand wordmark only — NOT headings. OPTION C: Rubik 600 italic — softened
   corners and a wider geometric structure, closer to the wordmark's roundness
   than Poppins. Scoped to .brand-name / .brand-sub. */
const brandFont = Rubik({
  subsets: ['latin'],
  weight: ['600'],
  style: ['italic'],
  variable: '--font-brand',
  display: 'swap',
});

/**
 * Site-wide metadata defaults. Every page overrides `title` and `description`;
 * what is set here is the machinery those overrides need to work.
 *
 * `metadataBase` is the one that matters most. Without it Next emits RELATIVE
 * OpenGraph and canonical URLs, and a relative `og:image` is simply dropped by
 * every scraper — the link previews look broken and nobody can tell why. It
 * also lets each page write `alternates.canonical` as a path.
 *
 * `title.template` appends the brand to any page that sets a plain string
 * title. Pages whose own title already ends in the brand use `title.absolute`
 * or just include it, so nothing is doubled.
 */
export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: `${SITE_NAME} — farm-grown spirulina powder & tablets`,
    template: `%s`,
  },
  description: SITE_TAGLINE,
  applicationName: SITE_NAME,
  alternates: { canonical: absoluteUrl(HOME_PATH) },
  openGraph: {
    type: 'website',
    siteName: SITE_NAME,
    locale: 'en_IN',
    url: absoluteUrl(HOME_PATH),
    title: `${SITE_NAME} — farm-grown spirulina powder & tablets`,
    description: SITE_TAGLINE,
    images: [OG_IMAGE],
  },
  twitter: {
    card: 'summary_large_image',
    title: `${SITE_NAME} — farm-grown spirulina powder & tablets`,
    description: SITE_TAGLINE,
    images: [OG_IMAGE.url],
  },
  robots: {
    index: true,
    follow: true,
    googleBot: { index: true, follow: true, 'max-image-preview': 'large' },
  },
  formatDetection: { telephone: false },
};

/**
 * Who we are, once, for every page.
 *
 * `@id` is the stable identifier the rest of the site's structured data points
 * at — each product Offer names this organization as its `seller` by this same
 * `@id`, which is what ties the graph together instead of leaving a dozen
 * unrelated mentions of the same name.
 *
 * `sameAs` is the list of profiles we control. It is the strongest signal
 * available for entity disambiguation, so it reads from SOCIAL_LINKS rather
 * than restating the URLs — one list, already curated (the QR referral
 * parameters were stripped there for exactly this kind of reuse).
 *
 * NO `telephone`. There is no published phone number — see the standing note
 * in lib/content/social.ts. WhatsApp is the staffed channel and is listed as a
 * contactPoint URL instead, which is truthful; a `telephone` field would put a
 * dialable number in a knowledge panel and ring an unattended phone.
 */
const organizationSchema = {
  '@context': 'https://schema.org',
  '@type': 'Organization',
  '@id': `${SITE_URL}/#organization`,
  name: SITE_NAME,
  url: absoluteUrl(HOME_PATH),
  logo: absoluteUrl('/assets/enerzeia-logo.png'),
  image: absoluteUrl(OG_IMAGE.url),
  description: SITE_TAGLINE,
  email: SUPPORT_EMAIL,
  address: { '@type': 'PostalAddress', ...POSTAL },
  contactPoint: {
    '@type': 'ContactPoint',
    contactType: 'customer support',
    email: SUPPORT_EMAIL,
    url: `https://wa.me/${SUPPORT_WHATSAPP}`,
    areaServed: 'IN',
    availableLanguage: ['en', 'hi'],
  },
  sameAs: SOCIAL_LINKS.map((s) => s.href),
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    // en-IN, not en: the prices are rupees, the shipping is domestic and the
    // support hours are IST. It costs nothing and tells a crawler which
    // English-speaking market this is for.
    <html lang="en-IN" className={`${playfair.variable} ${figtree.variable} ${brandFont.variable}`}>
      <body>
        {/* One Organization block for the whole site, in the layout so it is
            on every route rather than only the ones somebody remembered. */}
        <JsonLd data={organizationSchema} />

        {/* Announcement bar and footer are sitewide, so the region above and
            below the content is identical on every route. The bar sat only on
            the homepage before, which was half of why the two headers looked
            like different sites. */}
        <AnnouncementBar />
        {children}
        <Footer />
      </body>
    </html>
  );
}
