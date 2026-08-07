# Enerzia backend — API roadmap

The contract. Every endpoint's method, path, auth, request and response lives
here, and handler behaviour must match it exactly. If an implementation and
this file disagree, that is a bug — raise it rather than quietly following the
code.

Adding an endpoint? Define it here **first**, then build it.

- Base URL: `/api/v1` (health is outside the version prefix)
- Content type: `application/json; charset=utf-8` on every request and response
- Stack: Go 1.25 · gorilla/mux · MongoDB Atlas

---

## Conventions

### Envelope

Success — always an object with a single `data` key:

```json
{ "data": { "...": "..." } }
```

Error — always an object with a single `error` key:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Please enter a valid email for order updates.",
    "details": { "fields": [{ "field": "email", "message": "Please enter a valid email for order updates." }] }
  }
}
```

`message` is safe to show a shopper. `details` is optional. Driver, database
and stack details never appear in a response — they are logged instead.

### Error codes

| code | HTTP | when |
|---|---|---|
| `VALIDATION_ERROR` | 422 | payload parsed but failed a domain rule |
| `BAD_REQUEST` | 400 | malformed JSON, bad path/query param |
| `UNAUTHORIZED` | 401 | missing, malformed or expired token |
| `NOT_FOUND` | 404 | unknown route or resource |
| `CONFLICT` | 409 | state prevents the action (e.g. ordering an empty cart) |
| `RATE_LIMITED` | 429 | too many OTP requests |
| `INTERNAL` | 500 | anything unhandled |
| `GATEWAY_ERROR` | 502 | Razorpay is unreachable or rejected a request of ours |

`GATEWAY_ERROR` is deliberately distinct from `INTERNAL`: it tells the client
that nothing is wrong with the request and retrying may work, which is exactly
what a shopper staring at a failed checkout needs to know. The upstream error
detail is logged, never returned.

Validation failures are returned in the **same order** the frontend evaluates
them (see `product.md` §3.4), so a client that shows only `fields[0]` matches
the existing UI exactly.

### Money

Every monetary value is **`int64` paise**. ₹499 is `49900`, ₹1,030 is `103000`.
No floats anywhere, in JSON or in Go. Fields carrying money are suffixed
consistently (`unitPrice`, `subtotal`, `shipping`…) and documented below.

> Frontend note: `lib/shop/data.ts` currently stores whole rupees. When wiring
> up, divide by 100 at the API boundary — do not change `rupee()`, which
> already formats `en-IN` correctly.

### Auth

`Authorization: Bearer <jwt>`. Tokens are HS256, carry `sub` (user id) and
`phone`, and expire in 30 days. Endpoints marked **auth** return 401 without a
valid one.

### Identifiers

| thing | id | example |
|---|---|---|
| product (a sellable item) | human-readable slug | `tablets-120` |
| cart line | the `productId` it holds | `tablets-120` |
| address | ObjectId hex | `6a750011ac80d0a1ecb861f` |
| order | `EFF-` + six digits | `EFF-483413` |

There is no variant id: each size **is** a product. A cart line is its product,
so `DELETE /cart/items/tablets-120` needs no encoding.

---

## Health

### `GET /health`

No auth. Used by deploys and uptime checks.

**200**
```json
{ "data": { "status": "ok", "version": "0.1.0", "uptimeSeconds": 1042, "mongo": "up" } }
```

Returns **503** with `"status": "degraded"`, `"mongo": "down"` if the database
ping fails. Always returns a body — never an empty 200.

---

## Catalogue

### `GET /api/v1/products`

No auth. The shop grid.

| query | type | notes |
|---|---|---|
| `form` | `Powder` \| `Tablets` \| `Bundle` | optional; omitted or `all` returns everything. Unknown value → 400 |

**200**
```json
{
  "data": {
    "products": [
      {
        "id": "tablets-120",
        "family": "tablets",
        "form": "Tablets",
        "name": "Spirulina Tablets 500 mg — 120 tabs",
        "stat": "No binders, no fillers",
        "stat2": "30 days",
        "blurb": "Pure spirulina pressed into 500 mg tablets...",
        "grad": "radial-gradient(...)",
        "mrp": 47000,
        "price": 38000,
        "discountPercent": 19,
        "stock": 100,
        "soldOut": false
      }
    ]
  }
}
```

Nine products today — every size is its own entry, so the grid renders one card
per row with no size selector.

`discountPercent` is computed server-side as `round((1 − price/mrp) × 100)` so
the client never derives it. `soldOut` is derived from `stock <= 0`, never
stored.

`family` groups siblings — the three tablet sizes all carry `"tablets"`. It is
for display only; nothing about buying depends on it. Retired products
(`active: false`) are omitted from every listing but stay resolvable by id, so
an old order can still be explained.

Results are ordered by `position`, which is global across the grid, so siblings
sit together without the client sorting.

### `GET /api/v1/products/{id}`

No auth. The PDP. Same product object plus its detail content.

**200**
```json
{
  "data": {
    "product": { "...": "same shape as the list entry" },
    "siblings": [
      { "id": "tablets-60", "name": "Spirulina Tablets 500 mg — 60 tabs", "price": 20000, "soldOut": false }
    ],
    "rating": { "score": 4.8, "count": 312 },
    "badges": [
      { "title": "Lab tested", "subtitle": "Heavy metals & microbes" },
      { "title": "FSSAI licensed", "subtitle": "Lic. 100xxxxxxxxxxx" },
      { "title": "No binders", "subtitle": "100% spirulina" },
      { "title": "Free delivery", "subtitle": "On orders over ₹499" }
    ],
    "nutrition": {
      "servingSize": "5 g",
      "rows": [ { "key": "Protein", "value": "3.1 g" } ]
    }
  }
}
```

`siblings` are the other products in the same `family`, so a detail page can
offer "also available in 60 tabs" without the client knowing how families work.
Empty for a product with no siblings.

**404** `NOT_FOUND` — "That product does not exist."

### `GET /api/v1/content/trust`

No auth. The four-tile strip under the shop grid.

**200**
```json
{ "data": { "tiles": [ { "big": "60%+", "body": "Complete plant protein by weight, with all nine essential amino acids." } ] } }
```

---

## Auth

### `POST /api/v1/auth/otp/request`

No auth. Starts sign-in.

```json
{ "phone": "9876543210" }
```

`phone` is exactly 10 digits, no `+91`, no spaces. Anything else → **422**
`VALIDATION_ERROR`, "Enter a valid 10-digit mobile number".

**202**
```json
{ "data": { "expiresInSeconds": 300, "resendAfterSeconds": 30, "devCode": "123456" } }
```

`devCode` is returned **only** when `APP_ENV != "production"`, so the flow is
testable without an SMS provider. It must never appear in production responses
and must never be logged.

Rate limit: 3 requests per phone per 10 minutes, and one per 30 seconds →
**429** `RATE_LIMITED`, "Too many attempts. Try again in a minute."

### `POST /api/v1/auth/otp/verify`

No auth. Completes sign-in, creating the user on first success.

```json
{ "phone": "9876543210", "code": "123456" }
```

**200**
```json
{
  "data": {
    "token": "<jwt>",
    "expiresAt": "2026-09-06T10:30:00Z",
    "user": { "id": "665f...", "phone": "9876543210", "createdAt": "2026-08-07T10:30:00Z" }
  }
}
```

**422** `VALIDATION_ERROR` — "Enter the 6-digit code" when the shape is wrong.
**401** `UNAUTHORIZED` — "That code is not correct or has expired." for a wrong
or stale code. Codes are single-use, expire after 5 minutes, and are locked out
after 5 failed attempts. Codes are stored **hashed**, never in plaintext.

> This closes the frontend gap noted in `product.md` §3.3, where any six digits
> were accepted.

### `GET /api/v1/auth/me` — **auth**

**200** — `{ "data": { "user": { "id": "...", "phone": "...", "createdAt": "..." } } }`

---

## Cart — all **auth**

One cart per user, created lazily on first write. Prices are resolved from the
catalogue on every read, so a price change is reflected immediately and a
client can never set its own.

Shared cart object:

```json
{
  "lines": [
    {
      "productId": "tablets-120",
      "name": "Spirulina Tablets 500 mg — 120 tabs",
      "form": "Tablets",
      "grad": "radial-gradient(...)",
      "unitPrice": 38000,
      "unitMrp": 47000,
      "qty": 3,
      "lineTotal": 114000,
      "stock": 40,
      "soldOut": false
    }
  ],
  "totals": { "mrpTotal": 141000, "subtotal": 114000, "savings": 27000, "shipping": 0, "total": 114000 },
  "freeShipping": { "thresholdAmount": 49900, "qualified": true, "remainingAmount": 0 },
  "itemCount": 3,
  "hasBlockingIssues": false
}
```

`itemCount` is the sum of quantities — it drives the header badge.

`soldOut` on a line means the product ran out **after** it was added.
`hasBlockingIssues` is true when any line is sold out or exceeds available
stock; checkout refuses while it is true. The line is not silently removed —
the shopper should see what went wrong.

### `GET /api/v1/cart`
**200** the cart object. An empty cart is **200** with `lines: []`, never 404.

### `POST /api/v1/cart/items`
```json
{ "productId": "tablets-120", "qty": 1 }
```
`qty` ≥ 1 (default 1 if omitted), ≤ 99. Adding a product already present
**increments** it. **200** with the updated cart.

**422** for an unknown or retired `productId`, `qty` out of range, or a product
with no stock — a sold-out item cannot be added in the first place.

### `PATCH /api/v1/cart/items/{productId}`
```json
{ "qty": 2 }
```
Absolute set, not a delta. `qty: 0` deletes the line. **200** with the cart.
**404** if the line is not in the cart. **422** if `qty` exceeds stock.

### `DELETE /api/v1/cart/items/{productId}`
**200** with the cart. **404** if absent.

### `DELETE /api/v1/cart`
Empties it. **200** with the now-empty cart.

## Addresses — all **auth**

A shopper may save several delivery addresses. Exactly one is the default.

Address object:

```json
{
  "id": "6a750011ac80d0a1ecb861f",
  "label": "Home",
  "name": "Ananya Sharma",
  "email": "ananya@example.com",
  "line1": "12, Anand Residency, MG Road",
  "city": "Pune",
  "state": "Maharashtra",
  "pin": "411001",
  "isDefault": true
}
```

`label` is optional and shopper-supplied. Every other field is required and
validated in the order given in `product.md` §3.4, with those exact messages.

### `GET /api/v1/me/addresses`
**200** `{ "data": { "addresses": [ ... ] } }`, default first then newest.
A shopper with none gets `[]`, not a 404.

### `POST /api/v1/me/addresses`
Body is the address without `id`. **201** with the created address.
The **first** address saved becomes the default automatically.
Pass `"isDefault": true` to make a later one the default; the flag is cleared
on all others in the same write.

### `PUT /api/v1/me/addresses/{addressId}`
Full replacement. **200** with the updated address. **404** if it is not the
caller's. Clearing `isDefault` on the only default is ignored — a shopper with
addresses always has one selected.

### `DELETE /api/v1/me/addresses/{addressId}`
**200** `{ "data": { "addresses": [ ... ] } }` — the remaining list.
**404** if it is not the caller's. Deleting the default promotes the next one.

Deleting an address **never** alters a placed order: orders carry a copy, not a
reference (`schema.md` decision 3).

## Orders — all **auth** except the webhook

Payment is collected by **Razorpay Checkout**. There is no cash on delivery, and
no payment instrument is ever posted to this API — the shopper picks UPI, card,
netbanking or a wallet inside Razorpay's modal. See `schema.md` §Razorpay for
the flow, the two secrets and the signature rules.

An order therefore exists **before** it is paid for. `POST /orders` creates it
in `pending_payment` holding a stock reservation; a verified callback or
webhook moves it to `placed`.

`status` is one of `pending_payment`, `placed`, `payment_failed`, `expired`,
`packed`, `shipped`, `delivered`, `cancelled`.

### `POST /api/v1/orders`

Opens a checkout. The client sends **no prices, no line items and no payment
details** — only which address to ship to.

```json
{ "addressId": "6a750011ac80d0a1ecb861f" }
```

`addressId` is optional; the shopper's default is used when it is omitted. The
body may be empty.

Preconditions, checked in this order:

1. cart non-empty → else **409** `CONFLICT`, "Your cart is empty."
2. every line still in stock → else **409** `CONFLICT`, naming the line
3. the address exists and belongs to the caller → else **404**; no saved
   address at all → **422**
4. any existing `pending_payment` order for this shopper is expired first and
   its stock returned, so a re-opened checkout cannot hold two reservations
5. stock is reserved per product with the guard in the filter
   (`schema.md` §products). If any decrement reports zero modified documents,
   someone took the last units between the check and the write: already-applied
   decrements are restored and the request fails **409** rather than overselling
6. a Razorpay order is created for exactly `totals.total` → on gateway failure
   the reservation is released and the request fails **502** `GATEWAY_ERROR`

**201**
```json
{
  "data": {
    "order": {
      "orderId": "EFF-483413",
      "status": "pending_payment",
      "createdAt": "2026-08-07T10:32:11Z",
      "expiresAt": "2026-08-07T10:47:11Z",
      "lines": [ { "...": "cart line shape, frozen at creation" } ],
      "totals": { "mrpTotal": 141000, "subtotal": 114000, "savings": 27000, "shipping": 0, "total": 114000 },
      "shippingAddress": { "...": "address at time of order" }
    },
    "razorpay": {
      "keyId": "rzp_test_XXXXXXXXXXXX",
      "razorpayOrderId": "order_PkX9aQ...",
      "amount": 114000,
      "currency": "INR"
    }
  }
}
```

`keyId` is Razorpay's **public** key and is meant to reach the browser; the key
secret and the webhook secret never leave the server.

`amount` is paise and equals `totals.total`, computed server-side. The client
cannot propose an amount.

The cart is **not** emptied here — only when payment is captured. A declined
card must not also cost the shopper their basket.

### `POST /api/v1/orders/{orderId}/payment/callback`

The browser's fast path: what Razorpay Checkout handed back on success.

```json
{
  "razorpayOrderId": "order_PkX9aQ...",
  "razorpayPaymentId": "pay_PkXB7z...",
  "razorpaySignature": "9f2b3c..."
}
```

The signature is verified as `HMAC_SHA256(razorpayOrderId + "|" +
razorpayPaymentId, RAZORPAY_KEY_SECRET)`, compared in constant time.

**200** the order, now `placed`, in the same shape as `GET /orders/{orderId}`.

**422** `VALIDATION_ERROR` — "We could not verify that payment." for a signature
that does not match, an id belonging to a different order, or an amount
mismatch. One message for all three: which check failed is a security detail,
so it is logged rather than returned.

**404** if the order is not the caller's.

Calling this twice is safe. If the webhook already moved the order to `placed`,
the second call is a no-op that still returns **200** with the order.

### `POST /webhooks/razorpay` — **no auth, signature-verified**

Razorpay's authoritative notification. Deliberately outside the `/api/v1`
prefix: it is not part of the shopper-facing API, and its request shape belongs
to Razorpay rather than to us.

Verified as `HMAC_SHA256(<raw body bytes>, RAZORPAY_WEBHOOK_SECRET)` against the
`X-Razorpay-Signature` header, **before the body is parsed**. Deduplicated on
the `X-Razorpay-Event-Id` header via `payment_events`.

Handled: `payment.captured` → `placed`, and the cart is emptied;
`payment.failed` → `payment_failed`, and the reservation is released;
`order.paid` → treated as a duplicate of capture.

**200** `{ "data": { "received": true } }` — returned once the signature
verifies, including for events we ignore and events already applied. A non-2xx
tells Razorpay to retry, so reporting failure on a successfully deduped event
would have it redelivered indefinitely.

**400** `BAD_REQUEST` for a missing or invalid signature. Nothing further about
the failure is returned.

### `GET /api/v1/orders`
**200** `{ "data": { "orders": [ ... ] } }`, newest first, caller's orders only.
`expired` orders are omitted — nothing happened, and they would read as failed
purchases the shopper never made. Everything else, including `pending_payment`
and `payment_failed`, is listed.

### `GET /api/v1/orders/{orderId}`
**200** the order. **404** if it does not exist **or** belongs to another user —
never distinguish the two, or the endpoint leaks which ids are real.

A `placed` order also carries `placedAt`, `etaText` ("in 3–5 days") and its
`payment` object:

```json
{ "method": "upi", "label": "UPI", "last4": null, "network": null, "vpa": "ananya@okaxis" }
```

`method` and its detail fields are whatever Razorpay reported, and are `null`
while an order is still `pending_payment`. `etaText` is computed at response
time, never stored.

`orderId` matches `^EFF-\d{6}$` and is unique — on collision the server retries
rather than returning a duplicate. Lines, totals and the shipping address are
**snapshots**: a later price change, a renamed product or a deleted address
must not alter a placed order.

---

## Cross-cutting

- **CORS**: `ALLOWED_ORIGINS` (comma-separated); the Next.js dev origin during
  development. Credentials not used — the token goes in a header.
- **Request id**: every request gets one, echoed as `X-Request-Id` and attached
  to logs.
- **Logging**: structured (`log/slog`), one line per request with method, path,
  status, duration, request id. Never log tokens, OTP codes, card details or
  the Mongo URI.
- **Panic recovery**: middleware converts a panic into a 500 `INTERNAL` and
  logs the stack.
- **Timeouts**: 10s read, 15s write on the HTTP server; 5s on Mongo operations.
- **Graceful shutdown** on SIGINT/SIGTERM with a 15s drain.
- **Unknown route / method** → 404 / 405 in the standard error envelope, not
  gorilla's plain-text default.

---

## Delivery phases

Endpoints grouped into shippable slices. `tasks.md` breaks these into units.

| phase | contents |
|---|---|
| **0 — Foundation** | module, config, Mongo connection, router, middleware, `/health`, Makefile, CI-able lint + coverage |
| **1 — Catalogue** | products list + detail, trust content, seeding |
| **2 — Auth** | OTP request/verify, JWT, `me` |
| **3 — Cart** | cart CRUD with server-side pricing |
| **4 — Addresses** | multiple addresses per user with ordered validation |
| **5 — Orders & payment** | Razorpay gateway, open checkout incl. guarded stock reservation, callback + webhook verification, reservation sweeper, list, detail |
| **6 — Hardening** | rate limiting, request ids, structured logs, graceful shutdown |

Later, explicitly not now: real SMS, refunds, coupons, admin, stock movement
history.
