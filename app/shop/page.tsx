import type { Metadata } from 'next';

import { ShopClient } from './ShopClient';

export const metadata: Metadata = {
  title: 'Shop | Enerzeia Future Farm',
  description:
    'Buy Enerzeia Future Farm spirulina — powder, tablets, refill pouches and bundles. Farm-grown, sun-dried at low heat, third-party lab tested.',
};

export default function ShopPage() {
  return <ShopClient />;
}
