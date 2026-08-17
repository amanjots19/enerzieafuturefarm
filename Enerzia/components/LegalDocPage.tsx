import Link from 'next/link';

import type { Block, LegalDoc } from '@/lib/content/types';

function toId(heading: string): string {
  return heading.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

function Blocks({ blocks }: { blocks: Block[] }) {
  return (
    <>
      {blocks.map((block, i) => {
        if (block.kind === 'p') return <p key={i}>{block.text}</p>;
        return (
          <ul key={i}>
            {block.items.map((item, j) => (
              <li key={j}>{item}</li>
            ))}
          </ul>
        );
      })}
    </>
  );
}

export function LegalDocPage({ doc }: { doc: LegalDoc }) {
  return (
    <div className="legal-page">
      <div className="legal-wrap">
        <Link href="/shop" className="legal-back">
          ← Back to shop
        </Link>

        <h1 className="legal-title">{doc.title}</h1>
        <p className="legal-updated">Last Updated: {doc.lastUpdated}</p>

        <div className="legal-intro">
          <Blocks blocks={doc.intro} />
        </div>

        {doc.sections.map((section) => (
          <section key={section.heading} id={toId(section.heading)} className="legal-section">
            <h2>{section.heading}</h2>
            <Blocks blocks={section.blocks} />
          </section>
        ))}

        <p className="legal-back-bottom">
          <Link href="/shop">← Back to shop</Link>
        </p>
      </div>
    </div>
  );
}
