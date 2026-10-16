# Food Store Calculator: Design Spec

Date: 2026-10-16
Source: `Code Homework Test.pdf` (scored on readability, maintainability, extensibility, logic)

## 1. Goal

A food store price calculator. A Go Fiber API calculates order prices with
discounts and enforces a one-customer-per-hour limit on the Red set. A Next.js
UI lets a customer pick quantities, enter an optional member card and see the
price breakdown. Built test-first. Runs with `docker compose up`.

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
| D2 | Any well-formed card (trimmed, 1-32 alphanumeric) counts as a member; no members table | Spec has no card database; a members table is easy to add later |
| D3 | Red limit is one global slot, held for 1 hour, stored in Redis | Spec says "only one customer ... within 1 hour" |
| D4 | The slot holder may recalculate during their hour | Otherwise the Recalculate button would block the customer who holds the slot |
| D5 | Customer identity is a server-issued anonymous cookie `cid` | Not forgeable by guessing; the frontend never handles identity |
| D6 | Money is integer satang (1/100 THB) | Avoids float errors; rounding is explicit |
| D7 | Products live in Postgres, seeded by migration | Prices out of code; extensibility |
| D8 | Each successful calculation is saved as an order (Postgres) | Audit trail; user choice "B" |
| D9 | UI uses THB and the 7 real menu items, not the mockup's `$` and sample products | The mockup is a layout reference only |

Rounding: each discount amount is rounded half-up to the nearest satang when computed.

Known limits (documented in `docs/INTERVIEW.md`):
- Clearing cookies gives a new anonymous identity, so the limit can be bypassed without accounts.
- The slot is claimed at calculation time (as the spec implies), so a customer can hold it without buying. Production would claim at checkout.
- Removing Red and recalculating does not release the slot until it expires.

## 4. Architecture

```
Code-homework/
  backend/
    cmd/server/          main.go, wiring only
    internal/menu/       Product type, ProductRepository interface
    internal/pricing/    pure calculator and Rule interface (no I/O)
    internal/redlimit/   Limiter interface, Redis impl, in-memory fake
    internal/order/      service: validate, claim Red, price, persist
    internal/store/      Postgres repositories (products, orders)
    internal/httpapi/    Fiber app, handlers, DTOs, cid middleware, error mapping
    migrations/          golang-migrate SQL (schema + product seed)
  frontend/              Next.js App Router, TypeScript
  docker-compose.yml     postgres, redis, api, web
  docs/                  this spec, plan, INTERVIEW.md
  README.md
```

Dependency direction: `httpapi -> order -> {pricing, menu, redlimit, store}`.
`pricing` imports nothing from the project. Interfaces are defined where they
are consumed (consumer-side), so every external dependency has an in-memory
fake for unit tests.

## 5. Pricing (`internal/pricing`)

- Input: list of `{product, quantity}` lines and `isMember bool`.
- `Rule` interface: `Apply(lines) []Discount`. Rules run in order; the member rule runs last on the subtotal after item discounts.
  - `PairDiscount{eligible: {Orange, Pink, Green}, percent: 5}`: pairs = quantity / 2 per eligible product; discount = pairs x 2 x price x 5%.
  - `MemberDiscount{percent: 10}`: applies only when `isMember`.
- Output: `TotalBefore`, `ItemDiscounts[] {ProductID, Name, Pairs, Amount}`, `MemberDiscount`, `FinalTotal`.
- Example: Orange x2 = 24000 satang, pair discount 1200, subtotal 22800, member 2280, final 20520.

## 6. Red set limiter (`internal/redlimit`)

```go
type Limiter interface {
    // Claim reserves the Red slot for ownerID for ttl. It succeeds if the slot
    // is free or already held by ownerID; otherwise it returns ErrHeld.
    Claim(ctx context.Context, ownerID string, ttl time.Duration) error
}
```

- Redis key `redset:holder`, value = `cid`, TTL 1 hour.
- Claim is one Lua script (atomic): if no key, `SET ... EX`; if value equals owner, succeed without extending the TTL; else return held.
- Tests: miniredis for the Redis implementation; a trivial fake for service tests.

## 7. API

All under `/api`. Errors share one shape: `{ "error": { "code", "message" } }`.

- `GET /api/products` returns `[{id, name, priceSatang}]`.
- `POST /api/calculate` takes `{memberCard?, items:[{productId, quantity}]}`. The customer comes from the `cid` cookie.
  - 200 returns `{totalBeforeSatang, itemDiscounts[{productId, name, pairs, amountSatang}], memberDiscountSatang, finalTotalSatang}`.
  - 400 `INVALID_REQUEST`: unknown product, quantity not a positive integer, empty order, bad card format.
  - 409 `RED_SET_UNAVAILABLE`: another customer holds the Red slot.
- `GET /healthz` returns 200 (used by Docker healthchecks).
- `cid` middleware: if the cookie is missing, generate a random UUID and set `cid` as `HttpOnly; SameSite=Lax; Path=/`; put it in the request context.

Order of operations in `calculate`: validate input -> price (pure) -> if Red quantity > 0, claim the slot (409 on conflict) -> persist order -> respond.
The slot is claimed only after validation passes, so an invalid request cannot take it.

## 8. Database (Postgres)

- `products(id text pk, name text, price_satang int check > 0)`, seeded with the 7 items.
- `orders(id uuid pk, customer_id text, member_card text null, total_before int, member_discount int, final_total int, created_at timestamptz)`.
- `order_items(order_id fk, product_id fk, quantity int, discount_satang int)`.
- Order + items are inserted in one transaction.
- Migrations run on API start, in Docker Compose.

## 9. Frontend (Next.js, TypeScript)

- One screen, matching the PDF mockups: product rows with -/+ quantity, card number input, Calculate button. A result view with Total (before discount), Discounted items, Discount from member card, Final total, and Recalculate.
- `next.config` rewrites `/api/*` to the Go API, so the browser uses one origin and the `cid` cookie needs no CORS setup.
- Structure: typed API client (`lib/api.ts`), a `useCalculator` hook for state, presentational components, and a THB formatter for satang.
- The 409 error shows: "Red set is reserved by another customer for up to 1 hour."

## 10. Testing (test-first, vertical slices)

- `pricing`: table-driven tests for the PDF examples, the member discount, rounding, mixed orders and the empty order.
- `redlimit`: miniredis tests for same owner, different owner, and expiry.
- `order` and `httpapi`: fakes for Limiter and repositories; Fiber `app.Test` for status codes, error shapes and `cid` cookie behavior.
- `store`: tests against real Postgres (skipped unless `DATABASE_URL` is set; run in Docker Compose).
- Frontend: Vitest and React Testing Library for the hook, quantity controls and result rendering.

## 11. Delivery

- `README.md`: prerequisites, `docker compose up`, ports, how to run tests (backend and frontend).
- `docs/INTERVIEW.md`: walkthrough of each design decision, trade-offs, known limits, and likely interview questions with answers.

## 12. Out of scope

Authentication, a members table, payment or checkout, an orders listing endpoint, i18n, multiple currencies.
