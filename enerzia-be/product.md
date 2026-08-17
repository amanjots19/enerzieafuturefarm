# Enerzia Future Farm — product reference

What the shop is, what each frontend screen does, and the domain rules the API
has to honour. Written from the implemented frontend (`app/shop`,
`components/shop`, `lib/shop` at the repo root), which was itself built from
the Claude Design project "Spirulina shop design system".

This file is the *why*. `roadmap.md` is the *contract*. `tasks.md` is the *when*.

---

## 1. The business

Enerzeia Future Farm sells farm-grown spirulina direct to consumers in India:
powder, pressed tablets, low-waste refill pouches, and bundles. Single
ingredient, FSSAI-licensed facility, batch lab-tested. Prices are in INR, the
audience is Indian, and sign-in is by mobile number + OTP — not email/password.

---

## 2. Catalogue

Four products, each with two or three size variants. A **variant** is what is
actually bought and priced; a product on its own has no price.

| id | form | name | variants (label · sub · MRP → price) |
|---|---|---|---|
| `powder` | Powder | Pure Spirulina Powder | 100 g · 20 servings · ₹250→₹200<br>250 g · 50 servings · ₹560→₹450 |
| `tablets` | Tablets | Spirulina Tablets 500 mg | 60 tabs · 15 days · ₹250→₹200<br>120 tabs · 30 days · ₹470→₹380<br>300 tabs · 75 days · ₹1100→₹850 |
| `refill` | Powder | Spirulina Refill Pouch | 250 g · 50 servings · ₹520→₹420<br>500 g · 100 servings · ₹980→₹760 |
| `bundle` | Bundle | Daily Wellness Duo | Starter · 100 g + 60 tabs · ₹500→₹380<br>Family · 250 g + 120 tabs · ₹1030→₹790 |

Per-product fields the UI renders: `form`, `name`, `stat`, `stat2` (two short
selling points shown side by side), `blurb` (PDP paragraph), `grad` (a CSS
gradient standing in for the product photograph — no real product images exist
yet), and the variant list.

**Forms** are `Powder`, `Tablets`, `Bundle`. Note `refill` is also form
`Powder`, so the Powder filter returns two products. The filter set shown in
the UI is All / Powder / Tablets / Bundles.

**Default variant** is index 0 for every product except `tablets`, which
defaults to index 1 (120 tabs). This is a merchandising choice carried over
from the design and must survive into the API.

Shared PDP content, currently identical for every product and hardcoded in the
frontend — the API should own it, per product, so it can diverge later:

- Rating: 4.8, 312 reviews
- Badges: Lab tested / FSSAI licensed / No binders / Free delivery
- Nutrition per 5 g serving: Protein 3.1 g, Iron 4.2 mg, Phycocyanin 750 mg,
  Beta-carotene 1.2 mg, Energy 19 kcal
- Trust strip on the shop screen: 62%+, FSSAI, 0, 48 hrs

---

## 3. Screens and what each one needs

The frontend is a single client-side state machine with six screens
(`lib/shop/types.ts: Screen`). There is no URL per screen yet.

### 3.1 Shop (`shop`)
The product grid. Needs: the full catalogue with variants, the form filter, and
the trust strip.

Each card shows the **selected variant's** price, MRP and discount, and a size
selector. Adding puts that variant in the cart; the Add button then becomes an
inline `− n in cart +` stepper. Decrementing to zero removes the line and
restores the Add button. Selection is tracked **per product per variant**, so
one size can be in the cart while another still shows Add to cart.

### 3.2 Product detail (`pdp`)
One product. Needs everything above plus `blurb`, rating, badges and nutrition.
Has its own quantity stepper (floor 1) *before* adding. The size selection is
shared with the grid — changing it here changes it there.

### 3.3 Sign in (`login`)
Two stages on one screen.

1. **Phone stage** — 10-digit Indian mobile, `+91` fixed. Input strips
   non-digits and caps at 10. Invalid → "Enter a valid 10-digit mobile number".
2. **OTP stage** — 6 digits, stripped and capped. Invalid → "Enter the 6-digit
   code (try 123456)".

Sign-in is a **gate, not a destination**: it is triggered by trying to reach
the cart, and on success it forwards to wherever the shopper was heading. Once
signed in the gate is transparent.

> **Known gap.** The frontend only checks that the OTP is *six digits long* —
> any six digits sign you in, despite the "try 123456" copy. Real verification
> is the backend's job.

> **Two changes since the design was written.** Addresses became a *list*: a
> shopper can save several and pick one at checkout; the validation rules below
> are unchanged and apply to each. And **every size is now its own product** —
> the grid shows nine cards, not four with size pills, and there is no size
> selector on a product page.

### 3.4 Cart (`cart`)
Auth-required. Shows each line (product, variant + sub, unit price, MRP,
quantity stepper, line total, remove), the delivery address form, and the order
summary. Empty state: "Nothing here yet." with a Browse products button.

Address fields, all required, validated in this exact order — the first failure
is the only message shown:

| # | field | rule | message |
|---|---|---|---|
| 1 | name | non-blank | Please enter the name for delivery. |
| 2 | email | `^\S+@\S+\.\S+$` | Please enter a valid email for order updates. |
| 3 | phone | exactly 10 digits | Please enter a valid 10-digit mobile number for delivery. |
| 4 | line1 | ≥ 6 chars trimmed | Please enter your full street address. |
| 5 | city, state | both non-blank | Please enter your city and state. |
| 6 | pin | exactly 6 digits | PIN code must be 6 digits. |

> **Added 2026-08-17.** `phone` is the delivery contact for *this address* and
> is not the account's sign-in number, even though they are usually the same —
> a shopper sending a gift needs the recipient reachable at the door. It is
> required on every write; addresses saved before it existed have none, stay
> usable, and fall back to the account number at checkout.

### 3.5 Checkout (`checkout`)
Auth-required. Read-only address summary (Edit returns to cart), order summary,
and a single `Pay ₹<total>` button.

> **Changed since the design was written.** Payment now goes through
> **Razorpay Checkout**, and **cash on delivery is gone.** The screen no longer
> has a payment-method picker of its own: pressing Pay asks our API to open a
> checkout, then hands off to Razorpay's modal, where the shopper chooses UPI,
> card, netbanking or a wallet. There are no UPI-ID or card fields on our pages
> any more, and so none of the validation below survives on the client or the
> server.
>
> The rules the design specified — UPI `^[\w.\-]{2,}@[a-zA-Z]{2,}$`, 16-digit
> card, `MM/YY` expiry, 3-digit CVV — are recorded here only so nobody
> reintroduces them. Razorpay owns that validation now.

What the shopper's payment method means to us afterwards: Razorpay reports the
method it actually used, and we store it for the receipt (`UPI`, `Card`,
`Netbanking`, `Wallet`), along with the last four digits and network for a
card, or the VPA for UPI. **No card number ever reaches this server**, which is
the main reason for the change.

The order is created *before* payment, holds a stock reservation for 15
minutes, and only becomes a real order when Razorpay confirms the money moved.
An abandoned checkout expires and returns its stock. See `roadmap.md` §Orders.

> **Known gap.** Refunds are not built. A payment captured against an order
> that cannot be fulfilled is refunded by hand in the Razorpay dashboard.

### 3.6 Confirmation (`done`)
Order id, ETA, shipping address, the ordered lines, amount paid and method. The
cart is emptied. Order ids look like `EFF-483413` — `EFF-` plus six digits.

---

## 4. Domain rules the API owns

These are currently computed client-side. The server must own all of them,
because a client can lie about any of them.

**Cart line identity** is the `productId`. Adding a product already in the cart
increments quantity rather than creating a second line. A line whose quantity
reaches zero is deleted.

**Stock.** Each product carries a unit count. `stock <= 0` is sold out —
derived, never stored as a flag. A sold-out item cannot be added to a cart, and
one that sells out while sitting in someone's cart blocks their checkout with
the line flagged rather than silently removed.

**Totals**, in this order:

```
mrpTotal = Σ (variant.mrp   × qty)
subtotal = Σ (variant.price × qty)
savings  = mrpTotal − subtotal
shipping = 0  if subtotal == 0 or subtotal ≥ 499
           50 otherwise
total    = subtotal + shipping
```

**Free-delivery nudge**: when `0 < subtotal < 499`, show "Add ₹(499 − subtotal)
more for free delivery."

**Discount percentage** shown on a variant is `round((1 − price/mrp) × 100)`.

**Money.** The frontend currently holds whole rupees as plain numbers. The API
uses **int64 paise** throughout to keep money exact — ₹499 is `49900`. The
frontend needs a divide-by-100 at the edge when it is wired up; see
`roadmap.md` §Money.

**Currency/locale**: INR, formatted `en-IN` (₹1,030 — lakh grouping).

---

## 5. Out of scope for now

Explicitly not being built yet, listed so nobody assumes they exist: real SMS
delivery, refunds, cash on delivery, stock movement history, shipping/logistics
integration, coupons, reviews (the 4.8/312 is static), search, wishlists,
returns, and the marketing landing page (still static `index.html` at the repo
root, not migrated to Next.js).

Payment capture moved *into* scope with Razorpay — see §3.5.
