import type { SocialLink } from './types';

/**
 * Contact details and official brand profiles.
 *
 * SUPPORT_EMAIL, SUPPORT_PHONE and SUPPORT_WHATSAPP are the SINGLE source for
 * these values. The contact page, the footer, the Terms and the Privacy Policy
 * must all read from here — the phone number and address must never be typed
 * out a second time somewhere else, because the second copy is the one that
 * goes stale.
 *
 * The owner confirmed on 2026-08-13 that support@enerzeiafuturefarm.com is a
 * mailbox they own and monitor, and that the number receives WhatsApp. Both
 * legal documents originally carried malformed Gmail addresses
 * ("gamil.com", and a doubled ".com"); those were corrected to this address on
 * the owner's instruction.
 */
export const SUPPORT_EMAIL = 'support@enerzeiafuturefarm.com';

/** Display form. The tel: and wa.me links use SUPPORT_PHONE_E164. */
export const SUPPORT_PHONE_DISPLAY = '+91 88263 45094';
export const SUPPORT_PHONE_E164 = '+918826345094';
/** wa.me wants the number without the leading '+'. */
export const SUPPORT_WHATSAPP = '918826345094';

export const POSTAL_ADDRESS = 'Village Saimpal, Kalanaur, Rohtak, Haryana – 124113, India';

/**
 * Support hours, confirmed by the owner on 2026-08-13.
 *
 * This REVERSES the earlier standing decision to omit hours entirely — that
 * held only while no window had been supplied, on the grounds that a wrong
 * "we're open now" is worse than saying nothing. The owner has now confirmed
 * this is their actual staffed window, so it is published.
 *
 * If the window ever stops being staffed, delete this constant rather than
 * leaving it stale: an unstaffed published window generates complaints, which
 * is exactly the failure the original decision was avoiding.
 */
export const SUPPORT_HOURS = 'Mon–Sat, 9 am – 6 pm IST';

/** One-line brand descriptor used in the footer's brand column. */
export const BRAND_BLURB =
  'Spirulina grown, dried and packed on our own farm. FSSAI licensed, lab tested every batch.';

/**
 * Official profiles. The URLs supplied by the owner carried QR-scan referral
 * parameters (`igsh`, `igshid`, `utm_source=qr`) which identify the scan that
 * produced the link, not the profile — they are stripped here so the footer
 * links to the canonical profile.
 *
 * The Facebook entry keeps its `profile.php?id=` form because that page has no
 * vanity URL; do not "tidy" it into facebook.com/<name>, which is a different
 * page that is not ours.
 */
export const SOCIAL_LINKS: SocialLink[] = [
  { name: 'Instagram', href: 'https://www.instagram.com/enerzeiafuturefarm' },
  { name: 'X', href: 'https://x.com/enerzeia' },
  { name: 'Facebook', href: 'https://www.facebook.com/profile.php?id=61592481037327' },
  { name: 'YouTube', href: 'https://www.youtube.com/@enerzeiafuturefarm' },
  { name: 'Threads', href: 'https://www.threads.com/@enerzeiafuturefarm' },
];
