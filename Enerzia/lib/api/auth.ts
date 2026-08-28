import { request } from './http';
import type { MeData, OtpRequestData, OtpVerifyData, SessionData } from './types';

/**
 * Starts sign-in by requesting an OTP for `phone`.
 *
 * DORMANT. Sign-in goes through the MSG91 widget and `exchangeSession` below;
 * these two endpoints are the pre-2026-08-09 in-house flow, still routed on the
 * server but not reachable from any screen (enerzia-be/tasks.md 8.2 is parked
 * until admin auth lands). Nothing here was updated for international numbers.
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
 *
 * Calls POST /auth/session — the server verifies the token with MSG91 and
 * upserts the user. The returned `phone` carries its country code and no '+'
 * (`919876543210`); render it through `lib/shop/phone.ts` rather than by
 * prefixing one.
 */
export async function exchangeSession(accessToken: string): Promise<SessionData> {
  return request<SessionData>('POST', '/auth/session', { accessToken });
}

/** Returns the signed-in shopper's profile. Requires auth. */
export async function getMe(): Promise<MeData> {
  return request<MeData>('GET', '/auth/me');
}
