'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { me } from '@/lib/api/auth';
import { listProducts } from '@/lib/api/products';
import { clearToken, getToken, onUnauthorized } from '@/lib/api/token';
import { isApiError } from '@/lib/api/errors';
import { PageShell } from '@/components/PageShell';
import type { AdminProduct } from '@/lib/api/types';

function fmtPrice(paise: number): string {
  return `₹${(paise / 100).toFixed(0)}`;
}

export default function ProductsPage() {
  const router = useRouter();
  const [ready, setReady] = useState(false);
  const [email, setEmail] = useState('');
  const [products, setProducts] = useState<AdminProduct[] | null>(null);
  const [loadError, setLoadError] = useState('');

  useEffect(() => {
    const tok = getToken();
    if (!tok) { router.replace('/login'); return; }
    let cancelled = false;
    me()
      .then((d) => { if (!cancelled) { setEmail(d.email); setReady(true); } })
      .catch(() => { clearToken(); if (!cancelled) router.replace('/login'); });
    return () => { cancelled = true; };
  }, [router]);

  useEffect(() => {
    onUnauthorized(() => { clearToken(); router.replace('/login'); });
  }, [router]);

  useEffect(() => {
    if (!ready) return;
    let cancelled = false;
    listProducts()
      .then((ps) => { if (!cancelled) setProducts(ps); })
      .catch((err) => {
        if (!cancelled) {
          setLoadError(
            isApiError(err) ? err.message : 'Could not load products. Check your connection.',
          );
        }
      });
    return () => { cancelled = true; };
  }, [ready]);

  if (!ready) return <div className="loading-center">Loading…</div>;

  return (
    <PageShell email={email}>
      <main className="page-content">
        <div className="list-toolbar">
          <h2 className="page-title">Products</h2>
          <Link href="/products/new" className="btn btn-primary">
            New product
          </Link>
        </div>

        {loadError ? (
          <div className="error-msg">{loadError}</div>
        ) : products === null ? (
          <p className="muted">Loading products…</p>
        ) : products.length === 0 ? (
          <p className="muted">
            No products yet.{' '}
            <Link href="/products/new">Create the first one.</Link>
          </p>
        ) : (
          <div className="table-wrap">
            <table className="product-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th>Form</th>
                  <th>Price</th>
                  <th>MRP</th>
                  <th>Stock</th>
                  <th>Images</th>
                </tr>
              </thead>
              <tbody>
                {products.map((p) => (
                  <tr key={p.id} className={p.active ? undefined : 'row-retired'}>
                    <td>
                      <Link href={`/products/${p.id}`} className="product-link">
                        {p.id}
                      </Link>
                      {!p.active && <span className="badge-retired">Retired</span>}
                    </td>
                    <td>{p.name}</td>
                    <td>{p.form}</td>
                    <td className="cell-num">{fmtPrice(p.price)}</td>
                    <td className="cell-num">{fmtPrice(p.mrp)}</td>
                    <td className="cell-num">{p.stock}</td>
                    <td className="cell-num">{p.images.length}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>
    </PageShell>
  );
}
