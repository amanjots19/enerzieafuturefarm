'use client';

import { useState } from 'react';

import { BENEFIT_FAQS } from '@/lib/content/benefits';

/**
 * The Benefits page FAQ accordion.
 *
 * Deliberately the same component shape and the same `.home-faq*` classes as
 * `HomeFaqList` — one open at a time, real <button>s with aria-expanded and
 * aria-controls, the answer only in the DOM while open. It is a separate file
 * only because it reads a different question set; if a third page ever needs
 * one, fold all three into a single component taking the list as a prop rather
 * than adding a third copy.
 *
 * Unlike the homepage's, this list does NOT link on to /faq. These five are
 * benefit-shaped questions and two of them exist to say no; /faq answers the
 * ordering, shipping and returns questions, which is a different errand.
 */
export function BenefitsFaqList() {
  const [open, setOpen] = useState<number | null>(0);

  return (
    <div className="home-faq">
      {BENEFIT_FAQS.map((f, i) => {
        const isOpen = open === i;
        const panelId = `benefits-faq-panel-${i}`;
        const buttonId = `benefits-faq-button-${i}`;
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
    </div>
  );
}
