# Enerzia backend — tasks

Agile backlog. One task at a time, finished completely (code + tests + lint +
docs) before the next is started. See `../.claude/skills/enerzia-backend/SKILL.md`
for the definition of done.

Status: `TODO` · `WIP` · `DONE` · `BLOCKED`

---

## In progress

_nothing_ — **Phases 0–6 complete.** The backend is done to the roadmap's
scope. Only **Phase 7 (frontend integration)** remains, and it needs
re-planning first: the two rows below understate it badly.

**The workspace is now one git repo**, pushed to
`github.com/amanjots19/enerzieafuturefarm`. Both prior histories were preserved
via subtree import rather than discarded, so the static-site and backend
commits are still in `git log`. `Enerzia/` and `enerzia-be/` are ordinary
directories now, not nested repos.

Phase 6 gate: **CI green on GitHub** (run 31211813050) — lint, race tests and
the 90% coverage gate all pass in a runner with **no database and no secrets**,
which is independent proof that `mongotest` makes the suite genuinely hermetic.

## Blocked

_nothing blocking._ Razorpay **test** keys and the webhook secret are in `.env`
and working — the full journey was smoke-tested against live Atlas and their
real test API (see the Phase 5 gate).

One deferred item, not a blocker: Razorpay's dashboard rejects tunnel hostnames
(`.loca.lt` confirmed blocked), so a webhook delivered by Razorpay themselves
has never been received. Our own signed deliveries exercise the same handler
with the same secret. Resolve when the service is deployed somewhere with a
real domain; try `trycloudflare.com` or ngrok first if a tunnel is wanted
sooner.

Live keys are **not** in `.env` and should never be pasted into a chat — put
them straight into the deployed environment.

---

## Phase 0 — Foundation

| id | task | status | notes |
|---|---|---|---|
| 0.1 | Go module, directory layout, Makefile, `.golangci.yml`, `.env.example`, `.gitignore` | DONE | `make lint` / `test` / `cover` / `check`; 90% gate over `./internal/...` |
| 0.2 | Typed config loaded once from env, with validation and defaults | DONE | 100% cov; reports every problem at once; never echoes secret values |
| 0.3 | MongoDB connection: connect, ping, timeouts, clean disconnect | DONE | 95.8% cov via the `mongotest` fake — no live cluster needed. Remaining line is an unreachable defensive branch in `Disconnect`. |
| 0.4 | Router, JSON envelope helpers, error codes, 404/405 handlers, recovery + request-id + logging middleware | DONE | 100% cov; 404/405 go through the middleware chain too |
| 0.5 | `GET /health` incl. degraded state when Mongo is down | DONE | 100% cov; 503 + body when Mongo is down, driver detail logged not returned |
| 0.6 | `internal/mongotest` — in-process fake MongoDB (wire protocol handshake, commands, per-command failure injection) | DONE | Excluded from the coverage gate as test infrastructure; has its own suite. Replaces the `MONGO_TEST_URI` skips. |

Phase 0 gate at completion: **lint 0 issues · total coverage 99.2% · all tests green (race)**.

Phase 1 gate at completion: **lint 0 issues · total coverage 99.7% · all tests green (race)**.

Phase 3 gate at completion: **lint 0 issues · total coverage 97.4% · all tests green (race)** · full cart journey verified against live Atlas, and the stored document confirmed to contain no pricing.

Phase 2 gate at completion: **lint 0 issues · total coverage 98.2% · all tests green (race)** · full sign-in flow verified against live Atlas.

Phase 5 gate at completion: **lint 0 issues · total coverage 96.9% · all tests green (race)** · **full checkout journey verified against live Atlas and the real Razorpay test API** on 2026-08-08 — 18/18 checks. A real Razorpay order was created through their API; the signed `payment.captured` webhook moved the order to `placed` and emptied the cart; a replayed event id was deduped; a tampered body and a missing signature were both rejected 400; another shopper's read returned a 404 byte-identical to an unknown id. Server logs audited: no key, secret or Mongo URI present.

The one part still unproven: a webhook delivered **by Razorpay themselves**, which needs a public URL their dashboard accepts (`.loca.lt` is blocklisted). Our verification signs the payload with the real `RAZORPAY_WEBHOOK_SECRET`, so it exercises the true code path — but not their transport. Close this at deploy time.

## Model rework (user-requested, after Phase 4)

| id | task | status | notes |
|---|---|---|---|
| M.1 | ~~Variants become their own collection~~ | SUPERSEDED by M.5 | |
| M.5 | Every size becomes an independent product; `variants` collection folded away | DONE | 9 products, one per sellable item. Cart and orders hold a `productId`. `family` groups siblings for display only. Net simplification: one collection, one document per sellable thing. |
| M.2 | `stock` on the product; sold out derived, never stored | DONE | `TakeStock` guards with `$gte` **inside the filter**; verified on live Atlas that it refuses rather than going negative. `ReturnStock` compensates a partial failure. |
| M.3 | Addresses become an array on the user, each with its own `_id` | DONE | Multiple addresses, exactly one default, deleting the default promotes the next. `GET/POST /me/addresses`, `PUT/DELETE /me/addresses/{id}`. |
| M.4 | Contracts updated first: `schema.md`, `roadmap.md`, `product.md` | DONE | Now five modelling decisions; orders keep snapshotting deliberately. |

## Phase 1 — Catalogue

| id | task | status | notes |
|---|---|---|---|
| 1.1 | Product/variant domain model, `lineId` slug helper, discount calculation | DONE | `internal/catalogue`, 100% cov. Adds `Form`/`ParseFilter`, `Variant.DiscountPercent` verified against all 9 real prices, `Slug`/`LineID`/`SplitLineID`, and `Product.Validate` for seed and stored data. |
| 1.2 | Seed data for the 4 products + trust tiles + badges + nutrition, and a `seed` command | DONE | `internal/catalogue/seed.go` + `cmd/seed` (`make seed`), idempotent upserts. Tests pin all 9 prices to `product.md` §2. |
| 1.3 | Product repository against Mongo, tested with the `mongotest` fake | DONE | 100% cov. List/Get/TrustTiles/Seed/EnsureIndexes; tests assert the actual filter, sort and upsert flag sent to Mongo. |
| 1.7 | Smoke test the real Atlas connection once credentials land | DONE | Seeded 4 products + 4 tiles into `enerzia`, ran the API against it, exercised every endpoint incl. the full sign-in flow. Indexes verified on the live cluster. |
| 1.8 | `EnsureIndexes` per repository, called at startup | PARTIAL | Live cluster confirms `products.form_1`, `users.phone_1 UNIQUE`, `otp_codes.expiresAt_ttl` and `phone_1_createdAt_-1`. Orders indexes land with Phase 5. |
| 1.4 | `GET /api/v1/products` with `form` filter | DONE | 100% cov. Service parses the filter, handler maps `ErrUnknownForm` → 400. Detail content omitted from the list. |
| 1.5 | `GET /api/v1/products/{id}` with badges, nutrition, rating | DONE | 100% cov. Unknown id → 404; driver detail logged, never returned. |
| 1.6 | `GET /api/v1/content/trust` | DONE | 100% cov. Unseeded database returns `[]`, not an error — the strip is decoration. |

## Phase 2 — Auth

| id | task | status | notes |
|---|---|---|---|
| 2.1 | OTP model: hashed code, TTL index, attempt counter, single use | DONE | HMAC-keyed hash (not bare SHA-256 — see Discovered), single-use via a `consumed:false` filter, DB-side attempt counter |
| 2.2 | `POST /auth/otp/request`, `devCode` outside production | DONE | 202; rate limits checked before a code is generated so a flood costs one query, not a message |
| 2.3 | `POST /auth/otp/verify`, upsert user on first success | DONE | Closes the "any 6 digits" gap. One message for wrong/expired/spent/locked so nothing is enumerable. |
| 2.4 | JWT issue/verify + auth middleware | DONE | HS256 pinned; alg:none, foreign-key, wrong-issuer and no-expiry tokens all rejected |
| 2.5 | `GET /auth/me` | DONE | Stale token for a deleted account → 401, not 500 |

## Phase 3 — Cart

| id | task | status | notes |
|---|---|---|---|
| 3.1 | Cart model and line identity | DONE | Stored line is a pointer plus a count — no prices, per schema.md decision 1. Verified on disk. |
| 3.2 | Pricing/totals service — subtotal, MRP, savings, shipping threshold | DONE | Pure. Boundary tested at 49899/49900/49901 paise, plus a property check that the nudge and the charge can never disagree. |
| 3.3 | `GET /cart` | DONE | Empty cart is 200 with `lines: []`. Prices resolved live on every read. |
| 3.4 | `POST /cart/items` with increment-on-existing | DONE | Omitted qty defaults to 1; an explicit 0 is rejected. Validated against the catalogue before the cart is touched. |
| 3.5 | `PATCH /cart/items/{lineId}` — absolute qty, 0 deletes | DONE | A missing `qty` is 422, not a silent delete. |
| 3.6 | `DELETE /cart/items/{lineId}` and `DELETE /cart` | DONE | Unknown line → 404. |

## Phase 4 — Address

| id | task | status | notes |
|---|---|---|---|
| 4.1 | Address validator with the frontend's exact ordered messages | DONE | All five messages pinned verbatim, plus a walk test asserting the failure *order* matches the UI. Fields are trimmed on save. |
| 4.2 | `GET` / `PUT /me/address` | DONE | Absent address is `null`, not 404. A rejected write leaves the previous address untouched. No upsert — an address cannot conjure an account. |

## Phase 5 — Orders & payment

**Re-scoped 2026-08-07**: payment moves to **Razorpay** and **cash on delivery
is dropped** (user decision). `schema.md`, `roadmap.md` and `product.md` were
amended first — orders now exist in a `pending_payment` state holding a stock
reservation, and a signed callback or webhook is what makes them real. 5.1
predates the decision; 5.2 amends it.

| id | task | status | notes |
|---|---|---|---|
| 5.1 | Order model + `EFF-\d{6}` id generator | DONE | `internal/order`, 100% cov. `model.go` (`Order`, `Line`, `Totals`, `Payment`, `Status`) + `id.go` (`NewOrderID`/`ValidOrderID`). Snapshots decoupled from `cart` — own `Totals` type. `ShippingAddress` reuses `auth.Address`. Id is uniform over 000000–999999 via `crypto/rand` (no modulo bias, pinned by a distribution test), error-returning, stateless for the retry loop. |
| 5.2 | Amend the order model for Razorpay: new statuses, `payment` sub-document, drop COD | DONE | 100% cov. Added `StatusPendingPayment`/`StatusPaymentFailed`/`StatusExpired`; `PaymentStatus` type (created…refunded); `PaymentMethod` now reports Razorpay methods (upi/card/netbanking/wallet/emi/paylater); `PaymentCOD` deleted. `Payment` gains provider, status, amount, currency, three razorpay ids, method-detail fields (last4/network/vpa/bank/wallet), attempts, `*time.Time` capturedAt, and `*PaymentFailure` failure. `Order` gains `CreatedAt`/`ExpiresAt`; `PlacedAt` → `*time.Time`. `Validate` accepts a pending order (no method, nil PlacedAt) and rejects a placed order missing any of placedAt/method/capturedAt. Also added two cross-field checks in `Order.Validate`: (1) `Payment.Amount != Totals.Total` is rejected for every status — defence in depth per schema.md §Razorpay; (2) placed+ orders require `Payment.Status` to be `captured` or `refunded` — any pre-capture status (created/authorized/failed) is rejected. |
| 5.3 | Razorpay gateway behind an interface: create order, verify callback signature, verify webhook signature | DONE | `internal/razorpay`, 97.9% cov. `gateway.go` (Gateway interface, Client with 10s timeout, Unconfigured); `signature.go` (verifyCallbackSignature with keySecret, verifyWebhookSignature with webhookSecret). Both verifiers use `hmac.Equal`, never `==`. `NewClientWithBaseURL` allows tests to point at an httptest.Server — live API is never called from tests. `export_test.go` exposes `ClientBaseURL`/`ClientTimeout` for white-box assertions. `TestNewClient` pins the production base URL and 10s timeout. Config gains `RAZORPAY_KEY_ID`/`KEY_SECRET`/`WEBHOOK_SECRET` (required in production, optional elsewhere). Secret values never appear in errors. `TestCrossSecretRejection` proves each verifier rejects a valid signature made with the other secret. One line genuinely uncovered: the `json.Marshal` error branch in `CreateOrder` — unreachable because `CreateOrderRequest` contains only `int64` and `string` fields, which never fail to marshal. |
| 5.4 | Order repository: create, scoped reads, guarded status transition, indexes | DONE | `internal/order`, 100% cov. `repository.go` (Create, ByOrderID, ListForUser, MarkPlaced, EnsureIndexes). Two distinct sentinels for the two unique-index collisions on Create: `ErrDuplicateOrderID` (orderId_1) vs `ErrPendingOrderExists` (userId_1 partial) — classified by reading `keyPattern` from `WriteException.WriteErrors[0].Raw` (exact key lookup on "orderId"/"userId"), not by index-name substring. Tests use `mongotest.Reply` with `ok:1` + `writeErrors` + `keyPattern` so the driver produces a real `WriteException`. MarkPlaced takes `now time.Time` parameter (caller-supplied, deterministic in tests); tests assert `$set.updatedAt` equals the value passed, not a wall-clock instant. ByOrderID carries both orderId and userId in a single query. ListForUser excludes expired via `$ne`, sorts by createdAt desc. All 6 indexes with partial filter expressions per schema.md §Index creation. |
| 5.5 | `POST /orders`: ordered preconditions, expire any prior pending order, reserve stock, create the Razorpay order | DONE | `internal/order`: service.go + handler.go. Insert-first: order inserted without `razorpayOrderId` (omitempty avoids partial-index collision), then gateway.CreateOrder, then SetRazorpayOrderID. On any post-insert failure → releaseLines + ExpirePendingForUser → 502 GATEWAY_ERROR. ErrPendingOrderExists from Create returns reserved stock immediately → 409. Six-step precondition sequence enforced (empty-cart → blocking-line → address → expire-prior → stock → Razorpay). `Store` interface for fake injection in tests. `o.Validate()` called before repo.Create; failure is an internal 500 and releases stock. `Payment.Validate` relaxed for `PaymentStatusCreated` (empty RazorpayOrderID is valid before the gateway link). `internal/order`: 98.4% cov; total: 96.9%. |
| 5.6 | `POST /orders/{orderId}/payment/callback` | DONE | `ConfirmPayment` in service.go. Six-step guard: fetch → idempotent-placed-200 → not-pending-422 → signature-verify → razorpayOrderId-match → MarkPlaced. Webhook-beat-us (modified=false) re-fetches via ByOrderID. Cart cleared best-effort after successful transition. ErrSignatureInvalid used for all three verification failures so no security leak. Handler: empty body / missing fields → 422 field errors; ErrOrderNotFound → 404; ErrSignatureInvalid / ErrOrderNotPending → 422 "We could not verify that payment." `CartClearer` interface added; `*cart.Service` satisfies it in production. `providerRazorpay` const extracted (was at 4-occurrence threshold). `internal/server`: 100% — `TestAllHandlersAreRegistered` exercises auth, cart and order route registration. Unreachable dead code (not tested, by design): `userID !ok` body + the matching `return` in each handler (auth middleware always runs first); `genErr != nil` in the ID retry loop (analogous to json.Marshal in razorpay — crypto/rand.Read cannot fail in practice). |
| 5.7 | `POST /webhooks/razorpay`: raw-body HMAC, `payment_events` dedupe, capture/fail transitions | DONE | New files: `internal/order/events.go` (PaymentEvent, ErrDuplicateEvent, PaymentEventStore interface), `internal/order/events_repository.go` (PaymentEventsRepository.InsertEvent, IsDuplicateKeyError → ErrDuplicateEvent), `internal/order/webhook.go` (ErrWebhookSignatureInvalid, HandleWebhook, handleWebhookCapture, handleWebhookFailed). Modified: `service.go` (Store interface +ByRazorpayOrderID +MarkPaymentFailed; ServiceConfig/Service +Events; NewService), `repository.go` (+ByRazorpayOrderID +MarkPaymentFailed; bsonOpSet constant for goconst), `handler.go` (RegisterWebhook, razorpayWebhook — raw io.ReadAll before sig verify), `server/router.go` (RegisterWebhook on root r, outside /api/v1). Route: `POST /webhooks/razorpay` on root (no auth). Signature verified over raw bytes before any unmarshal (nolint:nilerr for deliberate swallow). Dedupe: insert-first (event id as _id). Amount assertion on capture. 200 for verified events including unknown/dup/order-not-found; 400 only for missing/invalid signature; 500 only on genuine DB failure. Stock released on payment.failed only when modified=true. Cart cleared best-effort using userID from resolved order. `make lint` clean. `internal/order`: 98.3%; total: 97.0%. |
| 5.8 | Reservation sweeper: expire abandoned pending orders and return their stock | DONE | New files: `internal/order/sweeper.go` (`SweepStore` interface, `Sweeper`, `NewSweeper`, `SweepOnce`, `Run`, `sweepReleaseLines`). Modified: `repository.go` (`FindExpiredPending` uses `status_1_expiresAt_1` index, `MarkExpired` guarded update); `config/config.go` (`SweeperInterval time.Duration`, default 1 min); `cmd/api/main.go` (goroutine: `go sweeper.Run(ctx, cfg.SweeperInterval)`). Write order is the entire point: guarded `MarkExpired` first, `ReturnStock` only when `modifiedCount == 1`. No TTL index. `Run` is tick-based, logs errors and continues so one bad sweep cannot kill the loop; stops cleanly on ctx cancel. Batch-bounded per tick (100). Tests: empty sweep; modifiedCount==0→no stock returned (the critical test); modifiedCount==1→all lines released; ReturnStock failure continues to next line; find error surfaces; mark error surfaces; Run stops on cancel; Run logs errors and continues. Repository tests assert the query filter and the update guard. `internal/order`: 98.1%; total: 97.0%. |
| 5.9 | `GET /orders` and `GET /orders/{orderId}` | DONE | Added `ListForUser` to `Store` interface; `ListOrders` and `GetOrder` service methods (thin pass-through, no business rules). New handlers: `listOrders`, `getOrder` in `handler.go`; new DTOs `listOrdersResponse`, `getOrderResponse`. Routes: `GET /api/v1/orders`, `GET /api/v1/orders/{orderId}`. Reuses existing `toPlacedOrderDTO`, `toOrderLineDTOs`, `toTotalsDTO` helpers. Pending orders: `placedAt`/`payment`/`etaText` absent via `omitempty` on nil pointers — never zero time or empty string. Other user's order: `ByOrderID` scopes by userId+orderId in one query; both cases return `ErrOrderNotFound`; handler returns identical 404 body. `TestOrderRouteRejectsWrongMethod` updated (GET is now registered; test uses PUT instead). Router test extended with GET /orders and GET /orders/{orderId} checks. `internal/order`: 97.9%; total: 96.9%. Phase 5 complete. |

## Phase 6 — Hardening

| id | task | status | notes |
|---|---|---|---|
| 6.1 | OTP rate limiting (3 / 10 min, 1 / 30 s) | DONE | Landed with 2.2 rather than deferred; enforced before code generation |
| 6.2 | CORS from `ALLOWED_ORIGINS` | DONE | `internal/httpx/cors.go` (`CORS` middleware): exact-match allowlist, Vary: Origin always set when Origin present, wildcard never emitted, credentials not enabled. Preflight (OPTIONS + ACRM) returns 204 without reaching any handler. Disallowed origin: handler runs, no CORS headers. No-origin request: untouched. Wired as the outermost wrapper (`httpx.CORS(cfg.AllowedOrigins)(r)`) in `internal/server/router.go` so 404/405 fallback handlers also carry CORS headers. Config field already existed; not re-added. Unit tests in `internal/httpx/cors_test.go` (4 branch-covering tests + wildcard assertion); integration tests in `internal/server/cors_test.go` (7 tests through the real router). `internal/httpx`: 100%; `internal/server`: 100%; total: 97.0%. |
| 6.3 | Graceful shutdown with 15s drain | DONE | Landed with the Phase 0 wiring rather than deferred; the row was simply never updated. Verified live on 2026-08-08: SIGTERM logged `shutdown signal received, draining` with `grace=15s`, then `shutdown complete`, with `cfg.ShutdownGrace` bounding the drain in `cmd/api/main.go`. |
| 6.4 | CI workflow running lint + coverage gate | DONE | `.github/workflows/ci.yml`: triggers on push + PR; pins Go 1.25.5 (from go.mod) and golangci-lint v2.12.2 (matches local install); `actions/setup-go@v5 cache: true` caches module + build caches; installs golangci-lint to `$GOPATH/bin` so the Makefile's binary check passes; steps: Install golangci-lint → `make lint` → `make test` (race, separate step so data races are distinguishable from coverage failures) → `make cover` (90% gate enforced inside the target, not duplicated in workflow) → upload `coverage.out` artefact with `if: always()`. Suite verified offline with `env -i` stripping all secrets and MONGO_URI — every package passed cleanly. No Makefile changes needed. |

## Phase 7 — Frontend integration

| id | task | status | notes |
|---|---|---|---|
| 7.1 | API client in the Next.js app, paise→rupee at the boundary | TODO | replaces `lib/shop/data.ts`. **Also: the grid must move from 4 cards with size pills to 9 flat cards** — the frontend no longer matches the model. |
| 7.2 | Move cart/auth state from `useReducer` to server-backed calls | TODO | fixes "cart lost on reload" |

---

## Discovered along the way

Things found mid-task that are not part of the task in hand.

| found in | item |
|---|---|
| 0.1 | Frontend holds money as whole rupees; API uses paise. Conversion needed at the boundary — tracked as 7.1. |
| 0.1 | Frontend OTP accepts any 6 digits (`product.md` §3.3). Real check lands in 2.3. |
| 0.6 | `mongodb.Disconnect` was not idempotent — a second call returned `client is disconnected`. The fake caught it; now treated as success, since a deferred close plus an explicit one is a normal shutdown shape. Fixed in place. |
| 0.6 | golangci-lint hides all but 3 occurrences of an issue by default; `max-same-issues: 0` set so systemic problems are not silently truncated. |
| 1.1 | Slugging is lossy, so a `lineId` cannot be reversed into a variant label. Resolution must go product → `VariantBySlug`, never by un-slugging. `Product.Validate` rejects two labels that slug alike so ids stay unique. |
| 1.2 | Schema had never been written down. `schema.md` now documents all six collections, their indexes and four modelling decisions (cart stores no prices; orders snapshot everything; product `_id` is a readable string; one cart/address doc per user). |
| 3.4 | Cart writes are read-modify-write; one shopper on two devices could lose an add. **Closed by the user — not an MVP concern.** |
| M.5 | Splitting sizes into independent products **removed** a whole collection and the id-mapping layer with it. Worth noting: the earlier variants split was the right call *for embedded variants*, but once each size is a product the same atomicity comes free. |
| M.5 | Sibling cross-links (`GET /products/{id}` → `siblings`) exist so the relationship between sizes is not lost when they become independent. Nothing about buying depends on `family`. |
| M.1 | Orders keep duplicating name, label and price. Raised as "repetition" but it is a deliberate snapshot: referencing a `variantId` alone would let tomorrow's price change rewrite last week's receipt. `variantId` is kept alongside for traceability only. |
| M.3 | Addresses stayed **embedded** rather than becoming their own collection: only ever read with their owner, and a handful per user. A collection would add a join to every checkout for nothing. Variants were different — stock needs atomic per-variant writes. |
| M.5 | Migrating live Atlas needed a manual drop of the `variants` collection and all old product documents — `Seed` upserts but never removes superseded shapes. |
| M.2 | Live Atlas carried a stale `products.form_1` index from the old schema alongside the new `form_1_position_1`. Dropped. `EnsureIndexes` creates but never removes, so superseded indexes need a manual drop. |
| M.2 | Re-seeding must not reset live inventory: `stock` is under `$setOnInsert`, and variants are matched on `(productId, label)` so their generated ids survive a re-seed. Pinned by a test. |
| 3.2 | Shipping must be decided from the **subtotal**, never the MRP total — otherwise a heavily discounted cart earns free delivery it has not paid for. Pinned by a test. |
| 3.1 | A cart line whose variant has left the catalogue is **dropped** on read rather than priced at zero. A vanished line is visible to the shopper; a ₹0 charge would not be. |
| 2.1 | **Corrected my own `schema.md` decision.** It said unsalted SHA-256 was fine for OTP codes because they are short-lived. That was wrong: the keyspace is 10^6, so anyone able to read `otp_codes` could reverse every live code instantly and sign in as anybody. Now HMAC-SHA256 keyed with a server-side pepper and domain-separated by phone number. `schema.md` updated. |
| 2.4 | `gosec` G101 fires on any constant whose name contains "token"; renamed `tokenIssuer` → `issuerName` rather than adding a suppression for a false positive. |
| 2.3 | `govet` shadow check caught an inner `err` shadowing the outer one in `Verify` — harmless today, but exactly the shape that swallows an error later. |
| 1.4 | **gorilla/mux loses the method-mismatch signal**: a later route failing on its *path* matcher clears it, so only the last-registered route ever answers 405. `POST /api/v1/products` returned 404. Fixed generally in `server.methodAware`, which re-matches the path under the other verbs. Reproduced in isolation first, so it is a mux behaviour and not our wiring. |
| 1.4 | A subrouter inherits neither `NotFoundHandler` nor `MethodNotAllowedHandler` from its parent; both are now set on `/api/v1` explicitly. |
| 1.3 | The driver sends bulk write payloads as OP_MSG **document sequences**, not inside the command body — `mongotest` now parses section type 1 so write assertions are possible at all. |
| 1.1 | `ParseFilter` treats form matching as case-sensitive (`powder` is rejected, `Powder` accepted), since the roadmap types the values as exact literals. Loosen it in 1.4 if the frontend ever sends lowercase. |
| 5.1 | The dispatch prompt named the snapshot line type `OrderLine`, but repo lint (revive) forbids the `order.OrderLine` stutter and cart already uses `cart.Line`. Named it `order.Line` to match — a naming choice, not a contract change. |
| 5.x | **Razorpay decision, 2026-08-07.** Payment moves to Razorpay Checkout; COD dropped. Contracts amended first, in order: `schema.md` (decision 6, `orders` rework, `payment_events`, §Razorpay), `roadmap.md` (§Orders rewritten, `GATEWAY_ERROR` added), `product.md` (§3.5 rewritten). Phase 5 grew from 4 tasks to 9. |
| 5.x | The frontend's checkout screen still renders its own UPI/card fields and a COD option (`components/shop/CheckoutScreen.tsx`, `lib/shop/data.ts` `PAY_OPTIONS`). All of it is dead under Razorpay — folded into task 7.1, which already owns the 4-cards→9-cards rework. |
| 5.1 | ~~`payment.last4` null-vs-empty~~ — **resolved by 5.2.** The whole `Payment` struct was replaced; method-detail fields are now absent until Razorpay reports them, and the empty→`null` mapping happens at the JSON edge. |
| 6.2 | `Vary: Origin` is set only when an `Origin` header is present, so a response to a **no-Origin** request carries no `Vary` at all. Strictly it should: the same URL does return different headers depending on `Origin`. Accepted deliberately — the unsafe direction is protected (an allowed-origin response always carries `Vary`, so a cache cannot serve it to a different origin), the only failure mode is a shared cache causing a *false block*, and there is no shared cache in front of this API. One-line fix (hoist the `Vary` set above the empty-origin return) if a CDN is ever put in front. |
| 5.8 | `FindExpiredPending`'s `cursor.All` decode-error branch is uncovered (that function sits at 77.8%). Reaching it needs a cursor that succeeds on `Find` but fails **during** document streaming, which `mongotest` cannot currently produce. Accepted: `internal/order` is 98.1% overall. Revisit if `mongotest` ever grows a streaming-error capability — the same branch in `ListForUser` would benefit. |
| 5.4 | **Testing a duplicate key needs `mongotest.Reply`, not `Fail`.** `Fail` sends `ok:0`, which the driver turns into a `CommandError` carrying no `WriteErrors` — so a dup-key test written with it silently exercises the wrong path and proves nothing. A real E11000 arrives as `ok:1` with a `writeErrors` array, which is what makes `WriteException.WriteErrors[0].Raw` (and its `keyPattern`) available at all. Applies to any future write-error test. |
| 5.2 | `.env.example` still documents `MONGO_TEST_URI`, which `internal/mongotest` made unnecessary (see the 0.6 rows). Harmless but misleading to a newcomer — clean up whenever `.env.example` is next touched. **Resolved in 5.3: `MONGO_TEST_URI` comment now clarifies the var is optional and tests skip without it; the key is retained so the mechanism is visible.** |

---

## Needed from the user

| what | why | status |
|---|---|---|
| ~~MongoDB Atlas SRV URI~~ | supplied and working; catalogue seeded, indexes created | resolved |
| ~~Database name~~ | `enerzia` | resolved |
| ~~Atlas IP allowlist~~ | confirmed working from this machine | resolved |
| ~~A separate test database~~ | ~~`MONGO_TEST_URI`~~ — no longer required; `internal/mongotest` fakes the server | resolved |
| JWT signing secret | can be generated locally — no need to source one | self-serve |
| OTP delivery provider (WhatsApp or SMS) | **decision open** — neither is free; see handoff.md. `LogSender` covers development, `UnconfiguredSender` fails loudly in production | **waiting** |
| **Razorpay `KEY_ID` + `KEY_SECRET`** (test mode) | needed to create a Razorpay order at all; test mode moves no real money | **waiting** — 5.3 ships against a fake until they land |
| **Razorpay `WEBHOOK_SECRET`** | a **different** secret from KEY_SECRET, set when the webhook is added in the Razorpay dashboard. Verifying one with the other fails in a way that looks like a forged request | **waiting** |
| A publicly reachable URL for the webhook | Razorpay must be able to POST to `/webhooks/razorpay`; a tunnel is fine for development | **waiting** — only blocks the live smoke test, not 5.7 |
