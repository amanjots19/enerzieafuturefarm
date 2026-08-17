import Image from 'next/image';

/**
 * The brand lockup: the swirl mark beside the ENERZEIA / FUTURE FARM wordmark.
 *
 * `/assets/enerzeia-swirl.png` was extracted from the full brand logo
 * (`enerzeia-logo.png`, 1136x730) rather than supplied separately: the source
 * is a flattened white-background raster of the whole lockup — swirl, wordmark
 * and enclosing oval — so the mark had to be cut out of it.
 *
 * It is a CONNECTED-COMPONENT extraction, not a rectangular crop. Flood-filling
 * from a seed inside the swirl (105,380) yields 42,261 px with bounds
 * x[105,416] y[168,556], disjoint from both the oval and the wordmark. A
 * rectangle cannot do this: the swirl's leftmost point (x=105) sits left of the
 * oval's rightmost point in the same band (x=100 at y=200), so any vertical cut
 * that clears the ring also slices the mark — which is exactly what a first
 * attempt at x=118 did. White was knocked out to alpha with the colour
 * un-premultiplied, so antialiased edges keep the gradient instead of fringing.
 *
 * If a proper vector of the mark ever arrives, swap the src — nothing else
 * here needs to change. The regeneration recipe is recorded above so the crop
 * is reproducible rather than a magic asset nobody can rebuild.
 *
 * CHANGE THE FILENAME when you regenerate this image. next/image caches the
 * optimised output by URL, and the browser caches it again on top; rewriting
 * the file in place leaves both serving the previous version, which looks
 * exactly like the new crop being wrong. That cost a full debugging round.
 */
export function BrandLockup({ tone = 'light' }: { tone?: 'light' | 'dark' }) {
  return (
    <>
      <span className="brand-mark">
        <Image
          src="/assets/enerzeia-brandmark.png"
          alt=""
          width={318}
          height={395}
          priority
        />
      </span>
      <span className={`brand-text${tone === 'dark' ? ' brand-text--dark' : ''}`}>
        <span className="brand-name">Enerzeia</span>
        <span className="brand-sub">Future Farm</span>
      </span>
    </>
  );
}
