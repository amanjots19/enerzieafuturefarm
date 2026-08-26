import type { Metadata } from 'next';
import Link from 'next/link';

import { Photo } from '@/components/Photo';
import { SiteHeader, type NavItem } from '@/components/SiteHeader';
import { BenefitsFaqList } from '@/components/benefits/BenefitsFaqList';
import { pageMetadata } from '@/lib/seo';
import {
  BENEFITS_CTA,
  BENEFITS_HERO,
  COMPARE,
  COMPARE_CTA_LABEL,
  COMPARE_HEADING,
  DOSING,
  DOSING_INTRO,
  FAQ_HEADING,
  LIMITS,
  LIMITS_DISCLAIMER,
  LIMITS_HEADING,
  NUTRIENTS,
  NUTRITION_FOOTNOTE,
  NUTRITION_INTRO,
  REASONS,
  REASONS_INTRO,
  SUITS,
  SUITS_INTRO,
  TOPLINE,
} from '@/lib/content/benefits';

// This IS the benefits page, so the nav offers the three places it is not.
const BENEFITS_NAV: NavItem[] = [
  { label: 'About us', href: '/about' },
  { label: 'Shop', href: '/shop', pinned: true },
  { label: 'Our Farm', href: '/farm' },
];

export const metadata: Metadata = pageMetadata({
  path: '/benefits',
  title: 'Spirulina benefits - what it actually adds | Enerzeia Future Farm',
  description: 'What a 5 g serving of spirulina carries - protein, iron, phycocyanin, B-vitamins - how much to take, and what the research does and does not settle. No treatment, weight-loss or detox claims.',
  socialTitle: 'Spirulina benefits, honestly',
});

/** The right-pointing arrow used on both call-to-action buttons. */
function ArrowRight({ size = 17 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M5 12h14M13 6l6 6-6 6" />
    </svg>
  );
}

/**
 * "Benefits" — the page that says what spirulina adds, and refuses to say more.
 *
 * A server component, like `/` and `/farm`: all the copy is static and the only
 * interactive piece is the FAQ accordion, which is its own client island.
 *
 * The section order is the argument, and it is the design's:
 *
 *   hero → four toplines → the nutrition table → six reasons, each carrying its
 *   own evidence footer → who it actually suits → how much to take, beside what
 *   we refuse to claim → questions → shop.
 *
 * The refusals sit BESIDE the dosing panel rather than in a footnote at the
 * bottom, which is where a page like this normally hides them. Anyone reading
 * how much to take is reading the one thing we most want qualified.
 *
 * The header's nav, the announcement bar and the footer are the site's own —
 * the brand lockup in particular is untouched, so this page carries the same
 * logo as every other route.
 */
export default function BenefitsPage() {
  return (
    <>
      <SiteHeader nav={BENEFITS_NAV} />

      <main className="benefits" id="top">
        {/* ------------------------------------------------------------ hero */}
        <section className="benefits-hero-section">
          <div className="benefits-hero">
            <div>
              <p className="home-eyebrow-pill">
                <span className="home-eyebrow-dot" aria-hidden="true" />
                {BENEFITS_HERO.eyebrow}
              </p>
              <h1 className="home-h1">
                {BENEFITS_HERO.headingLines.map((line, i) => (
                  <span key={line}>
                    {line}
                    {i < BENEFITS_HERO.headingLines.length - 1 && <br />}
                  </span>
                ))}
              </h1>
              <p className="home-hero-body">{BENEFITS_HERO.body}</p>

              <div className="home-cta-row">
                <a href="#nutrition" className="home-cta">
                  {BENEFITS_HERO.ctaLabel}
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2.75"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M12 5v14M6 13l6 6 6-6" />
                  </svg>
                </a>
                {/* Links to the refusals, not away from them — the design puts
                    this next to the primary CTA on purpose. */}
                <a href="#limits" className="home-cta-quiet">
                  {BENEFITS_HERO.quietCtaLabel}
                </a>
              </div>
            </div>

            <Photo photo={BENEFITS_HERO.photo} frameClassName="benefits-hero-art" priority />
          </div>

          <div className="ben-topline">
            {TOPLINE.map((t) => (
              <div className="ben-topline-item" key={t.title}>
                <span className="ben-topline-big" aria-hidden="true">
                  {t.big}
                </span>
                <div>
                  <h2 className="ben-topline-title">{t.title}</h2>
                  <p className="ben-topline-body">{t.body}</p>
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* ------------------------------------------------------- nutrition */}
        <section id="nutrition" className="home-section">
          <div className="ben-split">
            <div>
              <p className="home-eyebrow">{NUTRITION_INTRO.eyebrow}</p>
              <h2 className="home-h2">{NUTRITION_INTRO.heading}</h2>
              <p className="home-section-body">{NUTRITION_INTRO.body}</p>
            </div>

            {/* A description list, not a <table>: this is seven name/value
                pairs, not tabular data with meaningful rows and columns, and a
                screen reader announces the pairing correctly this way. */}
            <div className="ben-nutrition">
              <dl className="ben-nutrition-list">
                {NUTRIENTS.map((n) => (
                  <div className="ben-nutrient" key={n.name}>
                    <dt className="ben-nutrient-name">
                      {n.name}
                      <span className="ben-nutrient-note">{n.note}</span>
                    </dt>
                    <dd className="ben-nutrient-value">{n.value}</dd>
                  </div>
                ))}
              </dl>
              <p className="ben-nutrition-note">{NUTRITION_FOOTNOTE}</p>
            </div>
          </div>
        </section>

        {/* --------------------------------------------------------- reasons */}
        <section id="reasons" className="home-section">
          <div className="home-section-intro">
            <p className="home-eyebrow">{REASONS_INTRO.eyebrow}</p>
            <h2 className="home-h2">{REASONS_INTRO.heading}</h2>
            <p className="home-section-body">{REASONS_INTRO.body}</p>
          </div>

          {/* Every card carries a photograph and ends on its evidence footer —
              `Reason.photo` and `Reason.evidence` are both required, so a
              benefit cannot be added here without a picture or without saying
              how far the research goes. See REASONS in the content file. */}
          <div className="ben-reasons">
            {REASONS.map((r) => (
              <article className="ben-reason" key={r.title}>
                <Photo photo={r.photo} frameClassName="ben-reason-art" />
                <div className="ben-reason-body-wrap">
                  <span className="ben-reason-tag">{r.tag}</span>
                  <h3 className="ben-reason-title">{r.title}</h3>
                  <p className="ben-reason-body">{r.body}</p>
                  <p className="ben-reason-evidence">
                    <span>Where the evidence stands:</span> {r.evidence}
                  </p>
                </div>
              </article>
            ))}
          </div>
        </section>

        {/* ----------------------------------------------------------- suits */}
        <section className="home-section">
          {/* One panel: a wide band photograph with the four points beneath it,
              as drawn. The band is the only landscape frame on the page, which
              is what stops the run of portrait-and-card sections above it from
              reading as one long column. */}
          <div className="ben-band-panel">
            <Photo photo={SUITS_INTRO.photo} frameClassName="ben-band" />
            <div className="ben-band-body">
              <h2 className="home-h2">{SUITS_INTRO.heading}</h2>
              <p className="home-section-body ben-suits-lede">{SUITS_INTRO.body}</p>
              <div className="ben-suits">
                {SUITS.map((s) => (
                  <div className="ben-suit" key={s.title}>
                    <span className="ben-suit-num" aria-hidden="true">
                      {s.n}
                    </span>
                    <h3 className="ben-suit-title">{s.title}</h3>
                    <p className="ben-suit-body">{s.body}</p>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </section>

        {/* ------------------------------------------- dosing + the refusals */}
        <section id="howmuch" className="home-section">
          <div className="ben-two">
            {/* The dark panel, matching /farm's byte and the footer. */}
            <div className="ben-dosing">
              <p className="ben-dosing-eyebrow">{DOSING_INTRO.eyebrow}</p>
              <h2 className="ben-dosing-heading">{DOSING_INTRO.heading}</h2>
              <dl className="ben-dosing-list">
                {DOSING.map((d) => (
                  <div className="ben-dosing-row" key={d.k}>
                    <dt className="ben-dosing-key">{d.k}</dt>
                    <dd className="ben-dosing-val">{d.v}</dd>
                  </div>
                ))}
              </dl>
              <p className="ben-dosing-note">{DOSING_INTRO.footnote}</p>
            </div>

            <div className="ben-stack">
              <div className="ben-compare">
                <h3 className="home-h3">{COMPARE_HEADING}</h3>
                <dl className="ben-compare-list">
                  {COMPARE.map((c) => (
                    <div className="ben-compare-row" key={c.label}>
                      <dt className="ben-compare-label">{c.label}</dt>
                      <dd className="ben-compare-text">{c.text}</dd>
                    </div>
                  ))}
                </dl>
                <Link href="/shop" className="ben-inline-link">
                  {COMPARE_CTA_LABEL}
                  <ArrowRight size={15} />
                </Link>
              </div>

              {/* The refusals. Deliberately given the same weight as the dosing
                  panel beside them rather than shrunk into a footnote. */}
              <div id="limits" className="ben-limits">
                <h3 className="home-h3">{LIMITS_HEADING}</h3>
                <ul className="ben-limits-list">
                  {LIMITS.map((l) => (
                    <li key={l}>
                      <span className="ben-limits-mark" aria-hidden="true">
                        ✕
                      </span>
                      <span>{l}</span>
                    </li>
                  ))}
                </ul>
                <p className="ben-limits-note">{LIMITS_DISCLAIMER}</p>
              </div>
            </div>
          </div>
        </section>

        {/* ------------------------------------------------------------- faq */}
        <section className="home-section home-section--narrow">
          <h2 className="home-h2">{FAQ_HEADING}</h2>
          <BenefitsFaqList />
        </section>

        {/* ------------------------------------------------------------- cta */}
        <section className="home-section">
          <div className="ben-closing">
            <div className="ben-closing-text">
              <h2 className="home-h2">{BENEFITS_CTA.heading}</h2>
              <p className="home-section-body">{BENEFITS_CTA.body}</p>
            </div>
            <Link href="/shop" className="home-cta">
              {BENEFITS_CTA.ctaLabel}
              <ArrowRight />
            </Link>
          </div>
        </section>
      </main>
    </>
  );
}
