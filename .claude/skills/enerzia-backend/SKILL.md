---
name: enerzia-backend
description: Rules and workflow for developing the Enerzia shop backend (Go + gorilla/mux + MongoDB) in enerzia-be/. Use whenever writing, changing, reviewing, testing, or planning backend code — new endpoints, handlers, repositories, models, migrations, or tests. Also use when the user asks "what's next" on the backend or references product.md, roadmap.md, or tasks.md.
---

# Enerzia backend development

Governs everything under `enerzia-be/`. The frontend (Next.js, repo root) is a
separate concern — do not change it from a backend task without saying so.

## 1. Load session knowledge first

Before writing or changing any backend code, read all three, in this order:

1. `enerzia-be/product.md` — what the product is, what each frontend screen
   does, and the domain rules the API has to honour.
2. `enerzia-be/roadmap.md` — the endpoint contracts: method, path, auth,
   request payload, response shape, status codes. This is the source of truth
   for the API surface.
3. `enerzia-be/schema.md` — the MongoDB collections, document shapes, indexes,
   and the modelling decisions behind them. Read before touching any
   repository, and update it in the same change as a shape or index change.
4. `enerzia-be/tasks.md` — the backlog, what is in progress, what is done.
5. `enerzia-be/handoff.md` — how we agreed to work, decisions still open, and
   anything left mid-flight by the previous session.

Never infer a contract from memory or from handler code alone. If the code and
`roadmap.md` disagree, stop and raise it — one of them is a bug, and which one
is the user's call.

If a task requires a contract that `roadmap.md` does not define, add the
definition to `roadmap.md` first, then implement it.

## 2. One task at a time

Work the smallest next unit in `tasks.md` and finish it completely — code,
tests, lint, docs — before starting another. Do not batch several tasks into
one change.

"Completely" means all of:

- the endpoint behaves as `roadmap.md` specifies, including error cases
- tests pass and coverage stays above the bar (§3)
- lint and vet are clean (§4)
- `tasks.md` is updated: move the task to Done, add anything discovered

If a task turns out to be bigger than one unit, split it in `tasks.md` and do
the first piece. If you find an unrelated problem mid-task, write it into
`tasks.md` as a new item rather than fixing it inline.

State which task you are starting, and report against that same task when done.

## 3. Test coverage above 90%

Non-negotiable floor: **90% statement coverage across `./internal/...`**,
enforced by the build.

```bash
cd enerzia-be && make cover
```

`cmd/` is excluded: it is process wiring (signal handling, `ListenAndServe`)
with no branching worth asserting. The gate is the total across the measured
packages; `make cover` also prints the per-package table, and **any package
below 90% needs a one-line reason recorded in `tasks.md`** so a weak package
cannot hide behind a healthy total.

Rules:

- Write tests in the same change as the code, never "later".
- Table-driven tests are the default style.
- Cover the failure paths: bad payloads, missing auth, not-found, conflicting
  state, database errors. Happy-path-only tests hit the number without earning
  it.
- Handler tests go through the real router via `httptest`, so routing, method
  matching and middleware are exercised — not the handler function in isolation.
- **Mongo is mocked, not skipped.** Use `internal/mongotest`, the in-process
  fake that speaks the wire protocol, so the whole suite runs offline with no
  Atlas connection and no `t.Skip`. `mongotest.Start(t)` gives a URI a real
  `mongo.Client` can connect to; `FailCommand` drives failure paths such as a
  rejected handshake or a primary that goes away mid-request. Never gate a test
  on `MONGO_TEST_URI` — a skipped test is an untested code path wearing a
  disguise.
- The fake is not a database: it stores nothing and runs no queries. When a
  repository test needs real query semantics, assert the *command* the
  repository issued rather than pretending to read data back, and note in
  `tasks.md` if genuine query behaviour still needs a smoke test against Atlas.
- Do not add `//coverage:ignore`-style escapes or pad coverage with tests that
  assert nothing.

If coverage cannot reach 90% for a genuine reason, say so explicitly and stop —
do not silently lower the threshold in the Makefile.

## 4. Linting must be clean

```bash
cd enerzia-be && make lint
```

Runs `gofmt -l`, `go vet`, and `golangci-lint run`. Zero findings before a task
is done. Fix the cause; do not add `//nolint` without a comment on the same
line explaining why the rule is wrong here.

## 5. Code conventions

- **Layering**: `handler → service → repository`. Handlers do HTTP only
  (decode, validate shape, write response). Services hold business rules.
  Repositories hold MongoDB queries. No layer skips another; no Mongo types
  above the repository.
- **Money is int64 minor units (paise)**. Never float. Format for display at
  the edge only.
- **Prices are server-authoritative.** Never trust a price, total, or discount
  sent by the client — look it up and recompute.
- **Errors**: return wrapped errors (`fmt.Errorf("...: %w", err)`) internally;
  map to the API error envelope in the handler layer only. Never leak driver
  or database errors to the client.
- **Context**: every repository and service call takes `context.Context` as its
  first parameter and honours cancellation.
- **No global state**. Dependencies are struct fields, injected at wiring time
  in `cmd/api/main.go`, so tests can substitute fakes.
- Exported identifiers get doc comments beginning with the identifier name.

## 6. Secrets and config

- All configuration comes from environment variables, loaded once into a typed
  config struct at startup. No `os.Getenv` scattered through the code.
- `.env` is git-ignored and must never be committed, printed, logged, or pasted
  into a response. `.env.example` holds the key names with empty or dummy
  values and is committed.
- Never log a connection string, token, OTP, or payment detail.
- If a task needs a credential that is not present, ask the user for it — do
  not invent a placeholder and carry on as if it works.

## 7. Definition of done

A backend task is done when, and only when:

- [ ] behaviour matches `roadmap.md`
- [ ] `make lint` clean
- [ ] `make cover` passes (≥90%)
- [ ] `make test` green
- [ ] `tasks.md` updated
- [ ] the change is reported honestly, including anything skipped and why
