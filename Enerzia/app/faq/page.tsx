import type { Metadata } from 'next';
import Link from 'next/link';

import { FAQS } from '@/lib/content/faq';
import type { Block } from '@/lib/content/types';

export const metadata: Metadata = {
  title: 'FAQ | Enerzeia Future Farm',
  description: 'Frequently asked questions about Enerzeia Future Farm Spirulina Tablets.',
};

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

export default function FaqPage() {
  return (
    <div className="legal-page">
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
