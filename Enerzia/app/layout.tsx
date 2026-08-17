import type { Metadata } from 'next';
import { Figtree, Playfair_Display, Rubik } from 'next/font/google';

import { Footer } from '@/components/Footer';
import { AnnouncementBar } from '@/components/home/AnnouncementBar';
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

export const metadata: Metadata = {
  title: 'Shop | Enerzeia Future Farm',
  description:
    'Buy Enerzeia Future Farm spirulina — powder, tablets, refill pouches and bundles. Farm-grown, sun-dried at low heat, third-party lab tested.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${playfair.variable} ${figtree.variable} ${brandFont.variable}`}>
      <body>
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
