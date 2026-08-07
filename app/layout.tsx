import type { Metadata } from 'next';
import { Figtree, Playfair_Display } from 'next/font/google';

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

export const metadata: Metadata = {
  title: 'Shop | Enerzeia Future Farm',
  description:
    'Buy Enerzeia Future Farm spirulina — powder, tablets, refill pouches and bundles. Farm-grown, sun-dried at low heat, third-party lab tested.',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${playfair.variable} ${figtree.variable}`}>
      <body>{children}</body>
    </html>
  );
}
