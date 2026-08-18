'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { me } from '@/lib/api/auth';
import { listOrders } from '@/lib/api/orders';
import { clearToken, getToken, onUnauthorized } from '@/lib/api/token';
import { isApiError } from '@/lib/api/errors';
import { PageShell } from '@/components/PageShell';
import type { AdminOrder } from '@/lib/api/types';

/* ----------------------------------------------------------------- helpers */

/** Divides paise by 100 and formats as ₹1,23,456 (en-IN). */
function fmtRupees(paise: number): string {
  // maximumFractionDigits: 0 alone ROUNDS: an order of ₹1,247.90 displayed as
  // ₹1,248, which is simply a wrong number in an order book. Show paise when
  // present, omit them when zero.
  const hasPaise = paise % 100 !== 0;
  return (
    '₹' +
    (paise / 100).toLocaleString('en-IN', {
      minimumFractionDigits: hasPaise ? 2 : 0,
      maximumFractionDigits: hasPaise ? 2 : 0,
    })
  );
}

function fmtDate(iso: string): string {
  return new Date(iso).toLocaleString('en-IN', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: true,
  });
}

const STATUS_CLASS: Record<string, string> = {
  placed: 'chip-placed',
  pending_payment: 'chip-pending',
  payment_failed: 'chip-failed',
  expired: 'chip-muted',
  packed: 'chip-packed',
  in_transit: 'chip-transit',
  shipped: 'chip-shipped',
  delivered: 'chip-delivered',
  cancelled: 'chip-failed',
};

const FULFILMENT_CLASS: Record<string, string> = {
  packed: 'chip-packed',
  in_transit: 'chip-transit',
  shipped: 'chip-shipped',
};

const STATUS_OPTIONS = [
  { value: '', label: 'Default (env-dependent)' },
  { value: 'all', label: 'All statuses' },
  { value: 'placed', label: 'Placed' },
  { value: 'pending_payment', label: 'Pending payment' },
  { value: 'payment_failed', label: 'Payment failed' },
  { value: 'expired', label: 'Expired' },
  { value: 'packed', label: 'Packed' },
  { value: 'shipped', label: 'Shipped' },
  { value: 'delivered', label: 'Delivered' },
  { value: 'cancelled', label: 'Cancelled' },
];

// Filter options have to be authored here — the API exposes no list of them.
// Their labels must still be the server's exact words (Fulfilment.Label in
// internal/order/model.go), or an operator filters by one word and reads
// another in the rows it returns. "In transit" was that drift.
const FULFILMENT_OPTIONS = [
  { value: '', label: 'Any fulfilment' },
  { value: 'none', label: 'Not started' },
  { value: 'packed', label: 'Processed' },
  { value: 'in_transit', label: 'Transit' },
  { value: 'shipped', label: 'Shipped' },
];

const LIMIT_OPTIONS = [10, 25, 50, 100];

/* ------------------------------------------------------------------ types */

interface Filters {
  status: string;
  fulfilment: string;
  limit: number;
}

/* ------------------------------------------------------------------ page */

export default function OrdersPage() {
  const router = useRouter();
  const [ready, setReady] = useState(false);
  const [email, setEmail] = useState('');

  // Filter state
  const [filters, setFilters] = useState<Filters>({ status: '', fulfilment: '', limit: 50 });
  // Pending (uncommitted) filter state — applied when "Apply" is clicked
  const [draft, setDraft] = useState<Filters>({ status: '', fulfilment: '', limit: 50 });

  // Cursor stack. cursorStack[0] is always undefined (first page).
  // cursorStack[n] is the before= cursor for page n.
  const [cursorStack, setCursorStack] = useState<Array<string | undefined>>([undefined]);
  const currentCursor = cursorStack[cursorStack.length - 1];

  // Result state
  const [orders, setOrders] = useState<AdminOrder[] | null>(null);
  const [nextBefore, setNextBefore] = useState<string | undefined>(undefined);
  const [loadError, setLoadError] = useState('');
  const [loading, setLoading] = useState(false);

  // Track the "active" filters to distinguish "no results" vs "load error".
  // Also used to detect whether the filter changed since last load.
  const activeFiltersRef = useRef(filters);

  /* ------------------------------------------------- auth guard */

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

  /* ------------------------------------------------- load orders */

  const load = useCallback(
    (f: Filters, cursor: string | undefined) => {
      if (!ready) return;
      let cancelled = false;
      setLoading(true);
      setOrders(null);
      setLoadError('');
      activeFiltersRef.current = f;

      listOrders({
        status: f.status || undefined,
        fulfilment: f.fulfilment || undefined,
        limit: f.limit,
        before: cursor,
      })
        .then((data) => {
          if (cancelled) return;
          setOrders(data.orders);
          setNextBefore(data.nextBefore);
          setLoading(false);
        })
        .catch((err) => {
          if (cancelled) return;
          setLoading(false);
          if (isApiError(err)) {
            setLoadError(err.message);
          } else {
            setLoadError('Could not load orders. Check your connection.');
          }
        });
      return () => { cancelled = true; };
    },
    [ready],
  );

  // Load on mount (once ready) and whenever filters or cursor changes.
  useEffect(() => {
    if (!ready) return;
    return load(filters, currentCursor);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, filters, currentCursor]);

  /* ------------------------------------------------- filter apply */

  function applyFilters() {
    // Reset to first page whenever the filter changes.
    setCursorStack([undefined]);
    setFilters({ ...draft });
  }

  /* ------------------------------------------------- pagination */

  function goNext() {
    if (!nextBefore) return;
    setCursorStack((s) => [...s, nextBefore]);
  }

  function goPrev() {
    if (cursorStack.length <= 1) return;
    setCursorStack((s) => s.slice(0, -1));
  }

  const pageIndex = cursorStack.length - 1; // 0-based
  const isFirstPage = pageIndex === 0;
  const isLastPage = !nextBefore;

  /* ------------------------------------------------- render */

  if (!ready) return <div className="loading-center">Loading…</div>;

  const hasActiveFilter = filters.status !== '' || filters.fulfilment !== '';

  return (
    <PageShell email={email}>
      <main className="page-content page-content--wide">
        <div className="list-toolbar">
          <h2 className="page-title">Orders</h2>
        </div>

        {/* Filter bar */}
        <div className="filter-bar">
          <div className="filter-group">
            <label className="filter-label" htmlFor="filter-status">Status</label>
            <select
              id="filter-status"
              className="filter-select"
              value={draft.status}
              onChange={(e) => setDraft((d) => ({ ...d, status: e.target.value }))}
            >
              {STATUS_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
          </div>

          <div className="filter-group">
            <label className="filter-label" htmlFor="filter-fulfilment">Fulfilment</label>
            <select
              id="filter-fulfilment"
              className="filter-select"
              value={draft.fulfilment}
              onChange={(e) => setDraft((d) => ({ ...d, fulfilment: e.target.value }))}
            >
              {FULFILMENT_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
          </div>

          <div className="filter-group">
            <label className="filter-label" htmlFor="filter-limit">Per page</label>
            <select
              id="filter-limit"
              className="filter-select"
              value={draft.limit}
              onChange={(e) => setDraft((d) => ({ ...d, limit: Number(e.target.value) }))}
            >
              {LIMIT_OPTIONS.map((n) => (
                <option key={n} value={n}>{n}</option>
              ))}
            </select>
          </div>

          <button type="button" className="btn btn-primary btn-filter-apply" onClick={applyFilters}>
            Apply
          </button>
        </div>

        {/* Content */}
        {loadError ? (
          <div className="order-state">
            <p className="order-state-title">Could not load orders</p>
            <p className="order-state-body error-text">{loadError}</p>
          </div>
        ) : loading ? (
          <p className="muted">Loading orders…</p>
        ) : orders !== null && orders.length === 0 ? (
          <div className="order-state">
            {hasActiveFilter ? (
              <>
                <p className="order-state-title">No orders match this filter</p>
                <p className="order-state-body">Try removing the status or fulfilment filter to see more.</p>
              </>
            ) : (
              <>
                <p className="order-state-title">No orders yet</p>
                <p className="order-state-body">Orders will appear here once customers place them.</p>
              </>
            )}
          </div>
        ) : orders !== null ? (
          <>
            <div className="table-wrap">
              <table className="order-table">
                <thead>
                  <tr>
                    <th>Order</th>
                    <th>Date</th>
                    <th>Phone</th>
                    <th className="cell-num">Items</th>
                    <th className="cell-num">Total</th>
                    <th>Status</th>
                    <th>Fulfilment</th>
                  </tr>
                </thead>
                <tbody>
                  {orders.map((o) => (
                    <tr key={o.orderId}>
                      <td className="cell-orderid">
                        <Link href={`/orders/${o.orderId}`} className="orderid-link">
                          {o.orderId}
                        </Link>
                      </td>
                      <td className="cell-date">{fmtDate(o.createdAt)}</td>
                      <td className="cell-phone">
                        {o.customer.phone ?? <span className="muted-cell">—</span>}
                      </td>
                      <td className="cell-num">
                        {o.lines.reduce((acc, l) => acc + l.qty, 0)}
                      </td>
                      <td className="cell-num">{fmtRupees(o.totals.total)}</td>
                      <td>
                        <span className={`status-chip ${STATUS_CLASS[o.status] ?? 'chip-muted'}`}>
                          {o.statusLabel}
                        </span>
                      </td>
                      <td>
                        <span
                          className={`status-chip ${
                            o.fulfilment ? (FULFILMENT_CLASS[o.fulfilment] ?? 'chip-muted') : 'chip-muted'
                          }`}
                        >
                          {o.fulfilmentLabel}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="pagination-bar">
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={goPrev}
                disabled={isFirstPage}
              >
                ← Prev
              </button>
              <span className="pagination-info">
                Page {pageIndex + 1} &middot; {orders.length} row{orders.length !== 1 ? 's' : ''}
              </span>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={goNext}
                disabled={isLastPage}
              >
                Next →
              </button>
            </div>
          </>
        ) : null}
      </main>
    </PageShell>
  );
}
