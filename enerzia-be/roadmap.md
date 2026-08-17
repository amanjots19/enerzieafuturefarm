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

Endpoints marked **admin** take a *different* token, minted by
`POST /admin/login` — see §Admin. Both are HS256 under the same `JWT_SECRET`
but carry a different `iss`, so a shopper token can never open an admin
endpoint and an admin token can never be spent as a shopper. That separation is
a verified claim, not a convention: neither parser accepts the other's issuer.

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
        "images": [
          { "url": "https://res.cloudinary.com/<cloud>/image/upload/v1/enerzia/products/abc.jpg",
            "publicId": "enerzia/products/abc",
            "alt": "Tablet jar, front" }
        ],
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

`images` is **at most five**, in display order — index 0 is the primary shot.
It is `[]` for a product nobody has uploaded a photograph for yet, which is
every product until the admin catalogue is used, so `grad` stays the fallback
rather than being replaced: a client renders `images[0]` when present and the
gradient when not. The array is never `null`.

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

**Every array in every response is `[]` when empty, never `null`** — `images`,
`badges`, `siblings`, `nutrition.rows`, `products`, `tiles`, all of them. This
is not a style preference. A client that writes `rows.map(...)` works against
the nine seeded products, which all have nutrition rows, and then throws the
first time it meets a product created without any — which is exactly how a
`null` here took the admin editor down (tasks.md 10.7). One shape, always.

**404** `NOT_FOUND` — "That product does not exist."

### `GET /api/v1/content/trust`

No auth. The four-tile strip under the shop grid.

**200**
```json
{ "data": { "tiles": [ { "big": "62%+", "body": "Complete plant protein by weight, with all nine essential amino acids." } ] } }
```

---

## Auth

**Sign-in moved to the MSG91 OTP widget on 2026-08-09** (user decision), to
avoid blocking launch on DLT registration. The widget owns the code end to
end — generating it, sending the SMS, retrying, and verifying it. This server
never sees a one-time code.

That removed two endpoints (`/auth/otp/request` and `/auth/otp/verify`), the
`otp_codes` collection, the HMAC-under-pepper hashing, the attempt counter, the
single-use guard and the resend rate limit. All of it now belongs to MSG91.
What stays ours: the `users` collection, the session JWT, and every endpoint
behind it.

> The security reasoning recorded against the old design still stands and is
> worth keeping in mind if sign-in is ever brought back in-house: a 6-digit
> code has a keyspace of only 10^6, so codes must never be stored recoverably.

### `POST /api/v1/auth/session`

No auth. Exchanges an MSG91 widget access token for our session token, creating
the user on first success.

Named for what it does, not for the provider — swapping MSG91 for another
widget later changes the implementation behind this path, not the path itself.

```json
{ "accessToken": "<JWT issued by the MSG91 widget>" }
```

The server verifies the token with MSG91 before trusting anything in it:

```
POST https://api.msg91.com/api/v5/widget/verifyAccessToken
authkey: <MSG91_AUTH_KEY>          ← server-only; never reaches the browser
content-type: application/json
{ "access-token": "<accessToken>" }
```

MSG91 replies `{ "type": "success", "message": "919999999999" }`, where
`message` is the verified identifier.

Two traps, both load-bearing:

1. **Success is signalled by `type`, not by the HTTP status.** MSG91 can answer
   200 with `type: "error"`. Checking the status alone would accept a failed
   verification as a valid sign-in — which is the whole authentication check.
2. **The number comes back with its country code** (`919999999999`), while
   `users.phone` stores 10 digits (`schema.md` §users). Strip the `91` prefix
   and validate the remainder is exactly 10 digits before upserting, or the
   same shopper becomes two accounts.

**200** — identical to what the old verify endpoint returned, so nothing
downstream changes:
```json
{
  "data": {
    "token": "<jwt>",
    "expiresAt": "2026-09-06T10:30:00Z",
    "user": { "id": "665f...", "phone": "9876543210", "createdAt": "2026-08-07T10:30:00Z" }
  }
}
```

**422** `VALIDATION_ERROR` — "Please sign in again." when `accessToken` is
missing or empty.
**401** `UNAUTHORIZED` — "We could not verify that sign-in. Please try again."
for any rejected token, whatever MSG91's reason. One message for every failure
so nothing about the account or the token is enumerable.
**502** `GATEWAY_ERROR` — MSG91 unreachable or timed out. Distinct from 401
because retrying may work and nothing is wrong with the request.

The MSG91 auth key, the access token and MSG91's raw response are never
logged and never returned.

> **Operational constraint: the MSG91 widget must be configured MOBILE ONLY.**
> The widget can also verify by email, and if it does, `message` comes back as
> an email address. `users.phone` is the identity (`schema.md` §users, unique
> index), so there is nothing to key such an account on — the server correctly
> rejects it with the usual 401, but the shopper sees "we could not verify that
> sign-in" *after* MSG91 told them they succeeded, which is baffling and
> unloggable from their side.
>
> This is a setting in MSG91's panel, not in this codebase. Nothing here can
> enforce it and nothing here will explain it, so if sign-in ever starts
> failing for users who swear they verified successfully, **check the widget's
> channel configuration first.**
>
> Changing this would not be a config tweak but an identity-model change,
> touching `users`, the JWT subject and every order's ownership.

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
  "phone": "9876543210",
  "line1": "12, Anand Residency, MG Road",
  "city": "Pune",
  "state": "Maharashtra",
  "pin": "411001",
  "isDefault": true
}
```

`label` is optional and shopper-supplied. Every other field is required and
validated in the order given in `product.md` §3.4, with those exact messages.

`phone` is the **delivery contact for this address**, not the account's
sign-in number. They are usually the same and deliberately separate: a shopper
sending a gift needs the recipient reachable at the door, not themselves. Ten
digits, no `+91`, no spaces — the same shape as the sign-in number, with its
own message so it is obvious which field a 422 is about.

It is **required on every write**, so a shopper editing an older address adds
one. Addresses saved before the field existed have none and stay usable:
`phone` is simply absent on them, and checkout falls back to the account
number rather than blocking a shopper on a field they never saw.

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

**`status` is the payment lifecycle, and only Razorpay moves it.** Fulfilment
progress lives in a **separate `fulfilment` field** that only the admin order
book writes — see §Admin orders. Nothing on a shopper-facing endpoint reads it
yet, so packing a parcel or handing it to a courier changes nothing the shopper
sees. Mapping it into the account area is deliberately a later decision.

`packed`, `shipped`, `delivered` and `cancelled` are still valid `status`
values, and nothing produces any of them. They predate the split; see task
11.9.

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

**This list means "orders you paid for."** Only `placed` and the states after
it — `packed`, `shipped`, `delivered`, `cancelled` — appear. `expired`,
`pending_payment` and `payment_failed` are all omitted.

Changed 2026-08-12 (user decision). Previously only `expired` was excluded, and
an unpaid `pending_payment` order appeared alongside real ones — which produced
a card reading "Total paid ₹449" beside a "Payment pending" badge. Those two
statements contradict each other on the same row, and they do it on a screen a
shopper reads when they are already anxious about money.

An order becomes real when Razorpay confirms payment; before that it is a
checkout attempt, not a purchase. A shopper who abandoned one has lost nothing —
the items are still in their cart, and the reservation expires by itself.

`cancelled` is deliberately still listed: it *was* paid for, and hiding it would
erase a purchase the shopper remembers making.

Consequence for clients: every order returned here has `placedAt`, `etaText`
and a `payment` block, so those fields are safe to read on this endpoint. They
are still optional on `GET /orders/{orderId}`, which returns any status.

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

## Admin — all **admin** except the login itself

The catalogue console. It is the only writer of the `products` collection other
than seeding, and it exists so product copy, pricing, stock and photographs can
be changed without a deploy.

**Admin auth is deliberately not shopper auth.** A shopper proves a phone
number over OTP; an administrator proves an email and password held in the
server's environment. There is no `admins` collection and no sign-up: the
account is configuration, so there is no route by which one can be created,
escalated to, or enumerated at runtime.

| env var | what | notes |
|---|---|---|
| `ADMIN_EMAIL` | the single administrator's email | compared case-insensitively |
| `ADMIN_PASSWORD_HASH` | **bcrypt hash**, never the password | generate with `make admin-password`; the plaintext is never stored, logged, or sent |

Both are required in production. Outside production an empty value leaves admin
sign-in unconfigured, and every login attempt answers exactly as a wrong
password does — a misconfigured server must not be distinguishable from a wrong
guess.

### `POST /api/v1/admin/login`

No auth. Exchanges an email and password for an admin token.

```json
{ "email": "ops@enerzia.in", "password": "…" }
```

**200**
```json
{ "data": { "token": "<jwt>", "expiresAt": "2026-08-13T04:11:07Z", "email": "ops@enerzia.in" } }
```

The token is HS256 under `JWT_SECRET` with `iss: "enerzia-admin"`,
`sub: <email>`, `role: "admin"`, and a **12-hour** TTL — far shorter than the
shopper's 30 days, because it opens a write surface over the whole catalogue.

**422** `VALIDATION_ERROR` for a missing email or password, fields in that
order.

**401** `UNAUTHORIZED` — "Email or password is incorrect." for an unknown
email, a wrong password, **and** for admin sign-in not being configured. One
message for all three: distinguishing them tells an attacker whether the email
they guessed is the right one.

**429** `RATE_LIMITED` — "Too many sign-in attempts. Try again later." after 10
failed attempts from one IP within 15 minutes. The limit is per-IP and counts
only failures, so a wrong guess cannot be used to lock the real administrator
out of their own console.

The password is verified with `bcrypt.CompareHashAndPassword`, whose cost makes
guessing expensive by construction. The plaintext is never logged and never
appears in a response.

### `GET /api/v1/admin/me` — **admin**

**200** `{ "data": { "email": "ops@enerzia.in" } }`

Exists so the console can tell a live token from an expired one on load,
without writing anything.

### The admin product object

Every admin endpoint below returns this shape. It is the stored document plus
the two derived fields, and unlike the shopper's view it exposes `active`,
`badges`, `nutrition` and `rating` on the list as well as the detail — the
console edits them.

```json
{
  "id": "tablets-120",
  "family": "tablets",
  "form": "Tablets",
  "name": "Spirulina Tablets 500 mg — 120 tabs",
  "stat": "No binders, no fillers",
  "stat2": "30 days",
  "blurb": "Pure spirulina pressed into 500 mg tablets...",
  "grad": "radial-gradient(...)",
  "position": 11,
  "images": [ { "url": "https://res.cloudinary.com/…", "publicId": "enerzia/products/abc", "alt": "" } ],
  "mrp": 47000,
  "price": 38000,
  "discountPercent": 19,
  "stock": 100,
  "soldOut": false,
  "active": true,
  "rating": { "score": 4.8, "count": 312 },
  "badges": [ { "title": "Lab tested", "subtitle": "Heavy metals & microbes" } ],
  "nutrition": { "servingSize": "5 g", "rows": [ { "key": "Protein", "value": "3.1 g" } ] }
}
```

`discountPercent` and `soldOut` are computed on the way out and **rejected on
the way in** — a write that carries them is answered 422 rather than silently
ignoring them, so nobody builds a console that believes it is setting a
discount.

**Validation**, applied in this order and returned as ordered `fields` exactly
like §Addresses:

| # | field | rule | message |
|---|---|---|---|
| 1 | id | non-blank, `^[a-z0-9]+(-[a-z0-9]+)*$` | Product id must be lowercase letters, numbers and hyphens. |
| 2 | name | non-blank | Please enter a product name. |
| 3 | family | non-blank | Please enter the family this product belongs to. |
| 4 | form | `Powder` \| `Tablets` \| `Bundle` | Choose a form: Powder, Tablets or Bundle. |
| 5 | mrp | > 0 | MRP must be more than zero. |
| 6 | price | > 0 and ≤ mrp | Price must be more than zero and not above MRP. |
| 7 | stock | ≥ 0 | Stock cannot be negative. |
| 8 | position | ≥ 0 | Position cannot be negative. |
| 9 | images | ≤ 5 | A product can have at most five images. |
| 9b | images | each `url` an absolute `https://` URL | Image links must start with https://. |
| 9c | images | each `publicId` non-blank | Every image needs a Cloudinary public id. |
| 10 | rating.score | 0–5 | Rating must be between 0 and 5. |
| 11 | rating.count | ≥ 0 | Review count cannot be negative. |

Money arrives as **int64 paise**, as everywhere else. A console that shows
rupees converts at its own edge.

### `GET /api/v1/admin/products` — **admin**

**200** `{ "data": { "products": [ … ] } }` — **every** product, ordered by
`position`. Unlike `GET /products`, retired ones are **included**: the console
is where a retired product is brought back, so hiding it there would make that
impossible.

### `POST /api/v1/admin/products` — **admin**

Creates a product. The body is the admin object above; `id` is supplied by the
caller because it appears in URLs and in cart lines.

**201** the created product.

**409** `CONFLICT` — "A product with that id already exists." An id is never
silently reused: it is the join key of every cart line and order line ever
written, so overwriting one would re-point history at a different product.

**422** `VALIDATION_ERROR` per the table above.

### `GET /api/v1/admin/products/{id}` — **admin**

**200** the product, retired or not. **404** `NOT_FOUND`.

### `PUT /api/v1/admin/products/{id}` — **admin**

Replaces every editable field. A body `id` must match the path or be omitted —
a mismatch is **422**, never a rename, because renaming would orphan cart and
order lines.

**200** the updated product. **404** if it does not exist. **422** per the
table.

**`stock` is set absolutely here, not adjusted.** The value written is the
value stored, which is what a stock-take needs. It can therefore race a
checkout in flight: a reservation taken between the console's read and its save
is overwritten. That is accepted — the alternative is a delta API that cannot
express "there are 40 in the room" — but it means stock should be set when the
shop is quiet, and it is why `TakeStock`'s `$gte` guard, not this endpoint, is
what actually prevents overselling.

### `DELETE /api/v1/admin/products/{id}` — **admin**

**Retires** a product: `active: false`. It is not deleted and there is no
endpoint that deletes one — schema.md decision 4 requires that a past order can
still be explained, and a hard delete would leave order lines pointing at
nothing.

**200** the retired product, so the console can re-render it in place. **404**
if it does not exist. Retiring an already-retired product is a no-op that still
returns 200 — the caller asked for a state, and it holds.

To bring a product back, `PUT` it with `"active": true`.

### `POST /api/v1/admin/uploads/signature` — **admin**

Returns the parameters the browser needs to upload one image **directly to
Cloudinary**. The file never passes through this server: it would double the
bandwidth, put arbitrary uploaded bytes in our process, and force a request
timeout long enough to be a denial-of-service lever.

Request body: none.

**200**
```json
{
  "data": {
    "cloudName": "enerzia",
    "apiKey": "3915…",
    "timestamp": 1786608000,
    "folder": "enerzia/products",
    "signature": "9f2b…"
  }
}
```

`signature` is a SHA-1 of the signed parameters (`folder`, `timestamp`) sorted
by key and concatenated with the API secret, as Cloudinary's signed-upload
scheme specifies. **The API secret itself is never returned** — only a
signature over a payload the server chose. `apiKey` and `cloudName` are public
values by Cloudinary's design.

The signature is good for one upload within Cloudinary's validity window
(1 hour). The browser then POSTs the file, `api_key`, `timestamp`, `folder` and
`signature` to
`https://api.cloudinary.com/v1_1/{cloudName}/image/upload`, and keeps
`secure_url` and `public_id` from the response for the product's `images`
array. **Nothing is attached to a product until the product is saved** — an
upload with no save is an orphaned asset in Cloudinary, not a half-written
product.

**500** `INTERNAL` if Cloudinary is not configured. Unlike a failed login this
is *our* fault, not a credential the caller got wrong, and the administrator
who sees it is the person who can fix it.

| env var | what |
|---|---|
| `CLOUDINARY_CLOUD_NAME` | account's cloud name; public |
| `CLOUDINARY_API_KEY` | public key, sent to the browser |
| `CLOUDINARY_API_SECRET` | **server-only**; signs uploads, never leaves the process |
| `CLOUDINARY_FOLDER` | optional, defaults to `enerzia/products` |

Required in production, optional elsewhere — an empty cloud name selects an
unconfigured signer that fails every call, the same shape Razorpay and MSG91
already use.

---

## Admin orders — all **admin**

The order book. Fulfilment is **manual**: there is no courier integration, no
carrier API and no tracking feed. An order's fulfilment state is whatever an
operator last set it to, and that is the honest model of a process a person
carries out by hand.

### The `fulfilment` field

Fulfilment is a **separate field from `status`** (owner decision, 2026-08-17).
The two are orthogonal: `status` records what Razorpay did with the money,
`fulfilment` records what a person did with the parcel. Overloading one field
with both would mean an operator marking a box packed had rewritten the order's
payment state.

```
(absent) ──▶ packed ──▶ in_transit ──▶ shipped
```

| stored | shown in the console | means |
|---|---|---|
| absent | Not started | paid, nothing done to it yet |
| `packed` | Processed | boxed, label printed, waiting for the courier |
| `in_transit` | Transit | handed over, moving |
| `shipped` | Shipped | terminal — out of our hands |

**The shopper sees none of this yet.** No shopper-facing endpoint returns
`fulfilment`, and `GET /api/v1/orders` is unchanged: an order stays "Confirmed"
to its buyer through every fulfilment step. Mapping these states into the
account area is a separate, later decision (task 11.8) — which is the point of
splitting the field rather than reusing `status`, since a shopper's view can
now be designed without an operator's clicks driving it in the meantime.

The progression is strictly linear, and `shipped` is **terminal**. Nothing
tells us a parcel arrived: without a carrier integration a delivered state
could only ever be set on a guess or a phone call, so there is no value for it.

**Absent is a state, not a gap.** An order that has been paid for and not yet
touched has no `fulfilment` field at all, which reads as "Not started". That
also means every order already in the database is correct without a migration.

There is **no cancellation and no refund** (owner decision, 2026-08-17), so
`fulfilment` has no cancelled value either.

### `GET /api/v1/admin/orders` — **admin**

The fulfilment work queue, newest first, every shopper's orders.

| param | default | meaning |
|---|---|---|
| `status` | **environment-dependent — see below** | one status, a comma-separated set, or `all` |
| `fulfilment` | — | `none`, `packed`, `in_transit`, `shipped`, or a comma-separated set |
| `limit` | 50 | 1–200 |
| `before` | — | an RFC3339 `createdAt`; returns orders strictly older |

**The default depends on `APP_ENV`** (owner decision, 2026-08-17):

| `APP_ENV` | omitting `status` returns |
|---|---|
| `production` | the five paid statuses — `placed`, `packed`, `shipped`, `delivered`, `cancelled` |
| anything else | every status, including `pending_payment`, `payment_failed` and `expired` |

**In production the default is the work queue, not the whole table.** An
abandoned checkout is not an order: `pending_payment`, `payment_failed` and
`expired` are attempts, and a book that lists them buries the real work — the
same reasoning, and the same five statuses, as `GET /api/v1/orders`.

**Outside production the default is everything**, because a developer's
question is usually about the attempt that failed rather than the ones that
worked. A reservation that expired or a payment that declined is the thing
being debugged, and having to remember `?status=all` to see it is friction at
exactly the wrong moment.

This splits **only the default**. `?status=all` returns everything in every
environment, and an explicit `?status=…` means the same thing everywhere — so a
console that names what it wants behaves identically in dev and production, and
nothing can be visible in one and silently missing in the other.

**Nothing about the shopper's own list changes.** `GET /api/v1/orders` returns
paid orders only, in every environment. A shopper must never see a checkout
attempt presented as a purchase, and no environment variable relaxes that.

`fulfilment` filters independently of `status`. `?fulfilment=none` is the
queue that actually matters day to day — paid, and nothing done to it yet.
`none` matches orders where the field is absent.

Pagination is a **cursor on `createdAt`**, not a page number. Orders arrive
while the book is being read, and an offset would silently duplicate or skip a
row when that happens. On this screen that is an order shipped twice, or never.

**200**
```json
{
  "data": {
    "orders": [ { "…": "the admin order object" } ],
    "nextBefore": "2026-08-14T09:12:04Z",
    "count": 50
  }
}
```

`nextBefore` is absent on the last page. `count` is how many were returned, not
a total — a total over an unbounded collection costs a full scan on every page.

**422** `VALIDATION_ERROR` for an unknown status, a `limit` outside 1–200, or an
unparseable `before`.

### `GET /api/v1/admin/orders/{orderId}` — **admin**

**200** the admin order object. **404** `NOT_FOUND` if no such order.

Unlike the shopper's endpoint, a 404 here means only "no such order" — there is
no ownership dimension to leak.

### The admin order object

The shopper's order object plus what an operator needs and a shopper must never
see:

```json
{
  "orderId": "EFF-483413",
  "status": "placed",
  "statusLabel": "Placed",
  "fulfilment": "packed",
  "fulfilmentLabel": "Processed",
  "createdAt": "2026-08-07T10:32:11Z",
  "placedAt": "2026-08-07T10:33:48Z",
  "lines": [ { "…": "unchanged" } ],
  "totals": { "…": "unchanged" },
  "shippingAddress": { "…": "unchanged" },
  "payment": { "…": "unchanged" },
  "customer": { "userId": "6a750011ac80d0a1ecb861f", "phone": "9876543210" },
  "razorpayOrderId": "order_PkX9aQ…",
  "razorpayPaymentId": "pay_PkXB7z…"
}
```

`razorpaySignature` is **not** exposed. It is kept for audit and has no
operational use; a console has no reason to hold an HMAC.

`customer.phone` is `null` on orders created before the field existed.
`fulfilment` is `null`, and `fulfilmentLabel` is `"Not started"`, on an order
nobody has touched yet.

### `PATCH /api/v1/admin/orders/{orderId}/fulfilment` — **admin**

```json
{ "fulfilment": "in_transit" }
```

**200** the updated admin order object.

**409** `CONFLICT` for any move not on the diagram above. The message names
what the operator *can* do, because "no" on its own leaves them guessing which
button was right:

| case | message |
|---|---|
| skipping a step | "This order is Not started. The next step is Processed, not Shipped." |
| already there | "This order is already Processed." |
| past the end | "This order is already Shipped, which is the last step." |
| not paid for | "Only a placed order can be fulfilled. This one is Expired." |
| someone else moved it | "Somebody else changed this order just now. Reload and try again." |

Backwards moves and skips are both rejected — jumping straight to `shipped`
skips the step at which a label gets printed. Re-sending the state an order is
already in is also a 409, not a silent success: a no-op that answers 200 tells
an operator a parcel moved when nothing did.

The last row is a **guarded write**, not a re-read. The update filters on the
fulfilment state the server just read, so two operators clicking at once cannot
both advance the order and skip a step between them. The loser gets that
message rather than a false success.

**422** `VALIDATION_ERROR` for a missing `fulfilment`, or a value that is not a
fulfilment state at all — including `none`, which is a state an order can be
in but never a state you can move it *to*. The split is deliberate: 422 means
"there is no such state", 409 means "you cannot go there from here", and only
the second is something an operator can act on.

**400** `BAD_REQUEST` for a malformed body or an unknown field.

**404** if the order does not exist, or if the id is not `EFF-######` — a
malformed id cannot name a real order, so it is answered without a query.

A fulfilment change writes **only** `fulfilment` and `updatedAt`. It moves no
stock, no money, and — deliberately — not `status`. The only path that returns
stock is the reservation sweeper, unchanged.

### `GET /api/v1/admin/orders/{orderId}/label` — **admin**

A single 4×6 shipping label as `text/html`, ready to print. The console opens
it in a tab and the browser's print dialog does the rest.

**HTML rather than PDF** because `html/template` is stdlib: a PDF library would
be the first new dependency the module has taken on since it was started. The
browser owns pagination and printing, and the handler is testable by asserting
the rendered markup carries the right address and order id. If batch printing
ever justifies a PDF it is one handler to swap — the data shape survives.

`@page { size: 4in 6in; margin: 0 }`. Monochrome, with rules and weight rather
than filled panels: a thermal head prints by burning, and large solid areas
smear and shorten its life.

Down the label: **TO** (recipient, street, city, state, PIN, phone — the
largest block, because it is the thing a courier actually reads), **FROM** (the
configured origin), the order id and date, the line items as
`2 × Pure Spirulina Powder - 100 g` so whoever packs can check the box before
sealing it, and a boxed **`PREPAID — DO NOT COLLECT CASH`**. That last one is
not decoration: there is no COD in this shop, and a courier who assumes
otherwise will try to collect from the shopper at their door.

No barcode. A courier generates its own tracking barcode, and a Code128 of an
`EFF-` id would scan to something no one's system knows.

**409** `CONFLICT` for an order in `pending_payment`, `payment_failed` or
`expired`. Printing a label for an order nobody paid for is how a parcel goes
out for free.

**503** `UNAVAILABLE` — "Shipping origin is not configured." when the origin
below is unset. Deliberately a failure rather than a label with blanks where
the return address belongs: a parcel that can be neither delivered nor returned
is simply gone, along with what is in it.

`UNAVAILABLE` is its own code rather than `INTERNAL` so an operator can tell
"nobody set this up" from "something crashed" — the first has a fix they can
apply themselves.

The six variables are **all-or-nothing, checked at startup**. Setting some but
not all fails the boot and names each missing one; so does a `SHIP_FROM_PIN`
that is not six digits or a `SHIP_FROM_PHONE` that is not ten. A half-filled
origin would otherwise print a label missing exactly the line that gets an
undelivered parcel home, and would do it silently. Setting none is not an
error — the endpoint simply 503s and nothing else is affected.

**Quote any value containing spaces.** `make run` sources `.env` with `.`, so
an unquoted `SHIP_FROM_NAME=Enerzeia Future Farm` runs `Future` as a command
and the variable arrives empty.

**404** if the order does not exist.

| env var | what |
|---|---|
| `SHIP_FROM_NAME` | the name a returned parcel comes back to |
| `SHIP_FROM_LINE1` | street |
| `SHIP_FROM_CITY` | city |
| `SHIP_FROM_STATE` | state |
| `SHIP_FROM_PIN` | six digits, validated at startup |
| `SHIP_FROM_PHONE` | ten digits, the number a courier calls about a pickup |

Required for the label endpoint only. Unset, the endpoint 503s and the rest of
the API is unaffected — the same posture Cloudinary, Razorpay and MSG91 already
use for a missing credential.

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
| **7 — Frontend integration** | wiring the storefront to these endpoints; no new API surface |
| **8 — MSG91 sign-in** | `POST /auth/session`; the in-house OTP endpoints and collection removed |
| **9 — Account area** | order history and saved addresses in the storefront; no new API surface |
| **10 — Admin catalogue** | admin login, product CRUD, `products.images`, Cloudinary upload signing |
| **11 — Order book & labels** | `orders.fulfilment` (separate from `status`), `orders.customerPhone`, admin order list/detail/fulfilment, 4×6 HTML shipping label |

Later, explicitly not now: real SMS, refunds, order cancellation, coupons,
stock movement history, and any courier or logistics integration.
