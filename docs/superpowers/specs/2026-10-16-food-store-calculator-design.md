# Food Store Calculator: Design Spec

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
| D3 | Red limit is one global slot per limited product, held for its window (3600 s for Red), stored in Redis | Spec says "only one customer ... within 1 hour" |
| D4 | The slot holder may recalculate or reorder during the window; the timer does not restart | Otherwise the Recalculate button would block the customer who holds the slot |
| D5 | Customer identity is a server-issued anonymous cookie `cid`. The member card does not affect identity | Not forgeable by guessing; a typed card number could be used to take over someone's slot |
| D6 | Money is integer satang (1/100 THB) | Avoids float errors; rounding is explicit |
| D7 | Products, discount rules and product limits live in Postgres, seeded by migration | Prices and conditions out of code; extensibility |
| D8 | Each successful calculation is saved as an order with the rules that fired | Audit trail |
| D9 | UI uses THB and the 7 real menu items, not the mockup's `$` and sample products | The mockup is a layout reference only |
| D10 | Rules are loaded per request (tables are tiny); caching is a documented extension | YAGNI |

Rounding: each discount amount is rounded half-up to the nearest satang when computed.

Known limits (documented in `docs/INTERVIEW.md`):
- Clearing cookies gives a new anonymous identity, so the limit can be bypassed without accounts.
- The slot is claimed at calculation time (as the spec implies), so a customer can hold it without buying. Production would claim at checkout.
- Removing Red and recalculating does not release the slot until it expires.
- A new promo of an existing type is an `INSERT`; a new type of promo needs a new Go rule implementation.

## 4. Architecture

```
Code-homework/
  backend/
    cmd/server/          main.go, wiring only
    internal/menu/       Product type, ProductRepository interface
    internal/member/     Member type, MemberRepository interface
    internal/pricing/    pure calculator, Rule interface and rule types (no I/O)
    internal/rules/      RuleRepository interface, registry mapping rule rows to pricing.Rule
    internal/redlimit/   Limiter interface, Redis impl, in-memory fake
    internal/order/      service: validate, check member, claim limits, price, persist
    internal/store/      Postgres repositories (products, members, rules, limits, orders)
    internal/httpapi/    Fiber app, handlers, DTOs, cid middleware, error mapping
    migrations/          golang-migrate SQL (schema + seed)
  frontend/              Next.js App Router, TypeScript
  docker-compose.yml     postgres, redis, api, web
  docs/                  this spec, plan, INTERVIEW.md
  README.md
```

Dependency direction: `httpapi -> order -> {pricing, menu, member, rules, redlimit, store}`.
`pricing` imports nothing from the project. Interfaces are defined where they
are consumed (consumer-side), so every external dependency has an in-memory
fake for unit tests.

## 5. Pricing (`internal/pricing`)

- Input: list of `{product, quantity}` lines, `isMember bool`, and `[]Rule` supplied by the caller.
- `Rule` interface: `Apply(lines, subtotal) []Discount`. Rules run in ascending priority order; item rules run before the member rule, which applies to the subtotal after item discounts.
  - `BundlePercent{products, bundleSize, percentBP}`: bundles = quantity / bundleSize per eligible product; discount = bundles x bundleSize x price x percent.
  - `MemberPercent{percentBP}`: applies only when `isMember`.
- Output: `TotalBefore`, `ItemDiscounts[] {RuleCode, ProductID, Name, Bundles, Amount}`, `MemberDiscount`, `FinalTotal`.
- Example (seed rules): Orange x2 = 24000 satang, pair discount 1200, subtotal 22800, member 2280, final 20520.

## 6. Rules and limits (`internal/rules`, `internal/redlimit`)

Rules registry: each `rules.type` maps to a constructor that builds a
`pricing.Rule` from the row and its `rule_products`. An unknown type fails at
load time with a clear error.

```go
type Limiter interface {
    // Claim reserves the slot for productID for ownerID for ttl. It succeeds if
    // the slot is free or already held by ownerID; otherwise it returns ErrHeld.
    Claim(ctx context.Context, productID, ownerID string, ttl time.Duration) error
}
```

- Redis key `limit:{productID}:holder`, value = `cid`, TTL = `product_limits.window_seconds`.
- Claim is one Lua script (atomic): if no key, `SET ... EX`; if value equals owner, succeed without extending the TTL; else return held.
- The service claims a slot for each ordered product that has an active row in `product_limits`.
  Orders with several limited products claim in product ID order; if a later claim fails, earlier claims of this request are kept (documented limitation; none exist in seed data beyond Red).
- Tests: miniredis for the Redis implementation; a trivial fake for service tests.

## 7. API

All under `/api`. Errors share one shape: `{ "error": { "code", "message" } }`.

- `GET /api/products` returns `[{id, name, priceSatang}]`.
- `POST /api/calculate` takes `{memberCard?, items:[{productId, quantity}]}`. The customer comes from the `cid` cookie.
  - 200 returns `{totalBeforeSatang, itemDiscounts[{ruleCode, productId, name, bundles, amountSatang}], memberDiscountSatang, finalTotalSatang}`.
  - 400 `INVALID_REQUEST`: unknown product, quantity not a positive integer, empty order.
  - 400 `INVALID_MEMBER_CARD`: card not found or not ACTIVE. Card is normalized (trimmed, uppercased) before lookup.
  - 409 `LIMITED_PRODUCT_UNAVAILABLE` (message names the product): another customer holds the slot.
- `GET /healthz` returns 200 (used by Docker healthchecks).
- `cid` middleware: if the cookie is missing, generate a random UUID and set `cid` as `HttpOnly; SameSite=Lax; Path=/`; put it in the request context.

Order of operations in `calculate`: validate items -> look up member (if card given) -> load active rules and limits -> price (pure) -> claim limited slots (409 on conflict) -> persist order -> respond.
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

orders(id uuid pk, customer_id text not null,
       member_card text null references members(card_number),
       total_before int, member_discount int, final_total int,
       created_at timestamptz not null default now())

order_items(order_id fk, product_id fk, quantity int not null check (quantity > 0))

order_discounts(order_id fk, rule_code text not null, product_id text null, amount_satang int not null)
```

Seed data:
- 7 products; members `MEM0001` (ACTIVE) and `MEM0002` (SUSPENDED) for demos.
- rules: `PAIR_5` (BUNDLE_PERCENT, 500, size 2, priority 10) with `rule_products` Orange, Pink, Green; `MEMBER_10` (MEMBER_PERCENT, 1000, priority 100).
- product_limits: Red, 3600 seconds.

An order, its items and its discounts are inserted in one transaction.
Migrations run on API start, in Docker Compose.

## 9. Frontend (Next.js, TypeScript)

- One screen, matching the PDF mockups: product rows with -/+ quantity, card number input, Calculate button. A result view with Total (before discount), Discounted items, Discount from member card, Final total, and Recalculate.
- `next.config` rewrites `/api/*` to the Go API, so the browser uses one origin and the `cid` cookie needs no CORS setup.
- Structure: typed API client (`lib/api.ts`), a `useCalculator` hook for state, presentational components, and a THB formatter for satang.
- Error messages: 409 shows "<Product> is reserved by another customer for up to 1 hour."; `INVALID_MEMBER_CARD` shows "Member card not found."

## 10. Testing (test-first, vertical slices)

- `pricing`: table-driven tests for the PDF examples, the member discount, rounding, mixed orders, the empty order, and inactive/unknown rules.
- `rules`: registry builds each rule type from rows; unknown type errors.
- `redlimit`: miniredis tests for same owner, different owner, expiry, and per-product keys.
- `order` and `httpapi`: fakes for Limiter and repositories; Fiber `app.Test` for status codes, error shapes and `cid` cookie behavior. Includes: same customer orders Red twice in one hour, second with a valid member card (allowed, member discount applied); a different `cid` is rejected with 409; an unknown member card returns 400 and does not claim a slot.
- `store`: tests against real Postgres (skipped unless `DATABASE_URL` is set; run in Docker Compose), including migrations and seed data.
- Frontend: Vitest and React Testing Library for the hook, quantity controls and result rendering.

## 11. Delivery

- `README.md`: prerequisites, `docker compose up`, ports, demo member cards, how to run tests (backend and frontend).
- `docs/INTERVIEW.md`: walkthrough of each design decision, trade-offs, known limits, and likely interview questions with answers.

## 12. Out of scope

Authentication, member registration or management endpoints, rule/limit admin endpoints, payment or checkout, an orders listing endpoint, i18n, multiple currencies.
