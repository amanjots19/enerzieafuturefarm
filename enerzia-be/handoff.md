# Session handoff

How we work, where things stand, and what to do next.

Last updated 2026-08-12, after the storefront was wired end to end and sign-in
moved to the MSG91 widget.

---

## Start-of-session prompt

Paste this verbatim into a fresh session, opened at
`~/Desktop/EnerziaFutureFarm` (the workspace root, **not** inside `Enerzia/` —
the skill lives at the root and will not load otherwise).

````
We are building Enerzia Future Farm: a Go 1.25 + gorilla/mux + MongoDB Atlas
API in enerzia-be/, and a Next.js storefront in Enerzia/. One git repo.

Read these before doing anything, in this order:
  1. .claude/skills/enerzia-backend/SKILL.md — the rules we work by
  2. enerzia-be/product.md   — what the shop is and does
  3. enerzia-be/schema.md    — the MongoDB model and why it is shaped that way
  4. enerzia-be/roadmap.md   — the API contract
  5. enerzia-be/tasks.md     — the backlog and what is done
  6. enerzia-be/handoff.md   — this file: workflow, state, traps, what is next

How we work — the dispatch loop. You do not write feature code yourself:
  1. you write ONE dispatch prompt for the next task in tasks.md
  2. I paste it to a subagent
  3. I paste the subagent's report back to you, raw
  4. you VERIFY yourself — re-run make check, and open a REAL BROWSER for any
     frontend change. Never trust the report — then accept or write a fix-up
  5. you dispatch the next task

Rules that matter most:
  - one task per dispatch, finished completely (code + tests + lint + docs)
  - 90% coverage across ./internal/... , enforced by `make check`. Never lower
    it, never weaken a test to hold it up
  - Mongo is mocked with internal/mongotest, never skipped on a missing database
  - money is int64 paise, never float; prices are server-authoritative
  - contract changes are MY decision — if a task needs roadmap.md or schema.md
    changed, stop and ask me
  - ANOTHER AGENT is building the admin catalogue in this same repo. Read the
    ownership boundary at the top of tasks.md before touching shared code

Before dispatching anything, run `make check` in enerzia-be/ and
`npm run typecheck` in Enerzia/, and tell me both results.
````

---

## The dispatch loop

```
1. I write a dispatch prompt        →  copy it verbatim
2. you paste it to a subagent       →  it does the work
3. you paste its report back to me  →  raw output is fine
4. I verify against the real repo   →  accept, or write a fix-up prompt
5. I dispatch the next task
```

**Paste the report back even when it says "done".** Verification is not
optional and it is not paranoia — see "What verification has actually caught"
below.

**Contract changes are not delegable.** If a subagent asks to change an
endpoint shape, bring it to the user.

### Verify frontend work in a REAL browser

This is the single most important habit in this project. Subagent reports have
been wrong or incomplete on this repeatedly:

- a Node script "simulating React StrictMode" instead of loading the page
- instructions for how to verify, presented as if they were results
- a mocked `GET /orders` fixture that hid two real bugs (a truncated order id
  and a CSS gradient printed as visible text) which live data exposed in
  thirty seconds

**Mocking the endpoint you are verifying proves nothing.** Use real data.

Watch your own measurements too. Four times a "bug" turned out to be a bad
test: stale DOM node references after a React re-render, reading the DOM in the
same tick as a click, comparing against an unmounted list, and a keypress that
did not map to `ArrowDown`. Re-test before reporting a defect.

---

## Where things stand

**Backend Phases 0–6: complete and verified.** `make check` clean, **97.0%**
coverage, green under `-race`, CI green on GitHub, full checkout journey
verified against live Atlas and Razorpay's test API.

**Phase 7 (storefront wiring): complete bar cleanup.** The shop is real:
9 products from Atlas, filters, PDP, OTP sign-in, server-backed cart that
survives reload, saved addresses, review screen, Razorpay Checkout, thank-you
screen. Verified in a browser throughout.

**Phase 8 (MSG91 sign-in): working.** `POST /auth/session` verifies the
widget's access token and issues our JWT. The user confirmed a real sign-in
works after the MSG91 IP allowlist was sorted.

**Phase 9 (account area): half done.** Header account menu and order history
are built and verified; saved addresses and contact remain.

Git: `627addc`. The frontend and MSG91 work are committed. **12 files are
uncommitted** — the other agent's `internal/catalogue` changes plus the three
shared docs (see below).

---

## Two agents, one repo

The **shop** (this thread) and the **catalogue manager** (another session) are
being built in parallel. The ownership table is at the top of `tasks.md`. The
short version:

| area | owner |
|---|---|
| `Enerzia/`, `internal/{cart,order,msg91}`, shopper auth | shop |
| admin endpoints, `internal/catalogue` writes, image upload | catalogue |
| `internal/auth` | **shared — coordinate before deleting** |
| `roadmap.md`, `schema.md`, `tasks.md` | both — re-read before editing |

**Task 8.2 is parked** because of this. It deletes the OTP model, repository
and `Sender` from `internal/auth`, which is where admin login will be built.
The old OTP endpoints are dormant, not harmful. Deleting shared foundations
under another agent is not worth the tidiness — leave it until admin auth
lands.

`products.images` is additive on the wire, so the storefront ignores it safely.
It will not render photographs until task 9.5.

---

## What to do next

In priority order.

### 1. Two dispatches are outstanding, never reported back

- **7.13 — retry on a failed boot.** With the backend down, the shop shows
  "Unable to reach the server" and stays empty **permanently**: the boot effect
  runs once and never retries. A shopper concludes the site is broken and
  leaves. Small fix, disproportionate save.
- **9.6 — "My orders" lists only orders that were paid for.** The contract in
  `roadmap.md` §`GET /orders` is already rewritten; only the repository filter
  remains. Fixes a card reading "Total paid ₹449" beside "Payment pending".

Both prompts are written and were given to the user. Re-issue them.

### 2. Finish Phase 9

- **9.3 — saved addresses screen.** The list and form live inside
  `CartScreen`. **Extract** them so both screens share one implementation; a
  copy will drift.
- **9.4 — contact us.** Details supplied: `support@enerzeiafuturefarm.com`,
  phone and WhatsApp `8826345094`. No business hours were given — omit the
  line rather than inventing a response-time promise.

### 3. Cleanup debts

- **7.9** dead-code sweep
- **7.10** reconcile `roadmap.md` with the code — 12 discrepancies found during
  the original exploration, since joined by the auth rewrite
- **7.11** **pre-production data cleanup of live Atlas.** Development has run
  against the real `enerzia` database throughout. It holds test users,
  addresses, orders and decremented stock. Do not skip: a test order in a real
  order history is indistinguishable from a real one later

### 4. Before launch

- **8.4/8.5** drop `otp_codes` from Atlas; live smoke test of MSG91 sign-in
- **8.6** MSG91 panel: set the widget to **mobile only** (see traps)
- **8.7** whitelist the **production** server's egress IP in MSG91
- Razorpay webhook needs a public URL their dashboard accepts
- Rotate every credential that has been pasted into a chat transcript

---

## Things that will bite whoever picks this up

**MSG91 rejects everything with `418` if the caller's IP is not whitelisted.**
The check runs *before* the token is read, so a valid token and the string
`"bogus"` return the identical error. This cost several rounds of debugging.
`201` means the auth key is wrong; `418` means the key is fine and the caller
was rejected; `701` means the token itself was evaluated and refused. Also:
Go's dialer prefers IPv6, and only the IPv4 address is whitelisted — the
client is pinned to `tcp4` for exactly this reason. Do not remove that pin.

**MSG91 sends `code` as a quoted string sometimes and a bare number other
times.** There is a custom unmarshaler for it. Do not "simplify" it to
`string`.

**The MSG91 widget must be configured mobile-only in their panel.** It can also
verify by email, which returns an email as the identifier — and `users.phone`
is the identity, so the server correctly refuses it while the shopper sees a
baffling failure after MSG91 told them they succeeded. Nothing in the codebase
can enforce or explain this.

**Atlas allowlists by IP and this connection's IP rotates.** `tls: internal
error` on all shards means the IP, not the code. It happened three times in one
session.

**`Seed` upserts but never removes.** Superseded document shapes and retired
indexes survive a re-seed and need a manual drop.

**gorilla/mux loses the method-mismatch signal** when a later route fails on its
path matcher. `server.methodAware` recovers it; do not "simplify" it away or
every 405 silently becomes a 404.

**`npm run lint` is broken project-wide** — Next 16 removed `next lint` and
there is no ESLint config. `npm run typecheck` is the only automated gate the
frontend has. That makes browser verification more important, not less.

**Testing a duplicate key needs `mongotest.Reply`, not `Fail`.** `Fail` sends
`ok:0`, which the driver turns into a `CommandError` with no `WriteErrors`, so
the test proves nothing.

---

## What verification has actually caught

Kept as evidence that step 4 of the loop earns its cost. Every one of these
came back in a report marked done or passing:

| task | what was wrong |
|---|---|
| 5.2 | `Payment.Amount` never compared to `Totals.Total` — an order could be charged ₹1 for a ₹1,140 basket |
| 5.3 | the Razorpay HTTP client had **no timeout at all** |
| 5.4 | duplicate-key classification worked only by an accident of capitalisation |
| 5.7 | webhook marked events processed **before** processing them, so a database wobble silently swallowed a paid order on retry |
| 7.2 | `state.banner` was set on failure but rendered nowhere — every error was invisible |
| 7.6 | a `<button>` nested inside a `<button>`, causing a hydration error |
| 9.2 | order id truncated to `#F-118531`, and a CSS gradient printed as visible text |

The 5.7 one is the one worth remembering: money taken, order silently expired.

---

## Credentials

All in `enerzia-be/.env` (git-ignored) except the two public MSG91 widget
values, which are in `Enerzia/.env.local`:

- MongoDB Atlas — working
- `RAZORPAY_KEY_ID` / `KEY_SECRET` / `WEBHOOK_SECRET` — **test mode**
- `MSG91_AUTH_KEY` — the **account** key. Not the widget `tokenAuth`; swapping
  them gives `201`
- `NEXT_PUBLIC_MSG91_WIDGET_ID` / `TOKEN_AUTH` — client-side by design

**Everything here has been pasted into a chat transcript at some point and
should be rotated before launch.**
