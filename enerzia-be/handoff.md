# Session handoff

How we work, where things stand, and the prompts to dispatch next.

---

## Start-of-session prompt

Paste this verbatim into a fresh session, opened at
`~/Desktop/EnerziaFutureFarm` (the workspace root, **not** inside `Enerzia/` —
the backend skill lives at the root and will not load otherwise).

````
We are building the Enerzia Future Farm backend: Go 1.25 + gorilla/mux +
MongoDB Atlas, in enerzia-be/. The Next.js storefront is in Enerzia/.

Read these before doing anything, in this order:
  1. .claude/skills/enerzia-backend/SKILL.md — the rules we work by
  2. enerzia-be/product.md   — what the shop is and does
  3. enerzia-be/schema.md    — the MongoDB model and why it is shaped that way
  4. enerzia-be/roadmap.md   — the API contract
  5. enerzia-be/tasks.md     — the backlog and what is done
  6. enerzia-be/handoff.md   — this file: workflow, dispatch prompts, traps

How we work — the dispatch loop. You do not write feature code yourself:
  1. you write ONE dispatch prompt for the next task in tasks.md
  2. I paste it to a subagent
  3. I paste the subagent's report back to you, raw
  4. you VERIFY against the actual repo — re-run `make check` and `make test`
     yourself, never trust the report — then accept or write a fix-up prompt
  5. you dispatch the next task

Rules that matter most:
  - one task per dispatch, finished completely (code + tests + lint + docs)
  - 90% coverage across ./internal/... , enforced by `make check`. Never lower
    it, never weaken a test to hold it up
  - Mongo is mocked with internal/mongotest, never skipped on a missing
    database
  - money is int64 paise, never float; prices are server-authoritative
  - contract changes are MY decision, not yours or a subagent's — if a task
    needs roadmap.md changed, stop and ask me

State: Phases 0–4 are complete and verified against live Atlas
(lint clean, 96.1% coverage, green under -race). Phase 5 (Orders) is next and
its four dispatch prompts are already written in handoff.md — start by
re-reading Dispatch 5.1, confirming it still matches the repo, and giving it
to me to relay.

Before dispatching anything, run `make check` in enerzia-be/ and tell me the
result, so we both know the baseline is still green.
````

---

## The dispatch loop

You relay; I write and verify. One task per round.

```
1. I write a dispatch prompt        →  copy it verbatim
2. you paste it to a subagent       →  it does the work
3. you paste its report back to me  →  raw output is fine
4. I verify against the real repo   →  accept, or write a fix-up prompt
5. I dispatch the next task
```

**Paste the subagent's report back even when it says "done".** I verify against
the repo, not against the report — a subagent claiming `make check` passed is
a claim, not evidence. Verification takes me one command; a wrong claim that
slips through costs a lot more.

**One task per dispatch.** Batching several removes the checkpoint that makes
this safe.

**Contract changes are not delegable.** If a subagent comes back asking to
change an endpoint shape, bring it to me — editing `roadmap.md` is a decision,
and decisions stay with you.

### What I check on each report

- `make check` and `make test` re-run here, not quoted from the report
- behaviour matches `roadmap.md`, including status codes and error copy
- tests assert failure paths, not just the happy path
- coverage did not drop, and no test was weakened or deleted to hold it up
- no `//nolint` without a reason, no threshold edited in the Makefile
- scope respected — nothing unrelated touched
- `tasks.md` updated

---

## Where things stand

**Phases 0–4 complete**, then the model was reworked twice at your request.

| phase | what | state |
|---|---|---|
| 0 Foundation | module, config, Mongo, router, middleware, `/health`, `mongotest` fake | done |
| 1 Catalogue | products, seed, repository, three read endpoints | done |
| 2 Auth | OTP request/verify, JWT, middleware, `/auth/me` | done |
| 3 Cart | full CRUD with server-side pricing | done |
| 4 Addresses | multiple per user, one default | done |
| 5 Orders | **next** | not started |
| 6 Hardening | 6.1 rate limiting done; CORS, graceful shutdown, CI remain | partial |
| 7 Frontend integration | not started | |

Gate at close of session:
`lint 0 issues · total coverage 96.1% · all tests green under -race`

Live Atlas is migrated and seeded: 9 products, 4 trust tiles, no stray
collections.

### The current model, in one breath

**A product is a sellable item.** Nine of them — `powder-100g`, `tablets-120`,
`bundle-family` and so on. Each has its own price, stock, blurb, nutrition and
detail page, and gets its own card in the grid. There is **no variant entity**.
`family` groups siblings for display only. Carts and orders hold a `productId`.

Read `schema.md` before touching any repository. It is the source of truth and
it has been rewritten twice.

---

## Dispatch prompts — ready to send

Send these in order. Each is self-contained: a subagent starts cold.

### Dispatch 5.1 — Order model and id generator

```
Task 5.1 from enerzia-be/tasks.md: Order model + EFF-###### id generator.

Read first, in this order:
  1. enerzia-be/product.md   — §3.6 confirmation screen, §4 domain rules
  2. enerzia-be/schema.md    — §orders (the document shape and both indexes)
  3. enerzia-be/roadmap.md   — §Orders (the exact response shape)
  4. enerzia-be/tasks.md     — the 5.1 row
  5. .claude/skills/enerzia-backend/SKILL.md — conventions and definition of done

Scope — create internal/order/ with ONLY:
  - model.go: Order, OrderLine, Totals, Payment, Status. Money is int64 paise.
    Lines and totals are SNAPSHOTS: they copy name/price at purchase time and
    are never re-read from the catalogue. Keep productId for traceability.
  - id.go: NewOrderID() returning ^EFF-\d{6}$ using crypto/rand, plus a
    validator. It must be usable in a collision-retry loop.
  - model_test.go, id_test.go

Out of scope: the repository, the service, the handler, any routing, any
changes outside internal/order/.

Constraints:
  - money is int64 paise, never float
  - pure package: no I/O, no MongoDB imports, no net/http
  - table-driven tests; cover the failure paths, not just the happy path
  - `make check` must be clean before you report done

Report back, and nothing else:
  - files added
  - `make lint` result
  - `make cover` total, plus the line for internal/order
  - anything you could not do, and why
  - anything out of scope you noticed (for tasks.md — do NOT fix it)
```

> **The old 5.2–5.4 prompts were deleted on 2026-08-07.** They described a
> world with cash on delivery and card numbers posted to our own API, which the
> Razorpay decision removed. Leaving them here would have been worse than
> having no prompt at all: a subagent starts cold and trusts what it is given.
> Phase 5 is now nine tasks (`tasks.md`); prompts are written one at a time,
> against the amended contracts.

### Dispatch 5.2 — Amend the order model for Razorpay

```
Task 5.2 from enerzia-be/tasks.md: amend the order model for Razorpay.

Context: payment moved to Razorpay Checkout and cash on delivery was dropped.
The contracts were amended FIRST and are correct; internal/order/model.go was
written before the decision and is now out of step with them.

Read first, in this order:
  1. enerzia-be/schema.md    — decision 6, §orders (document shape + lifecycle),
                               and §Razorpay
  2. enerzia-be/roadmap.md   — §Orders
  3. enerzia-be/product.md   — §3.5
  4. .claude/skills/enerzia-backend/SKILL.md
Then read the existing internal/order/model.go and model_test.go.

Scope — internal/order/model.go and model_test.go ONLY.

Status: add pending_payment, payment_failed and expired to the existing five.
The full set is in schema.md §orders "Lifecycle"; match it exactly.

Payment: replace the current struct. It becomes the sub-document in
schema.md §orders — provider, status, amount (int64 paise), currency, the
three razorpay* fields, method, label, the method-detail fields (last4,
network, bank, wallet, vpa), attempts, capturedAt and failure.

  - DELETE PaymentCOD and its "Cash on delivery" label. COD no longer exists.
  - PaymentMethod becomes what Razorpay reports: upi, card, netbanking,
    wallet, emi, paylater. Labels: UPI, Card, Netbanking, Wallet, EMI,
    Pay Later.
  - method and label are EMPTY until Razorpay reports them — a pending order
    has no method yet. Validate must accept that, and must NOT accept a
    method-detail field (last4, vpa, bank...) on a payment with no method.
  - last4 and network are card-only; vpa is upi-only; bank is netbanking-only;
    wallet is wallet-only. Keep the existing "a stray last4 is rejected"
    rigour and extend it to the new fields.
  - There is still NO field for a card number, anywhere.

Order: add CreatedAt and ExpiresAt; PlacedAt becomes nil-able (a pending order
has not been placed). Validate must accept a pending_payment order — one that
has no placedAt and no payment method — and still reject a placed order that
is missing them. That split is the point of this task: a single Validate that
only understood a finished order would either reject every reservation or wave
through a half-finished receipt.

Use *time.Time for PlacedAt and Payment.CapturedAt. This is the first pointer
time in the repo — that is deliberate, not drift. schema.md documents both as
null before they happen, and a zero time.Time would marshal to a year-1 date
instead, which reads as a real timestamp to anything querying on it. Every
other time field stays a plain time.Time.

Out of scope: the repository, the gateway, any service or handler, any HTTP,
any signature verification, and anything outside internal/order/. Do NOT
create the Razorpay client — that is 5.3.

Constraints:
  - money stays int64 paise
  - pure package: bson tags fine, no driver calls, no net/http
  - table-driven; cover the new failure paths — a detail field on the wrong
    method, a placed order with no capturedAt, an unknown status
  - internal/order must stay >=90% covered on its own
  - `make check` clean before you report done

If you believe a contract file needs changing, STOP and say so — do not edit
schema.md, roadmap.md or product.md.

Report back, and nothing else:
  - files changed
  - `make lint` result
  - `make cover` total plus the internal/order line
  - anything you could not do, and why
  - anything out of scope you noticed (for tasks.md — do NOT fix it)
```

---

## Open decisions for you

| # | question | why it matters |
|---|---|---|
| 1 | **OTP delivery: WhatsApp or SMS?** Neither is free — comparison below. | Blocks production sign-in only; development works today via `devCode` |
| 2 | Money as int64 paise vs whole rupees | Frontend holds rupees; conversion lands in task 7.1 |
| 3 | The frontend still renders 4 cards with size pills | It must move to 9 flat cards to match the new model — task 7.1 |

### OTP delivery — the comparison

Neither channel is free at production volume, and cost is not the deciding
factor at this scale. Onboarding friction is. **Verify current pricing before
committing; both change often.**

| | SMS | WhatsApp |
|---|---|---|
| Onboarding | **DLT registration is mandatory in India** — entity, sender header and every template registered on the operator portal. Days to weeks. | Meta Business verification + an authentication template approval. Usually faster. |
| Per message | roughly ₹0.12–0.25 | authentication templates, typically a little cheaper |
| Reach | every phone, no app needed | only WhatsApp users — very high in India, not universal |
| Free tier | provider trial credit only | user-initiated *service* conversations are free, but an OTP is business-initiated and does not qualify |

Recommendation: **SMS via MSG91 or Fast2SMS** for universal reach; add WhatsApp
later if delivery rates disappoint. Start the DLT registration now — it is the
long pole, not the code.

The code side is done: implement `auth.Sender` (one method) and swap it in
`cmd/api/main.go`. Nothing else changes.

---

## Credentials

MongoDB Atlas is connected and working; `.env` is populated and git-ignored.
Nothing outstanding until a real OTP provider is chosen.

---

## Things that will bite whoever picks this up

- **`Seed` upserts but never removes.** A superseded document shape or a
  retired index survives a re-seed and needs a manual drop. This has already
  happened twice during the model reworks.
- **`make check` gates on 90% across `./internal/...`**, excluding `cmd/` and
  `mongotest` as infrastructure. Do not lower it; add tests.
- **Mongo is mocked, never skipped.** `internal/mongotest` is a real
  wire-protocol fake — `Respond`, `RespondSequence`, `Cursor`, `Fail`. A
  `t.Skip` on a missing database is an untested code path wearing a disguise.
- **gorilla/mux loses the method-mismatch signal** when a later route fails on
  its path matcher. `server.methodAware` recovers it; do not "simplify" it away
  or every 405 silently becomes a 404.
- **The frontend is now out of step with the backend** — it still assumes four
  products with size pills. Task 7.1.
