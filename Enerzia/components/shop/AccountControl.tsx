'use client';

import { useEffect, useRef, useState } from 'react';

import type { ShopAction } from '@/lib/shop/reducer';
import type { UserDTO } from '@/lib/shop/types';

export function AccountControl({
  user,
  dispatch,
  signOut,
}: {
  user: UserDTO | null;
  dispatch: (a: ShopAction) => void;
  signOut: () => void;
}) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close when clicking outside the control
  useEffect(() => {
    if (!open) return;
    function onMouseDown(e: MouseEvent) {
      const t = e.target as Node;
      if (!menuRef.current?.contains(t) && !triggerRef.current?.contains(t)) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', onMouseDown);
    return () => document.removeEventListener('mousedown', onMouseDown);
  }, [open]);

  // Escape closes and returns focus to the trigger
  useEffect(() => {
    if (!open) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setOpen(false);
        triggerRef.current?.focus();
      }
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [open]);

  // Move focus to the first menuitem when the menu opens
  useEffect(() => {
    if (!open) return;
    const first = menuRef.current?.querySelector<HTMLElement>('[role="menuitem"]');
    first?.focus();
  }, [open]);

  if (!user) {
    return (
      <button
        type="button"
        className="btn-account-signin"
        onClick={() => dispatch({ type: 'requireAuth', dest: 'cart' })}
      >
        Sign in
      </button>
    );
  }

  function handleArrowKey(e: React.KeyboardEvent<HTMLDivElement>) {
    const items = Array.from(
      menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [],
    );
    const idx = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      items[(idx + 1) % items.length]?.focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      items[(idx - 1 + items.length) % items.length]?.focus();
    }
  }

  function navigate(screen: 'orders' | 'addresses' | 'contact') {
    setOpen(false);
    dispatch({ type: 'go', screen });
  }

  function handleSignOut() {
    setOpen(false);
    signOut();
  }

  return (
    <div className="account-control">
      <button
        ref={triggerRef}
        type="button"
        className="btn-account"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="Account menu"
        onClick={() => setOpen((o) => !o)}
      >
        <svg
          width="18"
          height="18"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.25"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <circle cx="12" cy="8" r="4" />
          <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7" />
        </svg>
      </button>

      {open && (
        <div
          ref={menuRef}
          role="menu"
          aria-label="Account"
          className="account-menu"
          onKeyDown={handleArrowKey}
        >
          <div className="account-menu-header">+91 {user.phone}</div>
          <button
            type="button"
            role="menuitem"
            className="account-menu-item"
            onClick={() => navigate('orders')}
          >
            My orders
          </button>
          <button
            type="button"
            role="menuitem"
            className="account-menu-item"
            onClick={() => navigate('addresses')}
          >
            Saved addresses
          </button>
          <button
            type="button"
            role="menuitem"
            className="account-menu-item"
            onClick={() => navigate('contact')}
          >
            Contact us
          </button>
          <div role="separator" className="account-menu-sep" />
          <button
            type="button"
            role="menuitem"
            className="account-menu-item account-menu-signout"
            onClick={handleSignOut}
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}
