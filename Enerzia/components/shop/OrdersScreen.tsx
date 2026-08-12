'use client';

import { useState } from 'react';

import { addressDTOSummary, rupeeFromPaise } from '@/lib/shop/pricing';
import type { ShopActions } from '@/app/shop/useShop';
import type { ShopAction } from '@/lib/shop/reducer';
import type { OrderDTO, ShopState } from '@/lib/shop/types';

const STATUS_LABEL: Record<string, string> = {
  pending_payment: 'Payment pending',
  placed: 'Confirmed',
  payment_failed: 'Payment failed',
  packed: 'Packed',
  shipped: 'Shipped',
  delivered: 'Delivered',
  cancelled: 'Cancelled',
};

function statusLabel(s: string): string {
  return STATUS_LABEL[s] ?? s;
}

function statusClass(s: string): string {
  if (s === 'placed' || s === 'packed' || s === 'shipped' || s === 'delivered')
    return 'order-status--confirmed';
  if (s === 'pending_payment') return 'order-status--pending';
  if (s === 'payment_failed') return 'order-status--failed';
  return 'order-status--muted';
}

function shortId(orderId: string): string {
  return '#' + orderId.slice(-8).toUpperCase();
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-IN', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  });
}

function OrderRow({
  order,
  expanded,
  onToggle,
}: {
  order: OrderDTO;
  expanded: boolean;
  onToggle: () => void;
}) {
  const detailId = `order-detail-${order.orderId}`;

  return (
    <div className="order-row">
      <button
        type="button"
        className="order-summary"
        aria-expanded={expanded}
        aria-controls={detailId}
        onClick={onToggle}
      >
        <span className="order-summary-id">{shortId(order.orderId)}</span>
        <span className="order-summary-date">{formatDate(order.createdAt)}</span>
        <span className="order-summary-total">{rupeeFromPaise(order.totals.total)}</span>
        <span className={`order-status ${statusClass(order.status)}`}>
          {statusLabel(order.status)}
        </span>
        <span className="order-chevron" aria-hidden="true">
          {expanded ? '▴' : '▾'}
        </span>
      </button>

      {expanded && (
        <div id={detailId} className="order-detail">
          {/* Lines */}
          <div className="order-detail-section">
            <div className="order-detail-label">Items</div>
            <div className="mini-lines">
              {order.lines.map((line) => (
                <div className="mini-line" key={line.productId}>
                  <span className="order-line-name">
                    {line.name}
                    <span className="order-line-meta">
                      {' '}· {line.form}{line.grad ? `, ${line.grad}` : ''} × {line.qty}
                    </span>
                  </span>
                  <span>{rupeeFromPaise(line.lineTotal)}</span>
                </div>
              ))}
            </div>
            <div className="order-totals">
              {order.totals.savings > 0 && (
                <div className="mini-line">
                  <span className="muted">Discount</span>
                  <span className="save">− {rupeeFromPaise(order.totals.savings)}</span>
                </div>
              )}
              <div className="mini-line">
                <span className="muted">Delivery</span>
                <span>{order.totals.shipping === 0 ? 'Free' : rupeeFromPaise(order.totals.shipping)}</span>
              </div>
              <div className="mini-line order-grand-total">
                <span>Total paid</span>
                <span>{rupeeFromPaise(order.totals.total)}</span>
              </div>
            </div>
          </div>

          {/* Shipping address */}
          <div className="order-detail-section">
            <div className="order-detail-label">Delivered to</div>
            <div className="addr-summary">
              <strong>{order.shippingAddress.name}</strong>
              <br />
              {addressDTOSummary(order.shippingAddress)}
            </div>
          </div>

          {/* Payment + ETA — only present when status === 'placed' */}
          {order.payment && (
            <div className="order-detail-section">
              <div className="order-detail-label">Payment</div>
              <div>{order.payment.label ?? 'Paid online'}</div>
            </div>
          )}
          {order.etaText && (
            <div className="order-detail-section">
              <div className="order-detail-label">Expected delivery</div>
              <div>{order.etaText}</div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function OrdersScreen({
  state,
  dispatch,
}: {
  state: ShopState;
  dispatch: (a: ShopAction) => void;
  actions: ShopActions;
}) {
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const loading = state.pending.orders;
  const orders = state.orders;

  function toggle(orderId: string) {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      next.has(orderId) ? next.delete(orderId) : next.add(orderId);
      return next;
    });
  }

  return (
    <div className="wrap-md screen">
      <div className="orders-header">
        <h1 className="page-title">My orders</h1>
        <button
          type="button"
          className="btn btn-ghost"
          onClick={() => dispatch({ type: 'go', screen: 'shop' })}
        >
          ← Back to shop
        </button>
      </div>

      {loading && orders.length === 0 && (
        <div className="orders-loading">Loading your orders…</div>
      )}

      {!loading && orders.length === 0 && (
        <div className="empty">
          <p>No orders yet.</p>
          <button
            className="btn btn-primary"
            type="button"
            style={{ padding: '12px 24px' }}
            onClick={() => dispatch({ type: 'go', screen: 'shop' })}
          >
            Browse products
          </button>
        </div>
      )}

      {orders.length > 0 && (
        <div className="orders-list" role="list">
          {orders.map((order) => (
            <div role="listitem" key={order.orderId}>
              <OrderRow
                order={order}
                expanded={expandedIds.has(order.orderId)}
                onToggle={() => toggle(order.orderId)}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
