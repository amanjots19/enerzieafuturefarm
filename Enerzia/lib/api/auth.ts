import { request } from './http';
import type { MeData, OtpRequestData, OtpVerifyData, SessionData } from './types';

/**
 * Starts sign-in by requesting an OTP for `phone` (10 digits, no +91).
 * Returns timing metadata and, outside production, the dev code.
 */
export async function requestOtp(phone: string): Promise<OtpRequestData> {
  return request<OtpRequestData>('POST', '/auth/otp/request', { phone });
}

/**
 * Completes sign-in by verifying the OTP. Returns a bearer token and the
 * user. Throws ApiError with UNAUTHORIZED when the code is wrong or expired —
 * this is NOT the global sign-out signal (the request carries no token).
 */
export async function verifyOtp(phone: string, code: string): Promise<OtpVerifyData> {
  return request<OtpVerifyData>('POST', '/auth/otp/verify', { phone, code });
}

/**
 * Exchanges an MSG91 widget access token for our session JWT.
 * Calls POST /auth/session — the server verifies the token with MSG91,
 * strips the country code, and upserts the user.
 */
export async function exchangeSession(accessToken: string): Promise<SessionData> {
  return request<SessionData>('POST', '/auth/session', { accessToken });
}

/** Returns the signed-in shopper's profile. Requires auth. */
export async function getMe(): Promise<MeData> {
  return request<MeData>('GET', '/auth/me');
}
