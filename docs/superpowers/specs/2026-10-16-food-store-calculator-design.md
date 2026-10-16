# Food Store Calculator: Design Spec

> **Revision (2026-10-16, after implementation review):** the Red-set rule was simplified to its literal reading: if a Red set was ordered in the last hour, no further Red order is accepted, from anyone. Customer identity (the `cid` cookie, the `customer_id` column, and the "holder may re-order" behavior) was removed. Sections below describe the current design.

Date: 2026-10-16
Source: `Code Homework Test.pdf` (scored on readability, maintainability, extensibility, logic)

## 1. Goal

A food store price calculator. A Go Fiber API calculates order prices with
discounts, validates member cards against a members table, and enforces a
one-customer-per-hour limit on rare products (Red set). Discount and limit
conditions are stored in Postgres tables. A Next.js UI lets a customer pick
quantities, enter an optional member card and see the price breakdown. Built
test-first. Runs with `docker compose up`.

## 2. Requirements (from the PDF)

- Menu of 7 items (THB per set): Red 50, Green 40, Blue 30, Yellow 50, Pink 80, Purple 90, Orange 120.
- Orders may contain multiple items.
- Pair discount: each pair of the same colour among Orange, Pink, Green gets 5% off that pair. Orange x2 = (120+120) - 5%. Pink x4 = two discounted pairs. Green x3 = one discounted pair + one full-price set.
- Member discount: 10% off the total when a member card number is entered.
- Red set: only one customer may order it within 1 hour. Another customer in that window is rejected.
- API: product list endpoint, calculation endpoint, unit tests for calculation.
- UI: product list with add/remove quantity, member card input, Calculate button. Result shows total before discount, discounted items, member discount and final total.

## 3. Decisions and assumptions

| # | Decision | Why |
|---|----------|-----|
| D1 | Member 10% applies after pair discounts | "10% in total"; the PDF mockup shows it as a separate line after item discounts |
| D2 | Member cards are validated against a `members` table. An unknown or non-ACTIVE card rejects the request with 400 `INVALID_MEMBER_CARD` | The customer entered a card expecting a discount; silently ignoring it would mislead |
| D3 | Red limit is one global window per limited product (3600 s for Red), stored in Redis | Spec says "only one customer ... within 1 hour": read as "if Red was ordered in the last hour, reject further Red orders" |
| D4 | No customer identity is tracked. After the first accepted Red order, every Red order is rejected for the window, including from the same person (for example pressing Recalculate with Red in the cart) | Simplest faithful reading; needs no accounts or cookies |
| D5 | Orders store no customer ID | The rule does not depend on who the customer is |
| D6 | Money is integer satang (1/100 THB) | Avoids float errors; rounding is explicit |
| D7 | Products, discount rules and product limits live in Postgres, seeded by migration | Prices and conditions out of code; extensibility |
| D8 | Each successful calculation is saved as an order with the rules that fired | Audit trail |
| D9 | UI uses THB and the 7 real menu items, not the mockup's `$` and sample products | The mockup is a layout reference only |
| D10 | Rules are loaded per request (tables are tiny); caching is a documented extension | YAGNI |

Rounding: each discount amount is rounded half-up to the nearest satang when computed.

Known limits (documented in `docs/INTERVIEW.md`):
- Without identity, the person who ordered Red is also blocked if they recalculate with Red in the cart within the hour.
- The slot is claimed at calculation time (as the spec implies), so a price check by anyone blocks Red for an hour. Production would claim at checkout.
- Nothing releases the window early; it only expires.
- A new promo of an existing type is an `INSERT`; a new type of promo needs a new Go rule implementation.

## 4. Architecture

Backend follows Clean Architecture (Uncle Bob): four concentric layers, and
source-code dependencies point only inward.

```
Code-homework/
  backend/
    cmd/server/                  composition root: wires config, adapters, use cases, Fiber
    internal/
      domain/                    LAYER 1: enterprise rules, imports only the standard library
        product.go member.go     entities (Product, Member)
        money.go                 Money value object (int satang, rounding)
        order.go                 Order, OrderLine
        pricing.go               Calculator, Rule interface, BundlePercent, MemberPercent
        errors.go                domain errors (ErrInvalidQuantity, ErrUnknownProduct, ...)
      usecase/                   LAYER 2: application rules; defines ports, depends on domain only
        ports.go                 ProductRepository, MemberRepository, RuleRepository,
                                 LimitRepository, OrderRepository, SlotLimiter, Clock, IDGenerator
        calculate_order.go       CalculateOrder interactor (+ input/output models)
        list_products.go         ListProducts interactor
      adapter/                   LAYER 3: interface adapters (translate to/from the outside)
        http/                    Fiber handlers, request/response DTOs,
                                 error-to-status mapper, router
        postgres/                repositories implementing the ports; row -> domain.Rule mapping
        redis/                   SlotLimiter implementation (Lua script)
      platform/                  LAYER 4: frameworks and drivers
        config/                  env loading
        db/                      pgx pool, golang-migrate runner
    migrations/                  SQL schema + seed
  frontend/                      Next.js App Router, TypeScript (section 9)
  docker-compose.yml             postgres, redis, api, web
  docs/                          this spec, plan, INTERVIEW.md
  README.md
```

Dependency rule: `adapter -> usecase -> domain`; `platform` and `cmd` may import
anything but nothing imports them. `domain` imports no project package and no
third-party package. `usecase` never imports Fiber, pgx or go-redis.

Ports are interfaces declared in `usecase` (consumer side, "accept interfaces,
return structs"); adapters implement them. Every port has an in-memory fake for
unit tests, so the use cases and domain are tested with no database, Redis or
HTTP. Handlers are thin: parse -> call a use case -> present. Business logic is
never in a handler or a repository.

Why this layout: it maps one-to-one onto the scoring criteria. Readability: each
folder has one job. Maintainability: swapping Postgres or Redis touches only an
adapter. Extensibility: new rule types or endpoints add files and do not edit
existing ones. Logic: pricing is a pure function you can verify with a table.

## 5. Pricing (`internal/domain/pricing.go`)

- Input: list of `{product, quantity}` lines, `isMember bool`, and `[]Rule` supplied by the caller.
- `Rule` interface: `Apply(lines, subtotal) []Discount`. Rules run in ascending priority order; item rules run before the member rule, which applies to the subtotal after item discounts.
  - `BundlePercent{products, bundleSize, percentBP}`: bundles = quantity / bundleSize per eligible product; discount = bundles x bundleSize x price x percent.
  - `MemberPercent{percentBP}`: applies only when `isMember`.
- Output: `TotalBefore`, `ItemDiscounts[] {RuleCode, ProductID, Name, Bundles, Amount}`, `MemberDiscount`, `FinalTotal`.
- Example (seed rules): Orange x2 = 24000 satang, pair discount 1200, subtotal 22800, member 2280, final 20520.

## 6. Rules and limits (`adapter/postgres`, `adapter/redis`)

Rule mapping: the Postgres adapter's `RuleRepository` loads active `rules` rows
with their `rule_products` and maps each `type` to a domain constructor
(`domain.NewBundlePercent`, `domain.NewMemberPercent`), which validates its
parameters. An unknown type fails at load time with a clear error. The domain
knows nothing about tables.

```go
// declared in usecase/ports.go, implemented in adapter/redis
type SlotLimiter interface {
    // Claim records that productID was just ordered and blocks further orders of it
    // for ttl. If it was already ordered within the window, it returns domain.ErrSlotHeld.
    Claim(ctx context.Context, productID domain.ProductID, ttl time.Duration) error
}
```

- Redis key `limit:{productID}:ordered`, TTL = `product_limits.window_seconds`.
- Claim is one `SET key value NX PX ttl` command (atomic): the first caller creates the key and wins; every other caller in the window is rejected and the window is not extended.
- The service claims a slot for each ordered product that has an active row in `product_limits`.
  Orders with several limited products claim in product ID order; if a later claim fails, earlier claims of this request are kept (documented limitation; none exist in seed data beyond Red).
- Tests: miniredis for the Redis implementation; a trivial fake for service tests.

## 7. API

All under `/api`. Errors share one shape: `{ "error": { "code", "message" } }`.

- `GET /api/products` returns `[{id, name, priceSatang}]`.
- `POST /api/calculate` takes `{memberCard?, items:[{productId, quantity}]}`.
  - 200 returns `{totalBeforeSatang, itemDiscounts[{ruleCode, productId, name, bundles, amountSatang}], memberDiscountSatang, finalTotalSatang}`.
  - 400 `INVALID_REQUEST`: unknown product, quantity not a positive integer, empty order.
  - 400 `INVALID_MEMBER_CARD`: card not found or not ACTIVE. Card is normalized (trimmed, uppercased) before lookup.
  - 409 `LIMITED_PRODUCT_UNAVAILABLE` (message names the product): the product was already ordered within its window.
- `GET /healthz` returns 200 (used by Docker healthchecks).

Order of operations in the `CalculateOrder` use case: validate items -> look up member (if card given) -> load active rules and limits -> price (pure) -> claim limited slots (409 on conflict) -> persist order -> respond.
Slots are claimed only after all validation passes, so an invalid request cannot take one.

## 8. Database (Postgres)

```sql
products(id text pk, name text not null, price_satang int not null check (price_satang > 0))

members(card_number text pk,            -- normalized: trimmed, uppercase
        full_name text not null,
        status text not null check (status in ('ACTIVE','SUSPENDED')),
        created_at timestamptz not null default now())

rules(id serial pk, code text unique not null,
      type text not null check (type in ('BUNDLE_PERCENT','MEMBER_PERCENT')),
      percent_bp int not null check (percent_bp between 1 and 10000),   -- 500 = 5%
      bundle_size int null,                                              -- required for BUNDLE_PERCENT
      priority int not null,                                             -- lower runs first
      active boolean not null default true)

rule_products(rule_id int fk, product_id text fk, primary key (rule_id, product_id))

product_limits(product_id text pk fk, window_seconds int not null check (window_seconds > 0),
               active boolean not null default true)

orders(id uuid pk,
       member_card text null references members(card_number),
       total_before int, member_discount int, final_total int,
       created_at timestamptz not null default now())

order_items(order_id fk, product_id fk, quantity int not null check (quantity > 0))

order_discounts(order_id fk, rule_code text not null, product_id text null, amount_satang int not null)
```

Seed data (mirrors the PDF exactly):

| products.id | name | price_satang |
|---|---|---|
| red | Red set | 5000 |
| green | Green set | 4000 |
| blue | Blue set | 3000 |
| yellow | Yellow set | 5000 |
| pink | Pink set | 8000 |
| purple | Purple set | 9000 |
| orange | Orange set | 12000 |

| Table | Row | PDF condition |
|---|---|---|
| rules | `PAIR_5`: BUNDLE_PERCENT, percent_bp 500, bundle_size 2, priority 10 | "a pair of Orange, Pink or Green sets gets 5% discount for that pair" |
| rule_products | (`PAIR_5`, orange), (`PAIR_5`, pink), (`PAIR_5`, green) | the three eligible colours |
| rules | `MEMBER_10`: MEMBER_PERCENT, percent_bp 1000, priority 100 | "10% in total if customers have input member card numbers" |
| product_limits | red, window_seconds 3600 | "Red sets ... only one customer within 1 hour" |

Pair interpretation: a pair is two sets of the same colour (as in the PDF examples), not a mix such as Orange + Pink.
Each PDF example is a pricing test case:
- Orange x2 = (120 + 120) - 5% = 228.00
- Pink x4 = (80 + 80) - 5% + (80 + 80) - 5% = 304.00
- Green x3 = (40 + 40) - 5% + 40 = 116.00

Demo members: `MEM0001` (ACTIVE) and `MEM0002` (SUSPENDED).

An order, its items and its discounts are inserted in one transaction.
Migrations run on API start, in Docker Compose.

## 9. Frontend (Next.js, TypeScript)

Stack (versions checked 2026-10-16): Next.js 16 (App Router, Turbopack), React 19,
TypeScript strict, Tailwind CSS v4, Zod, Vitest + React Testing Library.

UI: one screen matching the PDF mockups: product rows with -/+ quantity, card
number input, Calculate button. A result view with Total (before discount),
Discounted items, Discount from member card, Final total, and Recalculate.

Best practices applied:
- **Server Components by default.** `app/page.tsx` is a Server Component that fetches
  `GET /products` on the server (using `API_URL`, the internal Docker address) and
  passes it to one Client Component island, `Calculator`. Only the interactive part
  ships JavaScript. Products are cached with `fetch(..., { next: { revalidate: 60 } })`.
- **Browser calls go through a rewrite.** `next.config.ts` rewrites `/api/*` to the Go
  API, so the browser uses one origin and needs no CORS setup. `calculate` is called from
  the client; a Server Action would add a hop and no benefit here.
- **Validate at the boundary.** Zod schemas parse every API response, so types are
  derived from runtime-checked data (`z.infer`) and not asserted with `as`.
- **Feature-based structure, logic out of components:**
  ```
  frontend/src/
    app/                  layout.tsx, page.tsx (RSC), globals.css, error.tsx, loading.tsx
    features/calculator/
      components/         ProductRow, QuantityStepper, MemberCardInput, ResultSummary
      hooks/useCalculator.ts   state machine: idle | loading | result | error
      api.ts              typed calls + Zod schemas
      types.ts
    lib/                  money.ts (satang -> "฿1,234.00" via Intl.NumberFormat), http.ts, env.ts
  ```
- **State:** `useReducer` inside `useCalculator`; no global store (YAGNI). Quantities are
  never negative; the stepper disables "-" at 0. Calculate is disabled with an empty cart.
- **Errors as UI states:** 409 shows the API message "<Product> was ordered recently
  and is not available right now. Please try again later." (the window is data, so the text does not
  name a duration); `INVALID_MEMBER_CARD` shows "Member card not found." Errors render
  in a `role="alert"` region.
- **Accessibility:** real `<button>` and `<label>` elements, `aria-label` on steppers
  ("Increase Pink set"), visible focus states, color contrast checked.
- **Config and build:** typed env access in `lib/env.ts`; `output: "standalone"` for a small
  Docker image; ESLint flat config (`eslint-config-next`) and `tsc --noEmit` in CI scripts;
  `noUncheckedIndexedAccess` on.
- **Mockups vs real data:** the mockups use `$` and sample products; the UI uses THB and
  the API's real products (D9).

## 10. Testing (test-first, vertical slices)

- `domain`: table-driven tests for the PDF examples, the member discount, rounding, mixed orders, the empty order, and rule constructor validation. No mocks needed.
- `usecase`: `CalculateOrder` and `ListProducts` tested with in-memory fakes of every port.
- `adapter/postgres`: rule row mapping, including unknown type error (integration, below).
- `adapter/redis`: miniredis tests for first claim, second claim rejected, expiry, and per-product keys.
- `adapter/http`: Fiber `app.Test` with a fake use case for status codes, error shapes and "no cookie issued". Use-case scenarios (with fakes) include: a second Red order within the hour is rejected for anyone, even with a valid member card; orders without Red still work while Red is blocked; an unknown member card returns 400 and does not claim the window.
- Postgres integration: repositories, migrations and seed data tested against a real Postgres (skipped unless `DATABASE_URL` is set; run in Docker Compose).
- Frontend: Vitest and React Testing Library for the hook, quantity controls and result rendering.

### 10.1 TDD workflow (applies to every task in the plan)

Red -> Green -> Refactor, one behavior at a time:
1. **Red:** write one failing test for the next behavior; run it and watch it fail for the right reason.
2. **Green:** write the minimum code to pass; run the whole package.
3. **Refactor:** clean up with tests green; no behavior change.
4. **Commit** after each green cycle (`test:` / `feat:` / `refactor:` messages), so history shows the test-first rhythm.

Rules:
- No production code without a failing test first. Bug fixes start with a test that reproduces the bug.
- Build in vertical slices, inside-out: domain (`Money`, pricing rules) -> use cases with fakes -> adapters -> HTTP -> frontend. Each slice is test-first and ends working.
- Test behavior through public APIs, not internals. Mock only at port boundaries (fakes for ports; never mock the domain).
- Tests are table-driven in Go (`t.Run` subtests, `t.Parallel` where safe), named by behavior (`TestBundlePercent_ThreeGreen_DiscountsOnePair`). Frontend tests query by role and label, as a user would.
- Determinism: inject `Clock` and `IDGenerator`; no `time.Now()` or random IDs inside use cases.
- Concurrency: the Redis claim has a test with many goroutines claiming at once (exactly one claim wins); run `go test -race`.
- Targets: domain and use cases ~100% line coverage; overall backend >= 85%; every PDF example and every error code has a test.
- Commands (all documented in README): `go test -race -cover ./...`, `go vet ./...`, `npm test`, `npm run lint`, `npx tsc --noEmit`.

## 11. Delivery

- `README.md`: prerequisites, `docker compose up`, ports, demo member cards, how to run tests (backend and frontend).
- `docs/INTERVIEW.md`: walkthrough of each design decision, trade-offs, known limits, and likely interview questions with answers.

## 12. Out of scope

Authentication, member registration or management endpoints, rule/limit admin endpoints, payment or checkout, an orders listing endpoint, i18n, multiple currencies.
