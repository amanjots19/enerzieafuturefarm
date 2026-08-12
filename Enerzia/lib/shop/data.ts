import type { Filter } from './types';

export const FILTERS: readonly { k: Filter; label: string }[] = [
  { k: 'all', label: 'All' },
  { k: 'Powder', label: 'Powder' },
  { k: 'Tablets', label: 'Tablets' },
  { k: 'Bundle', label: 'Bundles' },
];
