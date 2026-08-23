import { SUPPORT_EMAIL, SUPPORT_WHATSAPP } from '@/lib/content/social';

type ChannelKey = 'wa' | 'mail';

function ChIcon({ ch, size }: { ch: ChannelKey; size: number }) {
  if (ch === 'wa') {
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
      </svg>
    );
  }
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <path d="m2 7 10 7 10-7" />
    </svg>
  );
}

const CHANNELS: {
  key: ChannelKey;
  label: string;
  value: string;
  href: string;
  newTab?: true;
  ariaLabel: string;
}[] = [
  {
    key: 'wa',
    label: 'WhatsApp',
    /**
     * Not the phone number. The link opens a chat, and the number is not
     * published anywhere on the site — see the note in lib/content/social.ts.
     */
    value: 'Start a chat',
    href: `https://wa.me/${SUPPORT_WHATSAPP}`,
    newTab: true,
    ariaLabel: 'Message us on WhatsApp',
  },
  {
    key: 'mail',
    label: 'Email',
    value: SUPPORT_EMAIL,
    href: `mailto:${SUPPORT_EMAIL}`,
    ariaLabel: `Email us at ${SUPPORT_EMAIL}`,
  },
];

/**
 * Renders the two contact channel links (WhatsApp, Email) as the full
 * .contact-card treatment — icon circles, label, value. Used on /contact.
 *
 * The Call row is gone (owner's decision, 2026-08-23) — WhatsApp is the one
 * channel they staff, and it opens a chat rather than a call.
 *
 * There was a second `variant="footer"` here. The footer no longer uses it —
 * its "Talk to us" column has its own markup, and the old variant's
 * `.footer-channel` styles are gone, so the branch was removed rather than
 * left to render unstyled.
 */
export function ContactChannels() {
  return (
    <div className="contact-card">
      {CHANNELS.map((ch) => (
        <a
          key={ch.key}
          href={ch.href}
          {...(ch.newTab ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
          className="contact-row"
          aria-label={ch.ariaLabel}
        >
          <span className="contact-icon" aria-hidden="true">
            <ChIcon ch={ch.key} size={18} />
          </span>
          <span className="contact-text">
            <span className="contact-label">{ch.label}</span>
            <span className="contact-value">{ch.value}</span>
          </span>
        </a>
      ))}
    </div>
  );
}
