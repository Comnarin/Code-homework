# Food Store Calculator

A price calculator for a food store with 7 menu items. A **Go (Fiber) API** prices orders with
pair and member discounts and enforces the "one Red order per hour" limit. A
**Next.js UI** lets a customer pick quantities, enter a member card and see the breakdown.
Postgres (via GORM) holds products, members, discount rules and orders; Redis holds the Red-set lock.

## Prerequisites

- Docker with Compose (everything runs in containers).
- Only for local development outside Docker: Go 1.26 (Go downloads it automatically) and Node 22+.

## Run it

```bash
cp .env.example .env     # first time only; set POSTGRES_PASSWORD (and ports if you need)
docker compose up --build
```

| What | URL |
|---|---|
| Web UI | http://localhost:3000 (`WEB_HOST_PORT`) |
| API (direct) | http://localhost:18080 (`API_HOST_PORT`; the container itself uses 8080, and 18080 avoids clashing with other local services) |
| Postgres | `localhost:55432` (`POSTGRES_HOST_PORT`), user, password and database from your `.env` |

Stop with `Ctrl+C`. Postgres and Redis keep their data in Docker volumes, so orders and a held Red slot survive a plain stop and restart. For a clean start run `docker compose down -v`.

### Demo member cards

| Card | Status |
|---|---|
| `MEM0001` | active: gets 10% off |
| `MEM0002` | suspended: rejected with "Member card not found." |

Cards are case-insensitive and surrounding spaces are ignored.

### Try the Red set rule

1. Open http://localhost:3000, add a Red set and press Calculate: it works, and Red is now recorded as ordered for 1 hour.
2. Calculate again with Red in the cart (from any browser or window): it is rejected, because Red was already ordered within the hour. Orders without Red still work.

To reset the window: `docker compose exec redis redis-cli FLUSHALL`.

## Configuration (`.env`)

All settings come from environment variables. Each place that reads them has a committed
`.env.example` to copy; the real `.env` files are git-ignored, so secrets are never committed.

| File | Used by | Key settings |
|---|---|---|
| `.env` (from `.env.example`) | `docker compose` | `POSTGRES_PASSWORD` (required), `POSTGRES_USER`, `POSTGRES_DB`, `POSTGRES_HOST_PORT`, `API_HOST_PORT`, `WEB_HOST_PORT`, `ENABLE_TEST_TOOLS` |
| `backend/.env` (from `backend/.env.example`) | the API when run outside Docker (`go run ./cmd/server`) | `DATABASE_URL` (required), `REDIS_ADDR`, `PORT`, `ENABLE_TEST_TOOLS` |
| `frontend/.env` (from `frontend/.env.example`) | `npm run dev` / `npm run build` | `API_URL`, `ENABLE_TEST_TOOLS` |

Real environment variables always win over a `.env` file, and a missing `.env` is fine when the
variables are set another way (CI, production). Docker images never contain `.env` files.
`ENABLE_TEST_TOOLS=true` exposes a test-only "reset the Red set limit" endpoint and button; keep it
`false` anywhere real customers can reach.

## Pricing rules (from the task)

| Rule | Behavior |
|---|---|
| Pair discount | Each pair of the **same** colour among Orange, Pink or Green gets 5% off that pair. Pink x4 = two discounted pairs; Green x3 = one discounted pair + one full-price set. |
| Member discount | A valid member card gives 10% off the total **after** pair discounts. |
| Red set | If a Red set was ordered in the last hour, further Red orders are rejected for everyone until the hour is over. |

Money is handled as integer satang (1/100 THB) to avoid float errors; discounts round half-up.

## API

All errors look like `{"error":{"code":"...","message":"..."}}`.

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | liveness |
| GET | `/api/products` | the menu: `[{ "id", "name", "priceSatang" }]` |
| GET | `/api/promotions` | the active rules (`[{ code, type, percentBp, bundleSize?, productIds? }]`); the UI reads badges and percents from here, so rule changes need no frontend change |
| POST | `/api/calculate` | price an order |
| POST | `/api/test/reset-limits` | test aid, only when `ENABLE_TEST_TOOLS=true`: frees the Red set so the one-per-hour rule can be retried (the UI shows a "Reset Red set limit" button when the web container has the same flag) |

Request:

```json
{ "memberCard": "MEM0001", "items": [{ "productId": "orange", "quantity": 2 }] }
```

Response (amounts in satang):

```json
{
  "items": [{ "productId": "orange", "name": "Orange set", "quantity": 2, "unitPriceSatang": 12000, "lineTotalSatang": 24000 }],
  "totalBeforeSatang": 24000,
  "itemDiscounts": [{ "ruleCode": "PAIR_5", "productId": "orange", "name": "Orange set", "bundles": 1, "amountSatang": 1200 }],
  "memberDiscountSatang": 2280,
  "finalTotalSatang": 20520
}
```

| Status | Code | When |
|---|---|---|
| 400 | `INVALID_REQUEST` | empty order, unknown product, quantity not 1-1000, malformed JSON |
| 400 | `INVALID_MEMBER_CARD` | card not found or suspended |
| 409 | `LIMITED_PRODUCT_UNAVAILABLE` | Red was already ordered within the last hour |
| 500 | `INTERNAL` | unexpected failure (details are logged, never returned) |


## Run the tests

Backend (unit tests need nothing else):

```bash
cd backend
go test -race -cover ./...
```

To include the Postgres integration tests, start Postgres and point `DATABASE_URL` at it:

```bash
docker compose up -d postgres
DATABASE_URL='postgres://postgres:<POSTGRES_PASSWORD from .env>@localhost:55432/foodstore?sslmode=disable' go test -race -cover ./...
```

Frontend:

```bash
cd frontend
npm install
npm test            # component and logic tests (Vitest + Testing Library)
npm run typecheck
npm run lint
```

## Project structure

```
backend/                 Go, Clean Architecture
  cmd/server/            composition root: wires everything together
  internal/domain/       entities, Money, pricing rules (stdlib only)
  internal/usecase/      CalculateOrder, ListProducts + the interfaces they need
  internal/adapter/      http (Fiber) · postgres (GORM) · redis (SET NX claim)
  internal/platform/     config, database connection and migrations
  migrations/            SQL schema and seed data
frontend/                Next.js 16 App Router, TypeScript
  src/app/               page (streamed menu), error boundary
  src/features/calculator/  API client, reducer, hook, components
docs/                    design spec, implementation plan, interview notes
```

Dependency rule: `adapter -> usecase -> domain`. The domain never imports a framework, a database
driver or the use cases.

More: [design spec](docs/superpowers/specs/2026-10-16-food-store-calculator-design.md) ·
[implementation plan](docs/superpowers/plans/2026-10-16-food-store-calculator.md) ·
[interview walkthrough](docs/INTERVIEW.md)
