import { request } from './http';
import type { LoginData, MeData } from './types';

/**
 * Exchanges an email and password for an admin token.
 * Throws ApiError with UNAUTHORIZED for wrong credentials or unconfigured admin.
 * Throws ApiError with RATE_LIMITED after 10 failed attempts from the same IP.
 * Throws ApiError with VALIDATION_ERROR if email or password is missing.
 */
export async function login(email: string, password: string): Promise<LoginData> {
  return request<LoginData>('POST', '/admin/login', { email, password });
}

/**
 * Returns the signed-in administrator's email. Requires auth.
 * Use on load to verify a stored token is still valid before rendering
 * protected pages — an expired token must land on sign-in, not trigger a
 * cascade of 401s from every subsequent request.
 */
export async function me(): Promise<MeData> {
  return request<MeData>('GET', '/admin/me');
}
