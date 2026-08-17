'use client';

import Link from 'next/link';
import { useEffect, useId, useRef, useState, type ReactNode } from 'react';

import { BrandLockup } from '@/components/BrandLockup';

/**
 * One header for the whole site.
 *
 * The nav is deliberately tiny — links to the pages you are not on. "Our Farm"
 * was added once `/farm` existed to receive it; "Benefits" is still absent for
 * the same reason it always was, which is that it resolves nowhere. Add an
 * entry here only when its page is real, never as a placeholder that does
 * nothing.
 *
 * **The burger is back, below 560px.** Task 7.15 removed it when the nav was a
 * single link, and that was right at the time: hiding one link behind a
 * hamburger costs a tap and buys nothing. Two links changed the arithmetic. On
 * the shop — the one header that also carries an account control and a cart —
 * brand + two labels + those two controls measure 417px against a 375px
 * screen, so the labels wrapped onto two lines each and the bar looked broken.
 * Above 560px there is room for the links inline, so they stay inline.
 *
 * `right` is the shop's account control and cart. The homepage passes nothing,
 * so it never implies a cart it cannot show.
 */
export interface NavItem {
  label: string;
  href: string;
}

export function SiteHeader({ nav, right }: { nav: NavItem[]; right?: ReactNode }) {
  const [scrolled, setScrolled] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuId = useId();
  const burgerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // The bar shrinks and gains a surface once the page moves, per the design.
  useEffect(() => {
    const onScroll = () => {
      const y = window.scrollY || document.documentElement.scrollTop;
      setScrolled((was) => (y > 24 === was ? was : y > 24));
    };
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  // Escape closes and hands focus back to the button that opened it; a click
  // anywhere outside just closes. Both listeners are only attached while the
  // menu is open, so a closed header costs nothing.
  useEffect(() => {
    if (!menuOpen) return;

    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      setMenuOpen(false);
      burgerRef.current?.focus();
    };
    const onPointerDown = (e: MouseEvent | TouchEvent) => {
      const t = e.target as Node;
      if (menuRef.current?.contains(t) || burgerRef.current?.contains(t)) return;
      setMenuOpen(false);
    };

    document.addEventListener('keydown', onKey);
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('touchstart', onPointerDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('touchstart', onPointerDown);
    };
  }, [menuOpen]);

  // Widening past the breakpoint puts the links back in the bar. Without this
  // the menu stays mounted but invisible, so focus could land inside a panel
  // nobody can see.
  useEffect(() => {
    const mq = window.matchMedia('(min-width: 561px)');
    const onChange = () => { if (mq.matches) setMenuOpen(false); };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  return (
    <header className={`site-header${scrolled ? ' site-header--scrolled' : ''}`}>
      <div className="site-header-inner">
        <Link href="/" className="site-header-brand" aria-label="Enerzeia Future Farm — home">
          <BrandLockup />
        </Link>

        {/* Inline above 560px, hidden below it — the burger takes over there. */}
        <nav className="site-nav" aria-label="Primary">
          {nav.map((item) => (
            <Link href={item.href} key={item.href}>
              {item.label}
            </Link>
          ))}
        </nav>

        {right && <div className="site-header-right">{right}</div>}

        <button
          ref={burgerRef}
          type="button"
          className="site-burger"
          aria-label={menuOpen ? 'Close menu' : 'Open menu'}
          aria-expanded={menuOpen}
          aria-controls={menuId}
          onClick={() => setMenuOpen((o) => !o)}
        >
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.25"
            strokeLinecap="round"
            aria-hidden="true"
          >
            {menuOpen ? (
              <path d="M6 6l12 12M18 6L6 18" />
            ) : (
              <path d="M4 7h16M4 12h16M4 17h16" />
            )}
          </svg>
        </button>
      </div>

      {menuOpen && (
        <div className="site-menu" id={menuId} ref={menuRef}>
          <nav className="site-menu-list" aria-label="Primary">
            {nav.map((item) => (
              <Link
                href={item.href}
                key={item.href}
                className="site-menu-link"
                onClick={() => setMenuOpen(false)}
              >
                {item.label}
              </Link>
            ))}
          </nav>
        </div>
      )}
    </header>
  );
}
