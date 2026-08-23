import { EMAIL, EMAIL_HREF, WHATSAPP_HREF } from '@/lib/shop/contact';
import type { ShopAction } from '@/lib/shop/reducer';
import type { ShopState } from '@/lib/shop/types';

export function ContactScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
}) {
  return (
    <div className="wrap-sm screen">
      <h1 className="page-title">Contact us</h1>
      <p className="contact-intro">
        Questions about an order, a product, or delivery — reach us however suits you.
      </p>

      <div className="contact-card">
        <a
          href={WHATSAPP_HREF}
          target="_blank"
          rel="noopener noreferrer"
          className="contact-row"
          aria-label="Message us on WhatsApp"
        >
          <span className="contact-icon" aria-hidden="true">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
            </svg>
          </span>
          <span className="contact-text">
            <span className="contact-label">WhatsApp</span>
            <span className="contact-value">Start a chat</span>
          </span>
        </a>

        <a
          href={EMAIL_HREF}
          className="contact-row"
          aria-label={`Email us at ${EMAIL}`}
        >
          <span className="contact-icon" aria-hidden="true">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round">
              <rect x="2" y="4" width="20" height="16" rx="2" />
              <path d="m2 7 10 7 10-7" />
            </svg>
          </span>
          <span className="contact-text">
            <span className="contact-label">Email</span>
            <span className="contact-value">{EMAIL}</span>
          </span>
        </a>
      </div>

      {state.user && (
        <p className="contact-order-hint">
          Have your order number handy —{' '}
          <button
            type="button"
            className="contact-order-link"
            onClick={() => dispatch({ type: 'go', screen: 'orders' })}
          >
            My orders
          </button>
        </p>
      )}
    </div>
  );
}
