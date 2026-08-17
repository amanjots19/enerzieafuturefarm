'use client';

import Link from 'next/link';
import { useState } from 'react';

import { HOME_FAQS } from '@/lib/content/home';

/**
 * The homepage FAQ accordion.
 *
 * One open at a time, matching the design. Answers are only in the DOM when
 * open, which is fine here because the full set of eleven questions lives at
 * /faq as plain expanded text — that page is the one search engines should
 * read, and this section links to it.
 *
 * Controls are real <button>s with aria-expanded and aria-controls, so the
 * whole thing works from the keyboard.
 */
export function HomeFaqList() {
  const [open, setOpen] = useState<number | null>(0);

  return (
    <div className="home-faq">
      {HOME_FAQS.map((f, i) => {
        const isOpen = open === i;
        const panelId = `home-faq-panel-${i}`;
        const buttonId = `home-faq-button-${i}`;
        return (
          <div className="home-faq-item" key={f.q}>
            <button
              className="home-faq-q"
              id={buttonId}
              type="button"
              aria-expanded={isOpen}
              aria-controls={panelId}
              onClick={() => setOpen(isOpen ? null : i)}
            >
              <span>{f.q}</span>
              <span
                className={`home-faq-icon${isOpen ? ' home-faq-icon--open' : ''}`}
                aria-hidden="true"
              >
                ＋
              </span>
            </button>
            {isOpen && (
              <p className="home-faq-a" id={panelId} role="region" aria-labelledby={buttonId}>
                {f.a}
              </p>
            )}
          </div>
        );
      })}
      <p className="home-faq-more">
        <Link href="/faq">All questions →</Link>
      </p>
    </div>
  );
}
