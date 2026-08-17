'use client';

import { useState } from 'react';

import { ANNOUNCEMENT } from '@/lib/content/home';

/**
 * The dismissible strip above the header.
 *
 * Carries the free-shipping threshold only. The design also had "COD
 * available" (we have no cash on delivery — see lib/content/home.ts) and
 * "Ships in 24 h" (a dispatch-speed promise the owner chose not to make, and
 * one the Terms deliberately avoid guaranteeing).
 *
 * Dismissal is per page load, not persisted — a returning visitor sees it
 * again. Persisting it would need storage consent handling for something that
 * carries no personal data, which is not worth it.
 */
export function AnnouncementBar() {
  const [shown, setShown] = useState(true);
  if (!shown) return null;

  return (
    <div className="announce">
      <div className="announce-inner">
        <span className="announce-msg">{ANNOUNCEMENT}</span>
        <button
          className="announce-close"
          type="button"
          aria-label="Dismiss announcement"
          onClick={() => setShown(false)}
        >
          ✕
        </button>
      </div>
    </div>
  );
}
