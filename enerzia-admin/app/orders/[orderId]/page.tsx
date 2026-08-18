'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { me } from '@/lib/api/auth';
import { fetchLabelHTML, getOrder, setFulfilment } from '@/lib/api/orders';
import { clearToken, getToken, onUnauthorized } from '@/lib/api/token';
import { isApiError } from '@/lib/api/errors';
import { PageShell } from '@/components/PageShell';
import type { AdminOrder } from '@/lib/api/types';

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
  shipped: 'chip-shipped',
  delivered: 'chip-delivered',
  cancelled: 'chip-failed',
};

/**
 * Statuses the server will print a label for. Mirrors paidStatuses in
 * internal/order/admin_repository.go — the server still refuses with a 409, so
 * this only decides whether to offer the button rather than sit an operator in
 * front of a control that always errors.
 */
const LABELLABLE = new Set(['placed', 'packed', 'shipped', 'delivered', 'cancelled']);

const FULFILMENT_CLASS: Record<string, string> = {
  packed: 'chip-packed',
  in_transit: 'chip-transit',
  shipped: 'chip-shipped',
};

/**
 * The one step this order can advance to, and the button label for it.
 *
 * This mirrors the server's sequence so the button can be labelled before the
 * click. The server is still the authority: it re-checks and answers 409 with a
 * message naming what IS allowed, and that message is what gets shown. This is
 * a label, never a permission.
 */
const NEXT_STEP: Record<string, { value: string; label: string } | undefined> = {
  '': { value: 'packed', label: 'Mark Processed' },
  packed: { value: 'in_transit', label: 'Mark Transit' },
  in_transit: { value: 'shipped', label: 'Mark Shipped' },
  shipped: undefined, // terminal — nothing tells us a parcel arrived
};

export default function OrderDetailPage() {
  const router = useRouter();
  const params = useParams<{ orderId: string }>();
  const orderId = params.orderId;

  const [email, setEmail] = useState('');
  const [order, setOrder] = useState<AdminOrder | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);

  const [advancing, setAdvancing] = useState(false);
  const [advanceError, setAdvanceError] = useState<string | null>(null);

  const [labelBusy, setLabelBusy] = useState(false);
  const [labelError, setLabelError] = useState<string | null>(null);

  const booted = useRef(false);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    setNotFound(false);
    try {
      setOrder(await getOrder(orderId));
    } catch (e) {
      if (isApiError(e) && e.status === 404) {
        setNotFound(true);
      } else {
        setLoadError(
          isApiError(e) ? e.message : 'Could not reach the server. Check your connection.',
        );
      }
    } finally {
      setLoading(false);
    }
  }, [orderId]);

  useEffect(() => {
    onUnauthorized(() => {
      clearToken();
      router.replace('/login');
    });
    if (booted.current) return;
    booted.current = true;

    if (!getToken()) {
      router.replace('/login');
      return;
    }
    void (async () => {
      try {
        setEmail((await me()).email);
      } catch {
        clearToken();
        router.replace('/login');
        return;
      }
      void load();
    })();
  }, [router, load]);

  const advance = async (to: string) => {
    setAdvancing(true);
    setAdvanceError(null);
    try {
      setOrder(await setFulfilment(orderId, to));
    } catch (e) {
      // The server's 409 explains what the operator can do instead — show it
      // verbatim rather than inventing a message here.
      setAdvanceError(
        isApiError(e) ? e.message : 'Could not reach the server. Check your connection.',
      );
      // A refusal usually means somebody else moved this order, so the state on
      // screen is stale. Re-read rather than leave a wrong badge showing.
      if (isApiError(e) && e.status === 409) void load();
    } finally {
      setAdvancing(false);
    }
  };

  /**
   * Prints the label via a hidden same-origin iframe rather than a popup.
   *
   * window.open is blocked by default when the call is not synchronous with the
   * click, and this one waits on a fetch. The iframe also lands the operator
   * straight in the print dialog, which is the whole point — from there they
   * print to the thermal printer or save a PDF.
   */
  const printLabel = async () => {
    setLabelBusy(true);
    setLabelError(null);
    try {
      const html = await fetchLabelHTML(orderId);
      const frame = document.createElement('iframe');
      frame.style.position = 'fixed';
      frame.style.right = '0';
      frame.style.bottom = '0';
      frame.style.width = '0';
      frame.style.height = '0';
      frame.style.border = '0';
      document.body.appendChild(frame);

      const doc = frame.contentDocument;
      if (!doc) throw new Error('no iframe document');
      doc.open();
      doc.write(html);
      doc.close();

      // Printing before the stylesheet applies produces an unstyled label at
      // the wrong size, so wait for the frame to settle first.
      const run = () => {
        frame.contentWindow?.focus();
        frame.contentWindow?.print();
        // Leave the frame long enough for the dialog to read it, then clean up.
        window.setTimeout(() => frame.remove(), 60_000);
      };
      if (frame.contentWindow?.document.readyState === 'complete') run();
      else frame.onload = run;
    } catch (e) {
      setLabelError(
        isApiError(e) ? e.message : 'Could not reach the server. Check your connection.',
      );
    } finally {
      setLabelBusy(false);
    }
  };

  /** Saves the label to the operator's machine as a file. */
  const downloadLabel = async () => {
    setLabelBusy(true);
    setLabelError(null);
    try {
      const html = await fetchLabelHTML(orderId);
      const url = URL.createObjectURL(new Blob([html], { type: 'text/html' }));
      const a = document.createElement('a');
      a.href = url;
      a.download = `label-${orderId}.html`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      // Revoking immediately can cancel the download in some browsers.
      window.setTimeout(() => URL.revokeObjectURL(url), 30_000);
    } catch (e) {
      setLabelError(
        isApiError(e) ? e.message : 'Could not reach the server. Check your connection.',
      );
    } finally {
      setLabelBusy(false);
    }
  };

  const next = order ? NEXT_STEP[order.fulfilment ?? ''] : undefined;
  const canFulfil = order?.status === 'placed';

  return (
    <PageShell email={email}>
      <main className="page-content page-content--wide">
      <div className="list-toolbar">
        <Link href="/orders" className="nav-link">
          ← Order book
        </Link>
      </div>

      {loading && <div className="order-state"><p className="order-state-body">Loading order…</p></div>}

      {notFound && !loading && (
        <div className="order-state">
          <p className="order-state-title">No such order</p>
          <p className="order-state-body">
            No order matches <code>{orderId}</code>.
          </p>
        </div>
      )}

      {loadError && !loading && (
        <div className="order-state" role="alert">
          <p className="order-state-title">Could not load this order</p>
          <p className="order-state-body error-text">{loadError}</p>
          <button className="btn" type="button" onClick={() => void load()}>
            Try again
          </button>
        </div>
      )}

      {order && !loading && (
        <>
          <div className="list-toolbar">
            <h2 className="page-title">{order.orderId}</h2>
            <div className="order-chips">
              <span className={`status-chip ${STATUS_CLASS[order.status] ?? 'chip-muted'}`}>
                {order.statusLabel}
              </span>
              <span
                className={`status-chip ${
                  order.fulfilment ? FULFILMENT_CLASS[order.fulfilment] ?? 'chip-muted' : 'chip-muted'
                }`}
              >
                {order.fulfilmentLabel}
              </span>
            </div>
            <p className="muted">
              Placed {order.placedAt ? fmtDate(order.placedAt) : '—'} · Created{' '}
              {fmtDate(order.createdAt)}
            </p>
          </div>

          <section className="detail-panel">
            <h3 className="card-title">Fulfilment</h3>
            {advanceError && (
              <p className="error-msg" role="alert">
                {advanceError}
              </p>
            )}
            {!canFulfil && (
              <p className="muted">
                Only a placed order can be fulfilled. This one is {order.statusLabel}.
              </p>
            )}
            {canFulfil && next && (
              <button
                className="btn btn-primary"
                type="button"
                disabled={advancing}
                onClick={() => void advance(next.value)}
              >
                {advancing ? 'Saving…' : next.label}
              </button>
            )}
            {canFulfil && !next && (
              <p className="muted">
                This order is {order.fulfilmentLabel}, which is the last step. There is no
                delivery confirmation — nothing tells us a parcel arrived.
              </p>
            )}
          </section>

          {LABELLABLE.has(order.status) && (
          <section className="detail-panel">
            <h3 className="card-title">Shipping label</h3>
            {labelError && (
              <p className="error-msg" role="alert">
                {labelError}
              </p>
            )}
            <div className="label-actions">
              <button
                className="btn btn-primary"
                type="button"
                disabled={labelBusy}
                onClick={() => void printLabel()}
              >
                {labelBusy ? 'Preparing…' : 'Print label'}
              </button>
              <button
                className="btn btn-ghost"
                type="button"
                disabled={labelBusy}
                onClick={() => void downloadLabel()}
              >
                Download
              </button>
            </div>
            <p className="muted label-hint">
              4×6 thermal. The print dialog also offers “Save as PDF”.
            </p>
          </section>
          )}

          <section className="detail-panel">
            <h3 className="card-title">Deliver to</h3>
            <address className="ship-to muted-none">
              <strong>{order.shippingAddress.name}</strong>
              <br />
              {order.shippingAddress.line1}
              <br />
              {order.shippingAddress.city}, {order.shippingAddress.state}{' '}
              {order.shippingAddress.pin}
              <br />
              {/* The delivery contact is frozen on the order, not read from the
                  address — for a gift they are different people. */}
              {order.customer.phone ?? <span className="muted">no phone on this order</span>}
              <br />
              {order.shippingAddress.email}
            </address>
          </section>

          <section className="detail-panel">
            <h3 className="card-title">Items</h3>
            <div className="table-wrap">
              <table className="order-table">
                <thead>
                  <tr>
                    <th>Product</th>
                    <th className="cell-num">Qty</th>
                    <th className="cell-num">Unit</th>
                    <th className="cell-num">Line total</th>
                  </tr>
                </thead>
                <tbody>
                  {order.lines.map((l) => (
                    <tr key={l.productId}>
                      <td>{l.name}</td>
                      <td className="cell-num">{l.qty}</td>
                      <td className="cell-num">{fmtRupees(l.unitPrice)}</td>
                      <td className="cell-num">{fmtRupees(l.lineTotal)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <dl className="totals">
              <div>
                <dt>MRP</dt>
                <dd>{fmtRupees(order.totals.mrpTotal)}</dd>
              </div>
              <div>
                <dt>Savings</dt>
                <dd>− {fmtRupees(order.totals.savings)}</dd>
              </div>
              <div>
                <dt>Delivery</dt>
                <dd>
                  {order.totals.shipping === 0 ? 'Free' : fmtRupees(order.totals.shipping)}
                </dd>
              </div>
              <div className="totals-grand">
                <dt>Total</dt>
                <dd>{fmtRupees(order.totals.total)}</dd>
              </div>
            </dl>
          </section>

          <section className="detail-panel">
            <h3 className="card-title">Payment</h3>
            {order.payment ? (
              <dl className="kv">
                <div>
                  <dt>Method</dt>
                  <dd>
                    {order.payment.label ?? (
                      <span className="muted">not recorded on this order</span>
                    )}
                  </dd>
                </div>
                {order.payment.vpa && (
                  <div>
                    <dt>UPI</dt>
                    <dd>{order.payment.vpa}</dd>
                  </div>
                )}
                {order.payment.last4 && (
                  <div>
                    <dt>Card</dt>
                    <dd>
                      {order.payment.network ?? 'Card'} ····{order.payment.last4}
                    </dd>
                  </div>
                )}
                <div>
                  <dt>Razorpay order</dt>
                  <dd>
                    <code>{order.razorpayOrderId ?? '—'}</code>
                  </dd>
                </div>
                <div>
                  <dt>Razorpay payment</dt>
                  <dd>
                    <code>{order.razorpayPaymentId ?? '—'}</code>
                  </dd>
                </div>
              </dl>
            ) : (
              <p className="muted">No payment recorded — this order was never paid for.</p>
            )}
          </section>
        </>
      )}
      </main>
    </PageShell>
  );
}
