import { request } from './http';
import type { AdminProduct, AdminProductListData, UploadSignatureData } from './types';

/** All products including retired, ordered by position. */
export async function listProducts(): Promise<AdminProduct[]> {
  const data = await request<AdminProductListData>('GET', '/admin/products');
  return data.products;
}

export async function getProduct(id: string): Promise<AdminProduct> {
  return request<AdminProduct>('GET', `/admin/products/${encodeURIComponent(id)}`);
}

/**
 * Creates a product. body must NOT contain discountPercent or soldOut —
 * they are computed outputs and the server answers 422 if either is present.
 */
export async function createProduct(body: unknown): Promise<AdminProduct> {
  return request<AdminProduct>('POST', '/admin/products', body);
}

/**
 * Replaces every editable field. body must NOT contain discountPercent or
 * soldOut. Omit id from the body or send the path id unchanged — a mismatch
 * is a 422 by design (renaming would orphan cart and order lines).
 */
export async function updateProduct(id: string, body: unknown): Promise<AdminProduct> {
  return request<AdminProduct>('PUT', `/admin/products/${encodeURIComponent(id)}`, body);
}

/** Sets active: false. Never deletes — past orders still reference the product. */
export async function retireProduct(id: string): Promise<AdminProduct> {
  return request<AdminProduct>('DELETE', `/admin/products/${encodeURIComponent(id)}`);
}

/** Returns the parameters the browser needs to upload directly to Cloudinary. */
export async function getUploadSignature(): Promise<UploadSignatureData> {
  return request<UploadSignatureData>('POST', '/admin/uploads/signature');
}
