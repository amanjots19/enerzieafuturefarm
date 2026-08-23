import type { Metadata } from 'next';
import Link from 'next/link';

import { SiteHeader, type NavItem } from '@/components/SiteHeader';
import { Photo } from '@/components/Photo';
import {
  BYTE,
  FARM_CTA,
  FARM_HERO,
  GRADE_INTRO,
  GRADE_POINTS,
  OWN_FARM_POINTS,
  PROCESS,
  PROCESS_INTRO,
  VISIT,
} from '@/lib/content/farm';

// This IS the farm page, so the nav offers the two places it is not.
const FARM_NAV: NavItem[] = [
  { label: 'Home', href: '/' },
  { label: 'Shop', href: '/shop' },
];

export const metadata: Metadata = {
  title: 'Our farm - where Enerzeia spirulina is grown',
  description:
    'Enerzeia spirulina is grown in our own lined raceway ponds, harvested by filtering, dried at low heat and lab-tested batch by batch. Nothing is bought in and relabelled.',
};

/**
 * "Our Farm" — the sourcing page.
 *
 * A server component: the copy is static and the only interactive piece is
 * Photo, which is a client island purely so a missing photograph can fall
 * back to its gradient.
 *
 * The photographs are the argument here, so the layout alternates image and
 * text down the process rather than stacking the words in one column with a
 * gallery underneath — a reader should not be able to take the claim without
 * seeing the pond it refers to.
 *
 * The footer comes from the root layout and is not rendered here.
 */
export default function FarmPage() {
  return (
    <>
      <SiteHeader nav={FARM_NAV} />

      <main className="farm">
        {/* ------------------------------------------------------------ hero */}
        <section className="farm-hero-section">
          <div className="farm-hero">
            <div>
              <p className="home-eyebrow-pill">
                <span className="home-eyebrow-dot" aria-hidden="true" />
                {FARM_HERO.eyebrow}
              </p>
              <h1 className="home-h1">
                {FARM_HERO.headingLines.map((line, i) => (
                  <span key={line}>
                    {line}
                    {i < FARM_HERO.headingLines.length - 1 && <br />}
                  </span>
                ))}
              </h1>
              <p className="home-hero-body">{FARM_HERO.body}</p>
            </div>

            <Photo photo={FARM_HERO.photo} frameClassName="farm-photo farm-photo--tall" priority />
          </div>

          <div className="farm-points">
            {OWN_FARM_POINTS.map((p) => (
              <div className="farm-point" key={p.title}>
                <h2 className="farm-point-title">{p.title}</h2>
                <p className="farm-point-body">{p.body}</p>
              </div>
            ))}
          </div>
        </section>

        {/* --------------------------------------------------------- process */}
        <section id="process" className="home-section">
          <div className="home-section-intro">
            <p className="home-eyebrow">{PROCESS_INTRO.eyebrow}</p>
            <h2 className="home-h2">{PROCESS_INTRO.heading}</h2>
            <p className="home-section-body">{PROCESS_INTRO.body}</p>
          </div>

          {/* Every step carries a photograph — `ProcessStep.photo` is required,
              so a stage nobody has shot cannot be added here without the
              compiler objecting. That is deliberate: a gap in an alternating
              image/text rhythm reads as a missing picture, not as a design. */}
          <ol className="farm-steps">
            {PROCESS.map((s) => (
              <li className="farm-step" key={s.step}>
                <div className="farm-step-text">
                  <span className="farm-step-num" aria-hidden="true">
                    {s.step}
                  </span>
                  <h3 className="home-h3">{s.title}</h3>
                  <p className="farm-step-body">{s.body}</p>
                </div>
                <Photo photo={s.photo} frameClassName="farm-photo farm-photo--landscape" />
              </li>
            ))}
          </ol>

          {/* The process ends on the invitation rather than on another claim:
              somebody who has just read the whole route is the likeliest
              person to want to come and see it. */}
          <div className="farm-visit-panel">
            <p className="home-eyebrow">{VISIT.eyebrow}</p>
            <h3 className="home-h2">{VISIT.heading}</h3>
            <p className="farm-visit-body">{VISIT.body}</p>
            <Link href="/contact" className="home-cta">
              {VISIT.ctaLabel}
              <svg
                width="17"
                height="17"
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
            </Link>
          </div>
        </section>

        {/* ----------------------------------------------------------- grade */}
        <section className="home-section">
          <div className="home-section-intro">
            <p className="home-eyebrow">{GRADE_INTRO.eyebrow}</p>
            <h2 className="home-h2">{GRADE_INTRO.heading}</h2>
          </div>

          <div className="farm-grade-wrap">
            <Photo photo={GRADE_INTRO.photo} frameClassName="farm-photo farm-photo--portrait farm-grade-photo" />
            <div className="farm-grade">
              {GRADE_POINTS.map((g) => (
                <article className="farm-grade-card" key={g.title}>
                  <h3 className="home-h3">{g.title}</h3>
                  <p className="farm-grade-body">{g.body}</p>
                </article>
              ))}
            </div>
          </div>
        </section>

        {/* ------------------------------------------------------------ byte */}
        <section id="byte" className="home-section">
          <figure className="farm-byte">
            <Photo photo={BYTE.photo} frameClassName="farm-photo farm-photo--portrait farm-byte-photo" />
            <div className="farm-byte-text">
              <span className="farm-byte-mark" aria-hidden="true">
                &ldquo;
              </span>
              <blockquote className="farm-byte-quote">{BYTE.quote}</blockquote>
              <figcaption className="farm-byte-who">
                <span className="farm-byte-name">{BYTE.name}</span>
                <span className="farm-byte-role">{BYTE.role}</span>
              </figcaption>
            </div>
          </figure>
        </section>

        {/* --------------------------------------------- closing + visit ask */}
        <section className="home-section farm-closing-section">
          <div className="farm-closing">
            <Photo photo={FARM_CTA.photo} frameClassName="farm-photo farm-photo--portrait farm-closing-photo" />
            <div className="farm-closing-text">
              <h2 className="home-h2">{FARM_CTA.heading}</h2>
              <p className="home-section-body">{FARM_CTA.body}</p>
              <Link href="/shop" className="home-cta">
                {FARM_CTA.ctaLabel}
                <svg
                  width="17"
                  height="17"
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
              </Link>
            </div>
          </div>
        </section>
      </main>
    </>
  );
}
