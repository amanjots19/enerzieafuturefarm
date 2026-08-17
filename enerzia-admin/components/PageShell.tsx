'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { clearToken } from '@/lib/api/token';

interface Props {
  email: string;
  children: React.ReactNode;
}

export function PageShell({ email, children }: Props) {
  const router = useRouter();

  function signOut() {
    clearToken();
    router.replace('/login');
  }

  return (
    <div className="page-shell">
      <header className="page-header">
        <div className="header-left">
          <Link href="/products" className="header-brand">
            Enerzia Admin
          </Link>
          <nav className="header-nav">
            <Link href="/products" className="nav-link">Products</Link>
            <Link href="/orders" className="nav-link">Orders</Link>
          </nav>
        </div>
        <div className="header-actions">
          <span className="header-email">{email}</span>
          <button type="button" className="btn btn-ghost" onClick={signOut}>
            Sign out
          </button>
        </div>
      </header>
      {children}
    </div>
  );
}
