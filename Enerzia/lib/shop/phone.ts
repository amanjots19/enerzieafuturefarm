/**
 * Rendering an account's phone number.
 *
 * `user.phone` is whatever the server has stored, and since 2026-08-24 that is
 * an E.164 number **with its country code and no `+`** — `919876543210`,
 * `12025551234`. See `enerzia-be/schema.md` §users.
 *
 * ---------------------------------------------------------------------------
 * WHY THIS IS NOT JUST `'+' + phone`
 *
 * Accounts are migrated to the new format lazily, when their owner next signs
 * in. A shopper holding a session token issued before the change still reads
 * back the old ten-digit form from `GET /auth/me`, because nothing has
 * rewritten their document yet — tokens last 30 days, so this lasts a month
 * past the deploy.
 *
 * Prefixing a bare `+` onto a legacy number produces `+9876543210`, which is
 * not a number anywhere. It would be shown on the checkout summary and, worse,
 * handed to Razorpay as the payer's contact.
 *
 * Ten digits can only be a legacy Indian number: every stored value of that
 * length predates the change, and none could have been written since. So the
 * rule is exact rather than a guess.
 *
 * **Delete both functions once `enerzia-be/tasks.md` 14.2 has swept the
 * remaining ten-digit rows** — at that point every stored number carries its
 * own country code and the special case is dead weight.
 * ---------------------------------------------------------------------------
 */

/** Digits in an Indian subscriber number, without the country code. */
const LEGACY_LENGTH = 10;
const INDIA = '91';

/** True for a number stored before country codes were kept. */
function isLegacy(phone: string): boolean {
  return /^\d{10}$/.test(phone) && phone.length === LEGACY_LENGTH;
}

/**
 * Returns the number in E.164 form without the leading `+`, upgrading a legacy
 * ten-digit value on the way. Use this wherever a machine reads the number —
 * Razorpay's `prefill.contact`, for one.
 */
export function accountPhoneE164(phone: string): string {
  if (!phone) return '';
  return isLegacy(phone) ? INDIA + phone : phone;
}

/**
 * Returns the number as a person should read it: `+91 9876543210`,
 * `+1 2025551234`. Returns '' for an absent number so callers can drop the
 * line rather than render a stray '+'.
 */
export function formatAccountPhone(phone: string): string {
  const e164 = accountPhoneE164(phone);
  if (!e164) return '';
  // The split point is the country code, which is 1-3 digits and not derivable
  // without a lookup table. Two is right for India (91), the UK (44) and the
  // UAE (971 — which this gets wrong by one digit). A misplaced space is a
  // cosmetic flaw in a number that is still correct and still dialable; a
  // lookup table for every calling code is not worth carrying for that.
  if (e164.startsWith(INDIA) && e164.length === INDIA.length + LEGACY_LENGTH) {
    return `+${INDIA} ${e164.slice(INDIA.length)}`;
  }
  return `+${e164}`;
}
