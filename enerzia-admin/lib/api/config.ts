/**
 * Base URL for every API request. The NEXT_PUBLIC_ prefix is mandatory —
 * anything else is stripped from the client bundle and arrives undefined.
 */
export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? 'http://localhost:8080/api/v1';
