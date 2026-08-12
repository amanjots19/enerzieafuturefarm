import { request } from './http';
import type { AddressBody, AddressData, AddressListData } from './types';

/** Lists all saved addresses, default first. Requires auth. */
export async function listAddresses(): Promise<AddressListData> {
  return request<AddressListData>('GET', '/me/addresses');
}

/**
 * Saves a new delivery address. The first address saved becomes the default
 * automatically. Pass `isDefault: true` to promote a later one. Requires auth.
 */
export async function addAddress(addr: AddressBody): Promise<AddressData> {
  return request<AddressData>('POST', '/me/addresses', addr);
}

/** Replaces an address wholesale. Requires auth. */
export async function updateAddress(id: string, addr: AddressBody): Promise<AddressData> {
  return request<AddressData>('PUT', `/me/addresses/${encodeURIComponent(id)}`, addr);
}

/** Deletes an address and returns the remaining list. Requires auth. */
export async function removeAddress(id: string): Promise<AddressListData> {
  return request<AddressListData>('DELETE', `/me/addresses/${encodeURIComponent(id)}`);
}
