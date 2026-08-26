import type { Metadata } from 'next';
import Link from 'next/link';

import { JsonLd } from '@/components/JsonLd';
import { FAQS } from '@/lib/content/faq';
import type { Block } from '@/lib/content/types';
import { absoluteUrl } from '@/lib/content/site';
import { pageMetadata } from '@/lib/seo';

export const metadata: Metadata = pageMetadata({
  path: '/faq',
  title: 'FAQ | Enerzeia Future Farm',
  description: 'Frequently asked questions about Enerzeia Future Farm spirulina: how much to take, taste, shipping, returns and lab testing.',
});

function renderBlock(block: Block, i: number) {
  if (block.kind === 'p') return <p key={i}>{block.text}</p>;
  return (
    <ul key={i}>
      {block.items.map((item, j) => (
        <li key={j}>{item}</li>
      ))}
    </ul>
  );
}

/**
 * `FAQPage` structured data, from the same FAQS the page renders.
 *
 * Built from the rendered source rather than retyped, which is what keeps it
 * eligible: Google requires the marked-up answer to be the visible answer, and
 * a second hand-maintained copy is guaranteed to drift out of that agreement.
 *
 * `acceptedAnswer.text` allows limited HTML; the answers here are structured
 * Blocks (paragraphs and lists), so each is flattened to plain text. A list
 * item is joined with a full stop and a space so two bullets do not run
 * together into one sentence.
 */
function faqSchema() {
  return {
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    '@id': `${absoluteUrl('/faq')}#faq`,
    mainEntity: FAQS.map((item) => ({
      '@type': 'Question',
      name: item.question,
      acceptedAnswer: {
        '@type': 'Answer',
        text: item.answer
          .map((b) => (b.kind === 'p' ? b.text : b.items.join('. ')))
          .join(' '),
      },
    })),
  };
}

export default function FaqPage() {
  return (
    <div className="legal-page">
      <JsonLd data={faqSchema()} />
      <div className="legal-wrap">
        <Link href="/shop" className="legal-back">
          ← Back to shop
        </Link>

        <h1 className="legal-title">Frequently Asked Questions</h1>

        <div className="faq-list">
          {FAQS.map((item, i) => (
            <div key={i} className="faq-item">
              <h2 className="faq-question">{item.question}</h2>
              <div className="faq-answer">
                {item.answer.map((block, j) => renderBlock(block, j))}
              </div>
            </div>
          ))}
        </div>

        <p className="legal-back-bottom">
          <Link href="/shop">← Back to shop</Link>
        </p>
      </div>
    </div>
  );
}
