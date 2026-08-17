/**
 * Shared shapes for the static content pages (Terms, Privacy, FAQ) and the
 * marketing content used in the shop and footer.
 *
 * Content is structured data rather than raw HTML strings so the pages stay
 * typed, cannot inject markup, and can be restyled without touching the words.
 * Legal text is edited HERE and nowhere else — no component may paraphrase it.
 */

/** One paragraph, or one bulleted list, inside a section. */
export type Block =
  | { kind: 'p'; text: string }
  | { kind: 'ul'; items: string[] };

/** A numbered clause of a legal document. */
export interface Section {
  /** Rendered heading, e.g. "9. Returns & Refunds". Also the anchor source. */
  heading: string;
  blocks: Block[];
}

/** A long-form legal document: Terms & Conditions, Privacy Policy. */
export interface LegalDoc {
  title: string;
  /** Human-readable revision date, e.g. "15 February 2026". */
  lastUpdated: string;
  /** Preamble shown above the numbered clauses. */
  intro: Block[];
  sections: Section[];
}

/** One question and its answer. An answer may run to several paragraphs. */
export interface FaqItem {
  question: string;
  answer: Block[];
}

/**
 * A customer testimonial. These are real customer feedback with names
 * withheld at the customer's discretion (confirmed by the business owner,
 * 2026-08-13) — do NOT add invented entries to this list, and do not present
 * marketing copy as customer testimony.
 */
export interface Testimonial {
  /** Out of 5. */
  rating: number;
  headline: string;
  body: string;
  attribution: string;
}

/** An official brand profile on an external platform. */
export interface SocialLink {
  name: string;
  href: string;
}

/**
 * A photograph rendered by `components/Photo.tsx`.
 *
 * `gradient` is not decoration: photographs are owner-supplied and a file can
 * be absent or mistyped, so every frame must degrade to something composed
 * rather than to a broken-image glyph. The gradient paints the frame and the
 * image sits over it.
 *
 * `alt` is never empty. It describes the specific photograph it was written
 * for, so swapping the file without swapping these words makes the page lie to
 * anyone using a screen reader.
 */
export interface Photo {
  /** Path under `public/`. */
  src: string;
  alt: string;
  /** Stand-in while the file is absent, and fallback if it fails to load. */
  gradient: string;
}
