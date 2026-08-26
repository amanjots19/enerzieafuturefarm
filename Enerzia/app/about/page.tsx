import type { Metadata } from 'next';
import Link from 'next/link';

import { Photo } from '@/components/Photo';
import { SiteHeader, type NavItem } from '@/components/SiteHeader';
import { HomeFaqList } from '@/components/home/HomeFaqList';
import {
  HERO,
  HERO_CHIPS,
  LIFESTYLE_INTRO,
  MOMENTS,
  PROOF,
  SEALS,
  STARTING_OUT,
  TASTE_TIPS,
} from '@/lib/content/home';
import { TESTIMONIALS } from '@/lib/content/testimonials';
import { pageMetadata } from '@/lib/seo';

// No "About us" — this IS that page. Shop is pinned, so it stays in the bar on
// a phone rather than falling into the burger with the other two.
const ABOUT_NAV: NavItem[] = [
  { label: 'Shop', href: '/shop', pinned: true },
  { label: 'Benefits', href: '/benefits' },
  { label: 'Our Farm', href: '/farm' },
];

export const metadata: Metadata = pageMetadata({
  path: '/about',
  title: 'About us - spirulina grown on our own farm | Enerzeia Future Farm',
  description: 'Who we are: spirulina harvested on our own farm, sun-dried at low heat and lab-tested batch by batch. How buyers use it, and how every batch is checked before it ships.',
});

/**
 * "About us" — the brand story page.
 *
 * THIS WAS THE HOMEPAGE, at `/`, until the root was pointed at the shop (see
 * the redirect in next.config.ts). Nothing about the page itself changed in
 * that move; it kept its `.home-*` class names, which are now the shared type
 * scale for /about, /farm and /benefits rather than one page's private
 * stylesheet. Renaming them would be a large diff for no behaviour change.
 *
 * A server component: everything here is static content, and the interactive
 * pieces are isolated client islands (AnnouncementBar, SiteHeader,
 * HomeFaqList).
 *
 * The design's #farm and #benefits sections were never built into this page —
 * both got their own routes instead (/farm, /benefits), which is where the
 * header's two entries point.
 *
 * The footer comes from the root layout, so it is not rendered here.
 */
export default function AboutPage() {
  return (
    <>
      {/* The announcement bar lives in the root layout so it sits above every
          header, not just this one. No cart or account in this header; both are
          shop-only, so this page never implies a cart it cannot show. */}
      <SiteHeader nav={ABOUT_NAV} />

      <main className="home">
        {/* ---------------------------------------------------------- hero */}
        <section className="home-hero-section">
          <div className="home-hero">
            <div>
              <p className="home-eyebrow-pill">
                <span className="home-eyebrow-dot" aria-hidden="true" />
                {HERO.eyebrow}
              </p>
              <h1 className="home-h1">
                {HERO.headingLines.map((line, i) => (
                  <span key={line}>
                    {line}
                    {i < HERO.headingLines.length - 1 && <br />}
                  </span>
                ))}
              </h1>
              <p className="home-hero-body">{HERO.body}</p>

              <div className="home-cta-row">
                <Link href="/shop" className="home-cta">
                  {HERO.ctaLabel}
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
                <a href="#proof" className="home-cta-quiet">
                  See how we test it
                </a>
              </div>

              <div className="home-chips">
                {HERO_CHIPS.map((c) => (
                  <a key={c.label} href={c.href} className="home-chip">
                    <span className="home-chip-dot" aria-hidden="true" />
                    {c.label}
                  </a>
                ))}
              </div>
            </div>

            <div className="home-hero-art-wrap">
              <Photo photo={HERO.photo} frameClassName="home-hero-art" priority />
              <div className="home-hero-badge">
                <div className="home-hero-badge-big">{HERO.badgeBig}</div>
                <div className="home-hero-badge-label">{HERO.badgeLabel}</div>
              </div>
            </div>
          </div>
        </section>

        {/* ----------------------------------------------------- lifestyle */}
        <section id="lifestyle" className="home-section">
          <div className="home-section-intro">
            <p className="home-eyebrow">{LIFESTYLE_INTRO.eyebrow}</p>
            <h2 className="home-h2">{LIFESTYLE_INTRO.heading}</h2>
            <p className="home-section-body">{LIFESTYLE_INTRO.body}</p>
          </div>

          <div className="home-moments">
            {MOMENTS.map((m) => (
              <article className="home-moment" key={m.title}>
                <Photo photo={m.photo} frameClassName="home-moment-art" />
                <div className="home-moment-body">
                  <div className="home-moment-when">{m.when}</div>
                  <h3 className="home-h3">{m.title}</h3>
                  <p className="home-moment-text">{m.body}</p>
                  <div className="home-moment-form">
                    <span>Form:</span> {m.form}
                  </div>
                </div>
              </article>
            ))}
          </div>

          <div className="home-two-up">
            <div className="home-tips">
              <h3 className="home-h3">Making it taste good</h3>
              <ul className="home-tip-list">
                {TASTE_TIPS.map((t) => (
                  <li key={t}>{t}</li>
                ))}
              </ul>
            </div>
            <div className="home-starting">
              <h3 className="home-h3">{STARTING_OUT.heading}</h3>
              <p className="home-starting-body">{STARTING_OUT.body}</p>
              <p className="home-starting-caution">{STARTING_OUT.caution}</p>
            </div>
          </div>

          {/* -------------------------------------------------------- proof */}
          <div id="proof" className="home-proof">
            <span className="home-proof-watermark" aria-hidden="true">
              {PROOF.watermark}
            </span>
            <div className="home-proof-head">
              <h3 className="home-h3">{PROOF.heading}</h3>
              <span className="home-proof-aside">{PROOF.aside}</span>
            </div>
            <div className="home-seals">
              {SEALS.map((s) => (
                <div className="home-seal" key={s.title}>
                  <div className="home-seal-ring">
                    <div className="home-seal-inner">
                      <span className="home-seal-mark">{s.mark}</span>
                      <span className="home-seal-sub">{s.ring}</span>
                    </div>
                  </div>
                  <div>
                    <div className="home-seal-title">{s.title}</div>
                    <div className="home-seal-body">{s.body}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* --------------------------------------------------------- reviews */}
        <section className="home-section">
          <h2 className="home-h2">From our buyers</h2>
          <div className="home-reviews">
            {TESTIMONIALS.slice(0, 3).map((t) => (
              <figure className="home-review" key={t.headline}>
                <div className="home-review-stars" aria-label={`${t.rating} out of 5`}>
                  {'★'.repeat(t.rating)}
                </div>
                <blockquote className="home-review-quote">{t.headline}</blockquote>
                <p className="home-review-body">{t.body}</p>
                <figcaption className="home-review-who">{t.attribution}</figcaption>
              </figure>
            ))}
          </div>
        </section>

        {/* ------------------------------------------------------------- faq */}
        <section className="home-section home-section--narrow">
          <h2 className="home-h2">Questions we get</h2>
          <HomeFaqList />
        </section>
      </main>
    </>
  );
}
