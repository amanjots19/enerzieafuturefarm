# Session handoff

How we work, where things stand, and what to do next.

Last updated 2026-08-14, after the homepage was built, the site was rethemed,
the brand mark was extracted from the logo, and delivery pricing moved to ₹50.

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

How we work. Two modes, and I will tell you which:
  - DISPATCH: you write ONE prompt for the next task, I paste it to a subagent,
    I paste the report back raw, you VERIFY yourself and accept or fix up.
  - DIRECT: you do the work yourself. I use this for design and UI, where the
    round trip through a subagent costs more than it saves.

Either way you verify yourself. Never trust a report — re-run `make check`, and
open a REAL BROWSER for any frontend change.

Rules that matter most:
  - one task at a time, finished completely (code + tests + lint + docs)
  - 90% coverage across ./internal/... , enforced by `make check`. Never lower
    it, never weaken a test to hold it up
  - Mongo is mocked with internal/mongotest, never skipped on a missing database
  - money is int64 paise, never float; prices are server-authoritative
  - contract changes are MY decision — if a task needs roadmap.md or schema.md
    changed, stop and ask me
  - ANOTHER AGENT is building the admin catalogue in this same repo. Read the
    ownership boundary in tasks.md before touching shared code

Servers: frontend on :3100 (`cd Enerzia && npm run dev`), API on :8080
(`cd enerzia-be && make run`). NOT :3000 — see the handoff for why.

Before starting, run `make check` in enerzia-be/ and `npm run typecheck` in
Enerzia/, and tell me both results.
````

---

## Where things stand

**Backend.** Phases 0–6 complete. `make check` green: total coverage 96.1%,
every package above the 90% floor. Delivery is ₹50 below ₹499 and free at or
above — `ShippingFee`/`FreeShippingThreshold` in `internal/cart/pricing.go` are
the only place that rule exists.

**Storefront.** Phase 7 wiring complete. Phase 9 account area complete: header
menu, orders, saved addresses, contact. Phases 9.1–9.7 all DONE in tasks.md.

**Built this session:**

- **Homepage at `/`** — it no longer redirects to `/shop`. Hero, "Living with
  it" moments, tips, proof seals, real testimonials, FAQ accordion, all fed from
  `Enerzia/lib/content/`. The `#farm` and `#benefits` sections are deliberately
  NOT built; they get their own pages.
- **Legal/info routes** — `/terms`, `/privacy`, `/faq`, `/contact`, all real
  server-rendered pages with their own metadata, because payment-gateway
  onboarding needs public URLs for them.
- **Site footer** in the root layout, so it and the announcement bar appear on
  every route.
- **One shared header.** `SiteHeader` is used by both the homepage and the shop;
  the shop passes its account control and cart in a `right` slot. The nav is a
  single link — the homepage offers "Shop", the shop offers "About us".
- **Sitewide retheme** to the Homepage design's cream + gold + deep green.
- **Brand mark extracted from the logo** — see the logo section below.
- **Mobile touch-target pass** — fourteen controls were under 44px.

**Servers.** Frontend `:3100`, API `:8080`. Both up at time of writing.

**Git.** 3 commits ahead of `origin/main`, unpushed. 71 modified files and 26
untracked paths in the working tree — a mix of mine and the catalogue agent's.
**Nothing from this session is committed.**

---

## Two agents, one repo

The other agent builds the **admin catalogue**. Theirs:

- `enerzia-admin/` (their frontend), `enerzia-be/internal/admin/`,
  `enerzia-be/internal/cloudinary/`, `enerzia-be/cmd/adminpass/`
- catalogue **writes**: `internal/catalogue/admin_*.go`, `validate.go`

Mine: everything in `Enerzia/`, `internal/{cart,order,msg91}`, shopper auth.

Shared and therefore contested: `product.md`, `roadmap.md`, `schema.md`,
`tasks.md`, `internal/server/router.go`, `internal/config/config.go`. Re-read
before editing; never commit their files with yours.

---

## The live catalogue is not yours to change

The site is live and there is now a **catalogue manager** — a person, not an
agent — who decides what the shop sells. Prices, product copy, stock, images
and which products are active are theirs, set through the admin console.

**Do not write product data.** Not through the seed, not through a one-off
script, not through Atlas directly. This holds even when the catalogue looks
wrong to you:

- A product missing from the shop is far more likely to have been **retired on
  purpose** than lost. It is not yours to restore.
- A price that looks like a typo (`tablets-120` at ₹798.90) is a pricing
  decision until the catalogue manager says otherwise.

Report what you notice, and let them decide. Restoring a "missing" product
silently overrides a human decision, and the shop starts selling something
nobody chose to sell.

### What this means in code

`internal/catalogue` has five writers to the products collection. Only one was
ever a hazard:

| writer | caller | verdict |
|---|---|---|
| `TakeStock` / `ReturnStock` | order placement | fine — inventory moving on a real sale, never touches price or active |
| `Create` / `Update` / `Retire` | admin console | fine — this *is* the catalogue manager's tool |
| `Seed` / `SeedOverwrite` | `cmd/seed` | the hazard |

So `cmd/seed` **refuses to run when `APP_ENV=production`**, and `Seed` is
insert-only everywhere else. It is a development bootstrap for an empty
database, nothing more. `SeedOverwrite` (`seed --overwrite`) still resets
products to code values, which is why it must never reach production either —
the refusal covers both.

If you find yourself wanting to seed production, the answer is the admin
console.

---

## What to do next

### 1. Commit and push

Nothing from this session is committed and there are 3 unpushed commits behind
it. This is the biggest risk on the board — a day of work sits only in the
working tree. Commit the `Enerzia/` work as its own commit; leave the shared
docs and the catalogue agent's files for them.

### 2. Finish the brand mark

The header lockup is the swirl (extracted from the logo) beside "Enerzeia /
Future Farm" set in **Rubik 600 italic**. That is the third font tried; the
owner has not approved it. Nunito and Poppins were both rejected.

If Rubik is wrong too, stop guessing at fonts — the remaining rounded faces
(Baloo 2, Quicksand, Comfortaa) have **no true italic**, so matching the slant
would need a synthetic skew. The exact-match route is to extract the wordmark
from the logo the same way the swirl was extracted (the letters are separate
ink components from the oval) and render it as an image.

### 3. Outstanding cleanups

- **`playwright` was added to `Enerzia/package.json`** by a subagent as a side
  effect of its own testing, never reported. There is no test script or config
  to go with it. Adopt it deliberately or back it out.
- **Dead files**: `Enerzia/components/shop/ContactScreen.tsx` and
  `Enerzia/lib/shop/contact.ts` — nothing imports either since `/contact`
  became a real route.
- **`enerzia-be/.env.bak-1786611601`** — a stray backup, untracked.
- **Unused asset**: `Enerzia/public/assets/enerzeia-mark.svg`, the outline trace
  the owner supplied. Not wired in; see the logo section.
- **`tasks.md` rows 7.14, 7.15, 9.x** are current. 9.4 is still TODO in the
  table although the work landed — reconcile.

### 4. Product names still contain em-dashes

The owner asked for em-dashes to become plain hyphens. Done everywhere the
frontend owns — zero remain in rendered text on `/`, `/contact`, `/terms`,
`/privacy`, `/faq`. But **every product name still has one** (`Pure Spirulina
Powder — 100 g`) because those come from Mongo, not the frontend. Fixing them
needs a seed change in `internal/catalogue`, which is the catalogue agent's
area. En-dashes were deliberately left (`Haryana – 124113`, `Mon–Sat`, `3–5 g`).

### 5. Before launch

- Purge test data from live Atlas (task 7.11) — test users, throwaway phone
  numbers, smoke-test orders, decremented stock.
- Razorpay live keys, and **add the deployed origin to `ALLOWED_ORIGINS`** —
  see the CORS trap below; getting this wrong looks exactly like an outage.
- Real photography. Every image on the homepage is a gradient placeholder;
  only `powder-100g` has a real product photo.
- The PDP's "4.8 · 312 reviews" is hardcoded, not a real aggregate
  (`product.md` §5 records there is no reviews feature). Decide before launch.

---

## Things that will bite whoever picks this up

**Port 3000 is taken by Colima.** `~/.colima/_lima/colima/ssh.sock` binds
`127.0.0.1:3000`. Next binds the IPv6 wildcard, so both can hold "port 3000" at
once: `localhost:3000` and `[::1]:3000` reach Next, `127.0.0.1:3000` reaches
Colima. The dev script is pinned to `-p 3100` to end the ambiguity. Do not move
it back without checking Colima.

**Changing the frontend port silently breaks the app.** `ALLOWED_ORIGINS` in
`enerzia-be/.env` is an explicit allow-list. Move the frontend and the API still
returns **200 with no `Access-Control-Allow-Origin` header**; the browser
discards every response and the shop shows "Unable to reach the server". That is
indistinguishable from a dead backend and cost a full debugging round. The list
now holds `:3000`, `:3001` and `:3100`.

**`next/image` caches by URL, and so does the browser.** Regenerate an image
under the same filename and BOTH layers keep serving the old one — clearing
`.next/cache/images` is not enough. This looked exactly like a bad image crop
for a whole round. **Change the filename when you regenerate an asset.**

**A margin cannot offset a centred flex item.** `.site-header-inner` is
`align-items: center`; adding `margin-top` changes the item's outer height and
the centring gives back half of it, so a 13px margin moved the nav 6.5px. Use
`position: relative; top:` if you need a real offset.

**The browser pane throttles when hidden.** Transitioned properties stay at
their start values indefinitely while untransitioned ones apply instantly — so
a hover looked broken (`transform` identity, `z-index` applied) when the CSS was
correct. Disable the transition and re-measure before believing a defect. The
pane also collapses to `innerWidth: 0` sometimes, which makes every measurement
nonsense; check the viewport before trusting numbers.

**Touch targets are keyed off the pointer, not the viewport.** The 44px rules
live in `@media (pointer: coarse)`. A `max-width` query would miss a large touch
tablet and would shrink targets on a narrow desktop window. Do not "simplify" it.

**MSG91 sign-in `418` is not a rejected token.** It means the caller was refused
before the token was read — usually the IP. The client is pinned to IPv4 for
this reason. `201` is success, `701` is an invalid token.

**Product `grad` gradients come from MongoDB**, not the frontend. They are
product art, not theme, and a retheme must leave them alone.

**The gold accent is not a text colour.** `--color-accent-2` (#b8933c) on cream
does not reach 4.5:1. Gold text must use `--color-accent-2-600/700` (#735a24),
which measures 6.15:1. The same rule will apply to whatever replaces it.

---

## The logo

`Enerzia/public/assets/`:

- **`enerzeia-logo.png`** (1136×730) — the full brand logo: swirl, wordmark and
  enclosing oval, flattened, white background, **no alpha**.
- **`enerzeia-brandmark.png`** (318×395, RGBA) — the swirl alone, extracted from
  the above and used in the header and footer.
- **`enerzeia-mark.svg`** — supplied by the owner, **not used**. It is a
  monochrome outline auto-trace of the whole lockup: hollow letterforms, black
  only, illegible at header size, invisible on the dark footer panel.

**How the mark was extracted, and why a crop will not work.** The swirl's
leftmost point is x=105; the oval's rightmost point in the same band is x=100.
Any vertical cut that clears the ring also slices the mark — a first attempt at
x=118 clipped it, which the owner spotted. It was extracted as a **connected
component**: flood-fill from a seed at (105,380) gives 42,261 px bounded at
x[105,416] y[168,556], provably disjoint from both the oval and the wordmark.
White was knocked out to alpha with the colour un-premultiplied so antialiased
edges keep the gradient instead of fringing.

**If a proper vector arrives**, swap the `src` in `components/BrandLockup.tsx`
and delete the extraction note. What would actually help: the swirl on its own,
transparent, ideally SVG.

**The logo's colours are deliberately NOT the site palette.** The logo is blue
(#003078) → teal (#007890) → green (#7cba4d). The site is deep green + gold on
cream, from the Homepage design. The owner confirmed on 2026-08-14 that the site
palette stays and only the wordmark follows the logo. A full logo-matched
retheme was built and reverted. **Do not "fix" this.**

---

## What verification has actually caught

Every one of these arrived in a report marked done or passing, or in my own
work. This is the argument for verifying rather than trusting.

| Where | What |
|---|---|
| 7.13 | `trustLoaded` cleared the banner unconditionally, so a race wiped the error AND the Retry button — an empty shop with no explanation |
| 9.5 | `.pdp-thumb-btn--active` used `var(--brand)`, which is undefined; the whole `outline` shorthand was dropped, so the selected thumbnail had no indicator at all |
| 9.5 | The `onError` fallback mutated the DOM directly; React never reset it, so one failed image left the PDP stuck on a gradient forever |
| 9.2 | Order id truncated to `#F-118531`, and a CSS gradient printed as visible text — both hidden by a mocked fixture, both obvious against real data |
| 9.6 | A `pending_payment` order showed "Total paid ₹449" next to "Payment pending" |
| Webhook | Events were marked processed before processing — a database hiccup meant money taken and the order silently expired |
| 7.14 | Fourteen controls under 44px, including the shop's category filters at 29px and the PDP quantity stepper at 34×34 |
| This session | The "clipped" brand mark was a stale `next/image` cache, not a bad crop — the asset was provably complete |

**And four times a "failing check" was my own bad measurement**, not a real
defect: stale DOM references, reading state in the same tick as a click, a
collapsed browser pane reporting `innerWidth: 0`, and a throttled page where
transitions never advanced. A failing check is not automatically a failing
implementation — confirm the measurement before reporting a bug.

---

## Mocking the endpoint you are verifying proves nothing

Task 9.2 passed against a mocked fixture and shipped two visible defects that
real data exposed in seconds. Verify against the live API.

When you genuinely need a state the live system will not produce — a backend
outage, a partial failure, a second product image — put a **proxy** in front of
the real API rather than stubbing the client. `scratchpad/proxy*.js` from this
session forwarded to `127.0.0.1:8080` and altered exactly one thing: a forced
500 on `/products`, or an extra image appended to one product. Everything else
stayed real. Point `NEXT_PUBLIC_API_BASE_URL` at the proxy, and **restore it
afterwards**.

---

## Credentials

Razorpay **test** keys and the webhook secret are in `enerzia-be/.env`, working.
MSG91 widget id and token are in `Enerzia/.env.local` and are client-side by
design; the account auth key is server-side only.

Support address is `support@enerzeiafuturefarm.com` — confirmed by the owner as
a mailbox they own and monitor. Two malformed variants appeared in the supplied
legal text (`gamil.com`, and a doubled `.com`) and were corrected.

Live keys are **not** in `.env` and must never be pasted into a chat — put them
straight into the deployed environment.
