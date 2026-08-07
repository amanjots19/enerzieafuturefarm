# MongoDB schema

Collections, document shapes, indexes and the modelling decisions behind them.
`roadmap.md` is the wire contract; this is what sits on disk. Where they differ
(they do, deliberately) it is called out below.

Database: `MONGO_DB` (default `enerzia`). All timestamps are UTC `date`. All
money is BSON `long` (int64) **paise** — never `double`, never rupees.

---

## Modelling decisions

Six choices shape everything below. Each exists to stop a specific bug.

**1. A product *is* a sellable item.** There is no separate variant entity.
"Spirulina Tablets 120 tabs" and "Spirulina Tablets 300 tabs" are two products
with their own id, price, stock, copy and detail page — each gets its own card
in the shop grid. `family` groups siblings for display, but nothing depends on
it: deleting the grouping would lose a cross-link, not break a purchase.

**2. Stock lives on the product** and is decremented with the guard inside the
filter. One document per sellable thing means one atomic write per sale, with
no positional array operators.

**3. The cart stores no prices.** A cart line is a `productId` and a quantity.
Prices are resolved on every read, so a client cannot post a price and a cart
left open overnight cannot charge yesterday's total.

**4. Orders snapshot everything — and this is not duplication to remove.**
An order freezes its lines, prices, totals and shipping address at purchase
time. Referencing a `productId` alone would mean tomorrow's price change
rewrites what a customer was charged last week, and editing an address would
alter a past order's shipping record.

**5. Addresses are an array embedded on the user**, each with its own `_id`.
A shopper can save several. They are not a separate collection because they are
only ever read with their owner and there are a handful — a collection would
add a join to every checkout for nothing.

**6. An order exists before it is paid for, and holds stock while it waits.**
Razorpay collects payment *after* we have created the order, so `orders` has a
`pending_payment` state that owns a stock reservation and an expiry. The
alternative — decrementing only once payment is confirmed — lets two shoppers
pay for the last unit, and the only remedy is a refund. Holding stock costs an
abandoned checkout a few minutes of inventory; not holding it costs a customer
their order after their money has already moved.

---

## `products`

One document per **sellable item**. Nine of them today. Read constantly,
written by seeding and by stock decrements.

```js
{
  _id: "tablets-120",                // human-readable, appears in URLs
  family: "tablets",                 // groups siblings for display only
  form: "Tablets",                   // "Powder" | "Tablets" | "Bundle"
  name: "Spirulina Tablets 500 mg — 120 tabs",
  stat:  "No binders, no fillers",
  stat2: "30 days",
  blurb: "Pure spirulina pressed into 500 mg tablets...",
  grad:  "radial-gradient(...)",     // stands in for a product photo
  position: 11,                      // display order across the whole grid

  mrp:   NumberLong(47000),          // paise
  price: NumberLong(38000),          // paise
  stock: 100,                        // units on hand; 0 means sold out
  active: true,                      // false retires it without losing history

  rating:    { score: 4.8, count: 312 },
  badges:    [ { title: "Lab tested", subtitle: "Heavy metals & microbes" } ],
  nutrition: { servingSize: "5 g", rows: [ { key: "Protein", value: "3.1 g" } ] }
}
```

| index | fields | why |
|---|---|---|
| `_id_` | `_id` | implicit; product detail lookup |
| `form_1_position_1` | `form`, `position` | the shop grid's form filter, already ordered |
| `family_1_position_1` | `family`, `position` | list a product's siblings without a scan |

**Sold out is derived, never stored**: `stock <= 0`. A separate boolean would be
a second source of truth that can disagree with the number beside it.

**Stock is decremented at order placement**, with the guard inside the filter:

```js
updateOne({ _id: productId, stock: { $gte: n } }, { $inc: { stock: -n } })
```

`modifiedCount == 0` means someone else took the last units. The `$gte` clause
is the whole point — `$inc` alone is atomic but still drives stock negative
under contention.

**`active: false` retires a product** rather than deleting it, so a past order
referencing it can still be explained. Retired products are excluded from the
catalogue but remain resolvable by id.

**Detail content is per product**, so two sizes of the same thing can carry
different nutrition or badges. Today they are seeded identically; diverging is
an edit, not a migration.

## `content`

Small singleton documents keyed by a name, for copy that belongs to no product.

```js
{
  _id: "trust",
  tiles: [
    { big: "60%+", body: "Complete plant protein by weight, ..." }
  ]
}
```

Index: `_id` only. One document today (`trust`); the pattern leaves room for
banners or FAQ blocks without a collection each.

## `users`

Created on first successful OTP verification. The phone number *is* the
identity — there is no password and no email login.

```js
{
  _id: ObjectId("..."),
  phone: "9876543210",               // 10 digits, no +91, no spaces
  addresses: [                       // [] until the shopper saves one
    {
      _id:   ObjectId("..."),        // referenceable, editable, selectable
      label: "Home",                 // optional, shopper-supplied
      name:  "Ananya Sharma",
      email: "ananya@example.com",
      line1: "12, Anand Residency, MG Road",
      city:  "Pune",
      state: "Maharashtra",
      pin:   "411001",
      isDefault: true
    }
  ],
  createdAt: ISODate("..."),
  updatedAt: ISODate("...")
}
```

| index | fields | why |
|---|---|---|
| `phone_1` | `phone`, **unique** | one account per number; makes the verify-time upsert race-safe |

**Addresses are an array, not a collection** — decision 5. Each entry carries
its own `_id`, so the API can address one directly
(`PUT /me/addresses/{addressId}`) without positional index games.

**Exactly one entry may have `isDefault: true`.** MongoDB cannot express that
constraint, so the service clears the flag on the others in the same update.
The first address saved becomes the default automatically; deleting the default
promotes the next one, so a shopper with addresses always has one selected.

`email` lives inside the address because that is what the UI asks for: an
address for order updates, not a login. Two addresses may legitimately carry
different emails.

## `otp_codes`

Short-lived sign-in codes. The most security-sensitive collection here.

```js
{
  _id: ObjectId("..."),
  phone: "9876543210",
  codeHash: "<hmac-sha256(pepper, "otp:"+phone+":"+code)>",  // never the code
  attempts: 0,                                 // failed verifications
  consumed: false,                             // single use
  createdAt: ISODate("..."),
  expiresAt: ISODate("...")                    // createdAt + 5 minutes
}
```

| index | fields | why |
|---|---|---|
| `expiresAt_1` | `expiresAt`, **TTL `expireAfterSeconds: 0`** | Mongo deletes expired codes; no cleanup job, and codes do not linger |
| `phone_1_createdAt_-1` | `phone`, `createdAt` desc | fetch the newest code, and count recent ones for the rate limit |

**The code is stored as a keyed hash, not a bare one.** An earlier draft of
this file argued plain SHA-256 was sufficient because codes are short-lived.
That was wrong: the keyspace is only 10^6, so anyone who could read this
collection could reverse every live code instantly and sign in as anybody.
It is now HMAC-SHA256 under a server-side pepper, which makes the stored value
useless without the secret. The phone number is mixed into the message so a
hash cannot be replayed against a different number. `attempts` and `consumed` enforce
those two limits at the database, not in memory, so they survive a restart and
work across replicas.

## `carts`

```js
{
  _id: ObjectId("...userId..."),     // the user's _id — one cart per user
  lines: [
    { productId: "tablets-120", qty: 3 }
  ],
  updatedAt: ISODate("...")
}
```

Index: `_id` only. Every access is by the authenticated user's id.

**A line is a product id and a count.** No price, no name. Everything the cart
response shows is joined at read time. The line's public identifier is the
`productId` itself, so `DELETE /cart/items/tablets-120` needs no encoding.

Line uniqueness (one row per `productId`) is enforced in the service on write,
since MongoDB cannot express uniqueness *within* an array.

## `orders`

An order is created **before** payment (decision 6), so the document covers a
reservation as well as a receipt.

```js
{
  _id: ObjectId("..."),
  orderId: "EFF-483413",             // customer-facing, ^EFF-\d{6}$
  userId: ObjectId("..."),
  status: "pending_payment",         // see the lifecycle below
  lines: [                           // frozen at creation time
    {
      productId: "tablets-120",      // kept for traceability, never read for display
      name: "Spirulina Tablets 500 mg — 120 tabs", form: "Tablets",
      grad: "radial-gradient(...)",
      unitPrice: NumberLong(38000), unitMrp: NumberLong(47000),
      qty: 3, lineTotal: NumberLong(114000)
    }
  ],
  totals: {
    mrpTotal: NumberLong(141000), subtotal: NumberLong(114000),
    savings:  NumberLong(27000),  shipping: NumberLong(0),
    total:    NumberLong(114000)
  },

  payment: {
    provider: "razorpay",
    status:   "created",             // created|authorized|captured|failed|refunded
    amount:   NumberLong(114000),    // paise — what Razorpay was asked to collect
    currency: "INR",

    razorpayOrderId:   "order_PkX9aQ...",  // from the Orders API, set at creation
    razorpayPaymentId: null,               // set when a payment attaches
    razorpaySignature: null,               // the callback's HMAC, kept for audit

    method: null,                    // upi|card|netbanking|wallet|emi|paylater
    label:  null,                    // display copy, stamped once method is known
    last4:  null, network: null,     // card only — never the number
    bank:   null, wallet: null, vpa: null,

    attempts:   0,                   // payment attempts Razorpay reported
    capturedAt: null,
    failure:    null                 // { code, source, step, reason, description }
  },

  shippingAddress: { /* a copy of the chosen address, not a reference */ },

  createdAt: ISODate("..."),
  expiresAt: ISODate("..."),         // createdAt + 15 min — NOT a TTL index
  placedAt:  null,                   // set when payment is captured
  updatedAt: ISODate("...")
}
```

### Lifecycle

```
pending_payment ──capture──▶ placed ──▶ packed ──▶ shipped ──▶ delivered
       │                        │
       ├──failure──▶ payment_failed          └──▶ cancelled (from any stage)
       └──expiry───▶ expired
```

`pending_payment` holds a stock reservation. `placed` means the money is
captured and verified — it is the first state a customer would call "ordered",
and the only one the confirmation screen ever shows. `payment_failed` and
`expired` both release the reservation; they are kept rather than deleted so an
abandoned attempt can be explained.

Only the transitions out of `pending_payment` are produced today; the
fulfilment states exist so the field's domain is complete.

### Indexes

| index | fields | why |
|---|---|---|
| `orderId_1` | `orderId`, **unique** | customer-facing lookup; the uniqueness is what makes the generator's collision retry correct rather than hopeful |
| `userId_1_createdAt_-1` | `userId`, `createdAt` desc | "my orders", newest first, without an in-memory sort. Sorted on `createdAt`, **not** `placedAt`: a pending order has no `placedAt` yet, and sorting on a null would scatter live attempts through the history |
| `payment.razorpayOrderId_1` | **unique**, partial on existence | both the callback and the webhook resolve an order by it; uniqueness is what makes a replayed webhook safe |
| `payment.razorpayPaymentId_1` | **unique**, partial on existence | one Razorpay payment can never be applied to two orders |
| `userId_1` | **unique**, partial `status: "pending_payment"` | at most one live reservation per shopper — see below |
| `status_1_expiresAt_1` | partial `status: "pending_payment"` | the sweeper finds abandoned reservations without scanning the collection |

**One pending order per shopper, enforced by a partial unique index.** Without
it a shopper who opens checkout, abandons it, and opens it again reserves the
same stock twice and can sell the shop out against themselves. Creating a new
attempt therefore expires the previous one — releasing its stock — before
inserting. The index is the backstop that makes that correct under a double
click, not just under well-behaved clients.

**`expiresAt` must NOT carry a TTL index.** A TTL would delete the document,
which loses the record *and* silently strands the stock it was holding — the
reservation would never be returned. Abandoned attempts are swept explicitly
instead: query `status: "pending_payment", expiresAt: { $lt: now }`, return
each line's stock, then mark the order `expired`.

The sweep uses the same guard-in-the-filter idiom as `TakeStock`:

```js
updateOne({ _id: orderId, status: "pending_payment" }, { $set: { status: "expired" } })
```

`modifiedCount == 0` means someone else — the callback, the webhook, or another
sweeper — got there first, so stock must **not** be returned. Releasing on a
blind write would hand back units that a paid order now owns.

Lines, totals and `shippingAddress` are **copied, not referenced** —
decision 4. This is the one place duplication is the point. `productId` is
retained so an order can be traced back to what was sold, but nothing shown to
a customer is read live: a later price change, a renamed product or a deleted
address must not alter a placed order.

**No card number, ever.** Card details never reach this server at all — they
are entered inside Razorpay's checkout (see §Razorpay). Only the non-sensitive
metadata Razorpay reports back is stored: `last4` and `network`, for display.
`payment.label` is stored so a receipt renders correctly even if the label text
changes later.

**The cart is emptied when payment is captured, not when the order is
created.** A failed payment must leave the shopper's cart exactly as it was, or
a declined card costs them their basket as well.

## `payment_events`

Every webhook Razorpay delivers, keyed by Razorpay's own event id. This
collection exists for one reason: **webhooks are retried, and a retried
`payment.captured` must not be applied twice.**

```js
{
  _id: "evt_PkXA1bC2d3E4f5",         // Razorpay's X-Razorpay-Event-Id — the dedupe key
  event: "payment.captured",         // payment.captured|payment.failed|order.paid
  orderId: "EFF-483413",             // ours, resolved from razorpayOrderId
  razorpayOrderId:   "order_PkX9aQ...",
  razorpayPaymentId: "pay_PkXB7z...",
  receivedAt: ISODate("..."),
  processed:  true,
  payloadDigest: "sha256:9f2b..."    // a digest, never the raw payload
}
```

| index | fields | why |
|---|---|---|
| `_id_` | `_id` | implicit, and it is the whole mechanism — see below |

**Dedupe is an insert, not a lookup.** Because Razorpay's event id *is* the
`_id`, handling an event begins with an insert: a duplicate-key error means
this event has already been processed, so the handler returns 200 and does
nothing. A read-then-write check would have a race between the two steps that
two concurrent retries can walk straight through.

**The raw payload is not stored**, only a digest of it. A webhook body can
carry payment metadata we have no reason to keep, and a collection of raw
gateway payloads is a liability at rest. The digest is enough to prove what was
received if a delivery is ever disputed.

---

## Razorpay

Payment is collected by Razorpay Checkout. **There is no cash on delivery**, and
no payment instrument of any kind is entered on our pages: the shopper picks
UPI, card, netbanking or a wallet inside Razorpay's own modal. That is the
point of using them — card data never touches this server, so it can never be
logged, stored, or breached from here.

> Verify the field and header names below against Razorpay's current
> documentation before implementing. They are stable, but they are *their*
> contract, not ours, and this file cannot be the authority on somebody else's
> API.

### Configuration

| env var | what | notes |
|---|---|---|
| `RAZORPAY_KEY_ID` | public key, `rzp_test_…` / `rzp_live_…` | sent to the browser; not a secret |
| `RAZORPAY_KEY_SECRET` | API secret | signs the **callback**; server-only |
| `RAZORPAY_WEBHOOK_SECRET` | webhook secret | signs the **webhook**; a *different* secret, set in the dashboard |

`KEY_SECRET` and `WEBHOOK_SECRET` are distinct, and using one to verify the
other's signature fails in a way that looks like a forged request. Neither is
ever logged, echoed in a response, or committed; `.env.example` carries the key
names with empty values, as with every other secret here.

No keys exist yet. Until they do, the gateway sits behind an interface with an
unconfigured implementation that fails loudly in production and is faked in
tests — the same shape `auth.Sender` already uses for OTP delivery.

### The flow

```
1. POST /orders           we compute the cart total server-side, create a
                          Razorpay order for exactly that amount, reserve
                          stock, and store our order as pending_payment
                          → return { orderId, razorpayOrderId, amount, keyId }

2. browser                opens Razorpay Checkout with that handle; the
                          shopper chooses a method and pays

3a. callback (fast path)  browser posts razorpay_order_id, razorpay_payment_id
                          and razorpay_signature back to us → verify → placed

3b. webhook (truth)       Razorpay posts payment.captured to us, signed →
                          verify → placed

4. sweeper                anything still pending_payment past expiresAt has
                          its stock returned and is marked expired
```

**The amount is ours, never the client's.** The Razorpay order is created
server-side from the server-computed cart total, so a client cannot ask to be
charged ₹1 for a ₹1,140 basket. On capture, the amount Razorpay reports is
asserted equal to `totals.total` anyway — defence in depth, because a mismatch
there means something is wrong that no other check would catch.

### Verifying the callback

The browser hands back three values. The signature is an HMAC-SHA256 over
`razorpay_order_id + "|" + razorpay_payment_id`, keyed with
**`RAZORPAY_KEY_SECRET`**:

```
expected = HMAC_SHA256(razorpay_order_id + "|" + razorpay_payment_id, KEY_SECRET)
accept   = hmac.Equal(expected, provided)      // constant time, never ==
```

A callback that fails verification is discarded and logged as a security event.
It is treated as untrusted input in every respect — it arrives from a browser,
so it can be replayed, forged, or simply absent when someone closes the tab
mid-payment.

### Verifying the webhook

The webhook is the **authoritative** signal. Razorpay signs the request with
`RAZORPAY_WEBHOOK_SECRET` and sends the hex digest in `X-Razorpay-Signature`,
with the event's id in `X-Razorpay-Event-Id`.

```
expected = HMAC_SHA256(<raw request body bytes>, WEBHOOK_SECRET)
```

**Over the raw body, before any JSON parsing.** Decoding and re-encoding
changes whitespace and key order, which changes the bytes, which changes the
digest — a subtle way to make every legitimate webhook look forged. The handler
must read and retain the exact bytes received.

The webhook route is **unauthenticated but signed** — Razorpay has no bearer
token of ours. The signature is the only thing standing between a public URL
and a forged "payment captured", so verification is not optional and must
happen before the body is trusted for anything, including logging it.

Events consumed: `payment.captured` (→ `placed`), `payment.failed`
(→ `payment_failed`, stock released), and `order.paid` as a belt-and-braces
duplicate of capture. Anything else is acknowledged with 200 and ignored —
returning an error to an event we simply do not handle makes Razorpay retry it
forever.

**Always answer 200 once the signature verifies and the event is recorded**,
even for an event we ignore or one we have already applied. A non-2xx means
"retry me", and a handler that reports failure for an event it has successfully
deduped will be redelivered indefinitely.

### The callback and the webhook race — deliberately

Both paths flip the same order from `pending_payment` to `placed`, and either
may arrive first. That is not a defect to design out; it is why the transition
is a single guarded update:

```js
updateOne({ _id: ..., status: "pending_payment" }, { $set: { status: "placed", ... } })
```

Whichever lands first wins, and the second is a no-op that still returns
success to its caller. Stock is **not** touched on this transition — it was
already reserved at creation — so a double-apply cannot double-decrement.

### Refunds

Not modelled. A captured payment against an order that cannot be fulfilled is,
today, a manual refund in the Razorpay dashboard. The reservation model exists
precisely so that this stays a rare operational exception rather than a routine
consequence of overselling.

---

## Index creation

Indexes are created at startup by an idempotent `EnsureIndexes` on each
repository, not by hand in Atlas. `createIndex` is a no-op when the index
already exists, so it is safe on every boot, and a fresh environment cannot
come up missing one. Tracked as task 1.8.

The `unique` indexes are correctness constraints, not optimisations. Each one
is what makes a specific race safe:

| index | the race it settles |
|---|---|
| `users.phone` | concurrent OTP verification creating two accounts for one number |
| `orders.orderId` | the id generator's collision retry being correct rather than hopeful |
| `orders.payment.razorpayOrderId` | a replayed callback or webhook attaching to a second order |
| `orders.payment.razorpayPaymentId` | one payment being applied to two orders |
| `orders.userId` (partial, pending) | a double-clicked checkout reserving the same stock twice |
| `payment_events._id` | a retried webhook being processed twice |

## Not modelled yet

No collections for coupons, reviews or shipments — out of scope per
`product.md` §5. Inventory is now partly modelled (`products.stock`); stock
*movement* history is still missing. Webhook idempotency, which this file
previously listed as an open gap, is now `payment_events`.

**Refunds are not modelled**, and with real money moving that is a deliberate
gap rather than an oversight: a captured payment on an unfulfillable order is
resolved by hand in the Razorpay dashboard until there is a reason to automate
it.
