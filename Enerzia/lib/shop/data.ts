import type { Filter, PayMethod, Product } from './types';

/* Content carried over verbatim from the Claude Design file
   "Spirulina Shop.dc.html" (the PRODUCTS / FILTERS constants and the
   renderVals() literals in its DCLogic script). */

export const PRODUCTS: readonly Product[] = [
  {
    id: 'powder',
    form: 'Powder',
    name: 'Pure Spirulina Powder',
    stat: '60% plant protein',
    stat2: 'Sun-dried, unflavoured',
    blurb:
      'Single-ingredient spirulina, harvested fresh and dried at low heat so the pigment and protein survive. One teaspoon into a smoothie, dal or juice.',
    grad: 'radial-gradient(120% 100% at 25% 20%,#cfe9d5,#5f9a76 55%,#1f5a41)',
    variants: [
      { label: '100 g', sub: '20 servings', mrp: 250, price: 200 },
      { label: '250 g', sub: '50 servings', mrp: 560, price: 450 },
    ],
  },
  {
    id: 'tablets',
    form: 'Tablets',
    name: 'Spirulina Tablets 500 mg',
    stat: 'No binders, no fillers',
    stat2: 'Pressed pure',
    blurb:
      'Pure spirulina pressed into 500 mg tablets — nothing added. For travel days, office desks and anyone who would rather not taste their greens.',
    grad: 'radial-gradient(120% 100% at 75% 20%,#dff0d6,#7fae5c 55%,#33612c)',
    variants: [
      { label: '60 tabs', sub: '15 days', mrp: 250, price: 200 },
      { label: '120 tabs', sub: '30 days', mrp: 470, price: 380 },
      { label: '300 tabs', sub: '75 days', mrp: 1100, price: 850 },
    ],
  },
  {
    id: 'refill',
    form: 'Powder',
    name: 'Spirulina Refill Pouch',
    stat: 'Low-waste packaging',
    stat2: 'Same powder, less plastic',
    blurb:
      'The same farm powder in a flat compostable pouch — refill your jar and keep the glass.',
    grad: 'radial-gradient(120% 100% at 30% 75%,#c9e4dd,#4c8a75 55%,#1c4a3d)',
    variants: [
      { label: '250 g', sub: '50 servings', mrp: 520, price: 420 },
      { label: '500 g', sub: '100 servings', mrp: 980, price: 760 },
    ],
  },
  {
    id: 'bundle',
    form: 'Bundle',
    name: 'Daily Wellness Duo',
    stat: 'Powder 100 g + 60 tablets',
    stat2: 'Best value',
    blurb:
      'Powder for mornings at home, tablets for everything else. Our most-repeated order.',
    grad: 'radial-gradient(120% 100% at 65% 30%,#e8f2e5,#8ab98f 55%,#2b6349)',
    variants: [
      { label: 'Starter', sub: '100 g + 60 tabs', mrp: 500, price: 380 },
      { label: 'Family', sub: '250 g + 120 tabs', mrp: 1030, price: 790 },
    ],
  },
];

export const FILTERS: readonly { k: Filter; label: string }[] = [
  { k: 'all', label: 'All' },
  { k: 'Powder', label: 'Powder' },
  { k: 'Tablets', label: 'Tablets' },
  { k: 'Bundle', label: 'Bundles' },
];

export const TRUST: readonly { big: string; body: string }[] = [
  { big: '60%+', body: 'Complete plant protein by weight, with all nine essential amino acids.' },
  { big: 'FSSAI', body: 'Licensed facility; every batch third-party tested for heavy metals.' },
  { big: '0', body: 'Binders, fillers, colours or preservatives — one ingredient only.' },
  { big: '48 hrs', body: 'From harvest to sealed pack, dried below 60 °C to protect phycocyanin.' },
];

export const PDP_BADGES: readonly { t: string; s: string }[] = [
  { t: 'Lab tested', s: 'Heavy metals & microbes' },
  { t: 'FSSAI licensed', s: 'Lic. 100xxxxxxxxxxx' },
  { t: 'No binders', s: '100% spirulina' },
  { t: 'Free delivery', s: 'On orders over ₹499' },
];

export const NUTRITION: readonly { k: string; v: string }[] = [
  { k: 'Protein', v: '3.1 g' },
  { k: 'Iron', v: '4.2 mg' },
  { k: 'Phycocyanin', v: '750 mg' },
  { k: 'Beta-carotene', v: '1.2 mg' },
  { k: 'Energy', v: '19 kcal' },
];

export const PAY_OPTIONS: readonly { k: PayMethod; label: string; sub: string }[] = [
  { k: 'upi', label: 'UPI', sub: 'GPay, PhonePe, Paytm' },
  { k: 'card', label: 'Card', sub: 'Credit or debit' },
  { k: 'cod', label: 'Cash on delivery', sub: '₹20 handling fee waived' },
];

export const PAY_LABELS: Record<PayMethod, string> = {
  upi: 'UPI',
  card: 'Card',
  cod: 'Cash on delivery',
};

export const FREE_SHIP_OVER = 499;
export const SHIP_FEE = 49;
