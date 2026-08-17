import {
  SUPPORT_EMAIL,
  SUPPORT_PHONE_DISPLAY,
  SUPPORT_PHONE_E164,
  SUPPORT_WHATSAPP,
} from '@/lib/content/social';

type ChannelKey = 'wa' | 'tel' | 'mail';

function ChIcon({ ch, size }: { ch: ChannelKey; size: number }) {
  if (ch === 'wa') {
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
      </svg>
    );
  }
  if (ch === 'tel') {
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07A19.5 19.5 0 0 1 4.4 10.8 19.79 19.79 0 0 1 1.32 2.18 2 2 0 0 1 3.3 0h3a2 2 0 0 1 2 1.72c.127.96.361 1.903.7 2.81a2 2 0 0 1-.45 2.11L7.09 7.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.907.339 1.85.573 2.81.7A2 2 0 0 1 22 16.92z" />
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
    value: SUPPORT_PHONE_DISPLAY,
    href: `https://wa.me/${SUPPORT_WHATSAPP}`,
    newTab: true,
    ariaLabel: `WhatsApp us on ${SUPPORT_PHONE_DISPLAY}`,
  },
  {
    key: 'tel',
    label: 'Call',
    value: SUPPORT_PHONE_DISPLAY,
    href: `tel:${SUPPORT_PHONE_E164}`,
    ariaLabel: `Call us on ${SUPPORT_PHONE_DISPLAY}`,
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
 * Renders the three contact channel links (WhatsApp, Call, Email) as the
 * full .contact-card treatment — icon circles, label, value. Used on /contact.
 *
 * There was a second `variant="footer"` here. The footer no longer uses it:
 * its "Talk to us" column labels the WhatsApp row "WhatsApp us" rather than
 * repeating the phone number, which is what stops the two phone-bearing rows
 * reading as a duplication bug. The old variant's `.footer-channel` styles are
 * gone, so the branch was removed rather than left to render unstyled.
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
