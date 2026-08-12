import { request } from './http';
import type { ProductDetailData, ProductListData, TrustData } from './types';

/**
 * Fetches the product grid. Pass a form string (`"Powder"`, `"Tablets"`,
 * `"Bundle"`) to filter; omit for all nine products.
 */
export async function getProducts(form?: string): Promise<ProductListData> {
  const qs = form ? `?form=${encodeURIComponent(form)}` : '';
  return request<ProductListData>('GET', `/products${qs}`);
}

/** Fetches one product with its detail content, siblings, badges and nutrition. */
export async function getProduct(id: string): Promise<ProductDetailData> {
  return request<ProductDetailData>('GET', `/products/${encodeURIComponent(id)}`);
}

/** Fetches the four trust-strip tiles shown under the shop grid. */
export async function getTrust(signal?: AbortSignal): Promise<TrustData> {
  return request<TrustData>('GET', '/content/trust', undefined, signal);
}
