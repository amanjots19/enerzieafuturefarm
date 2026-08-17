import Link from 'next/link';

import { BrandLockup } from '@/components/BrandLockup';

import {
  BRAND_BLURB,
  SOCIAL_LINKS,
  SUPPORT_EMAIL,
  SUPPORT_HOURS,
  SUPPORT_PHONE_DISPLAY,
  SUPPORT_PHONE_E164,
  SUPPORT_WHATSAPP,
} from '@/lib/content/social';

function WaIcon() {
  return (
    <svg
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.25"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ flexShrink: 0 }}
    >
      <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
    </svg>
  );
}

function PhoneIcon() {
  return (
    <svg
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.25"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ flexShrink: 0 }}
    >
      <path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07A19.5 19.5 0 0 1 4.4 10.8 19.79 19.79 0 0 1 1.32 2.18 2 2 0 0 1 3.3 0h3a2 2 0 0 1 2 1.72c.127.96.361 1.903.7 2.81a2 2 0 0 1-.45 2.11L7.09 7.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.907.339 1.85.573 2.81.7A2 2 0 0 1 22 16.92z" />
    </svg>
  );
}

function MailIcon() {
  return (
    <svg
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.25"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ flexShrink: 0 }}
    >
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <path d="m2 7 10 7 10-7" />
    </svg>
  );
}

export function Footer() {
  return (
    <footer className="footer-outer" aria-label="Site footer">
      <div className="footer-panel">
        <div className="footer-inner">

          <div className="footer-cols">

            {/* Column 1 — Brand */}
            <div className="footer-col">
              <div className="footer-brand-row">
                <BrandLockup tone="dark" />
              </div>
              <p className="footer-blurb">{BRAND_BLURB}</p>
            </div>

            {/* Column 2 — Talk to us */}
            <div className="footer-col">
              <p className="footer-col-head">Talk to us</p>
              <div className="footer-contact-list">
                <a
                  href={`https://wa.me/${SUPPORT_WHATSAPP}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="footer-contact-link"
                  aria-label="WhatsApp us"
                >
                  <WaIcon />
                  <span>WhatsApp us</span>
                </a>
                <a
                  href={`tel:${SUPPORT_PHONE_E164}`}
                  className="footer-contact-link"
                  aria-label={`Call us on ${SUPPORT_PHONE_DISPLAY}`}
                >
                  <PhoneIcon />
                  <span>{SUPPORT_PHONE_DISPLAY}</span>
                </a>
                <a
                  href={`mailto:${SUPPORT_EMAIL}`}
                  className="footer-contact-link footer-contact-email"
                  aria-label={`Email us at ${SUPPORT_EMAIL}`}
                >
                  <MailIcon />
                  <span>{SUPPORT_EMAIL}</span>
                </a>
                <p className="footer-hours">{SUPPORT_HOURS}</p>
              </div>
            </div>

            {/* Column 3 — Shop */}
            <div className="footer-col">
              <p className="footer-col-head">Shop</p>
              <nav className="footer-shop-links" aria-label="Shop navigation">
                <Link href="/shop" className="footer-shop-link">All products</Link>
                <Link href="/shop" className="footer-shop-link">Powder</Link>
                <Link href="/shop" className="footer-shop-link">Tablets</Link>
                <Link href="/terms#8-shipping" className="footer-shop-link">
                  Shipping &amp; returns
                </Link>
                <Link href="/faq" className="footer-shop-link">FAQ</Link>
              </nav>
            </div>

            {/* Column 4 — Follow the farm */}
            <div className="footer-col">
              <p className="footer-col-head">Follow the farm</p>
              <div className="footer-chips">
                {SOCIAL_LINKS.map(({ name, href }) => (
                  <a
                    key={name}
                    href={href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="footer-chip"
                    aria-label={`Enerzeia on ${name}`}
                  >
                    {name}
                  </a>
                ))}
              </div>
            </div>

          </div>

          <hr className="footer-divider" />

          <div className="footer-bottom">
            <div className="footer-bottom-left">
              <span className="footer-copy">© 2026 Enerzeia Future Farm</span>
              <Link href="/privacy" className="footer-legal-link">Privacy</Link>
              <Link href="/terms" className="footer-legal-link">Terms</Link>
            </div>
            <div className="footer-bottom-right">
              <span className="footer-badge">FSSAI licensed</span>
              <span className="footer-badge">Lab tested</span>
            </div>
          </div>

        </div>
      </div>
    </footer>
  );
}
