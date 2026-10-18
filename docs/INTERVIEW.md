# Interview walkthrough

How this project works, why it is built this way, and how to talk about it. Every file path below
exists in the repo; every code excerpt is copied from it.

## 1. The 60-second pitch

> I built a food-store price calculator as a Go Fiber API and a Next.js UI. The backend follows
> Clean Architecture: the pricing rules are a pure domain function with no I/O, the use cases talk
> to Postgres (through GORM) and Redis only through interfaces, and Fiber is just a thin delivery
> layer. Discounts and the "one Red order per hour" limit live in database tables, so
> a new promotion is a row, not a deploy. The Red-set rule is one atomic Redis Lua script (create the
> key with a one-hour expiry if it is free), so two simultaneous orders can never both win. I wrote it
> test-first. Measured coverage: 97% on the HTTP adapter, 98% on the domain, 95% on the use cases, 87% on
> the Redis adapter and about 94% overall for the backend with the Postgres and MinIO integration tests
> running (about 74% without them), 120 top-level Go tests (193 with subtests) and 68 frontend tests, CI on
> every push, and the whole thing starts with `docker compose up`.

## 2. What the project does

- 7 products. **Pair discount:** each pair of the *same* colour among Orange, Pink, Green gets 5% off that pair.
  **Member discount:** 10% off the total (after pair discounts) for a valid member card.
  **Red set:** if a Red set was ordered in the last hour, nobody can order Red again until the hour is over.
- Endpoints: `GET /api/products`, `GET /api/products/{id}/image`, `GET /api/promotions`, `POST /api/calculate`,
  `GET /healthz` (liveness), `GET /readyz` (pings Postgres and Redis) and, only when `ENABLE_TEST_TOOLS=true`,
  `POST /api/test/reset-limits`.
- UI: product list with +/-, card input, Calculate; result screen with total, item discounts, member discount, final total.

## 3. Follow one request end to end

`POST /api/calculate` with Orange x2, card `mem0001`:

```
Browser ──POST /api/calculate──▶ Next.js (:3000)
                                   │ rewrite /api/* (next.config.ts)
                                   ▼
                              Go API (:8080)
  adapter/http/handlers.go       calculate(): parse JSON → usecase.CalculateInput
  usecase/calculate_order.go     Execute():
        1. mergeItems       validate quantities, merge duplicate products
        2. resolveCarts     load products, reject unknown ones            ──▶ Postgres (products)
        3. checkMember      normalize card, must exist and be ACTIVE      ──▶ Postgres (members)
        4. ActiveRules      build domain rules from table rows            ──▶ Postgres (rules)
        5. domain.Calculate pure pricing → Quote
        6. claimLimited     record Red as ordered (Lua claim)             ──▶ Redis
        7. Orders.Save      order + items + discounts, one transaction    ──▶ Postgres
           (if 7 fails, the claims this request made are released)
  adapter/http/dto.go            Quote → JSON (amounts in satang)
Browser ◀── ResultSummary ◀──────────────────────────────────────────────
```

Key point to say out loud: **steps 1-5 only read; the slot is claimed (6) only after everything has
been validated**, so a bad request can never take the Red slot.

## 4. Clean Architecture on this codebase

```
domain  ◀── usecase ◀── adapter ◀── platform / cmd
(stdlib)    (ports)     (Fiber, GORM, Redis)   (config, db, wiring)
```

- **Dependency rule:** source code dependencies point inward only. `internal/domain` imports nothing
  but the standard library. `internal/usecase` never imports Fiber, GORM or go-redis.
- **Ports** (interfaces) are declared by the code that *uses* them, in `internal/usecase/ports.go`
  (`ProductRepository`, `MemberRepository`, `RuleRepository`, `LimitRepository`, `OrderRepository`,
  `SlotLimiter` with `Claim`/`Release`/`Clear`, `ImageStore`, `Clock`, `IDGenerator`). The adapters implement them. This is "accept interfaces,
  return structs" and it is what makes everything testable with in-memory fakes
  (`internal/usecase/fakes_test.go`).
- **Composition root:** `cmd/server/main.go` is the only place that knows every concrete type.
- **Persistence models are separate from domain types:** `adapter/postgres/models.go` holds the
  GORM-tagged structs; each repository maps to/from `domain` types. The domain has no `gorm:"..."` tags.
- **What changes if Postgres is replaced?** Only `adapter/postgres` (and `migrations/`). The use
  cases and domain do not change.

## 5. The core code, explained

**Money** (`domain/money.go`): integer satang, discounts as whole percents (5 = 5%), half-up rounding.

```go
func (m Money) ApplyPercent(percent int) Money {
	return Money((int64(m)*int64(percent) + 50) / 100)
}
```

**Rules are an interface** (`domain/pricing.go`), run in priority order on the running subtotal, so
"member 10% comes after the pair discount" is data (`priority 10` vs `100`), not an `if`:

```go
type Rule interface {
	Code() string
	Priority() int // lower runs first
	Apply(lines []Cart, subtotal Money, isMember bool) []Discount
}
```

`Calculate` also caps every discount at what is left of the total, so two generous rules can never push the
final price below zero. `BundlePercent` (the pair rule) and `MemberPercent` are in `domain/rules.go`. A pair is
`quantity / bundleSize` complete groups of the *same* product, so Pink x4 is 2 pairs and Green x3 is 1 pair.

**Rules and limits come from tables** (`migrations/000002_seed.up.sql`): `rules`, `rule_products`,
`product_limits`. `adapter/postgres/rules.go` turns a row into a domain rule with `buildRule`; an
unknown rule type fails loudly instead of being ignored.

**The Red rule** (`adapter/redis/slot_limiter.go`) is a small Lua script, which Redis runs atomically:

```lua
local current = redis.call('GET', KEYS[1])
if not current then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if current == ARGV[1] then return 0 end
return -1
```

The value stored is the *holder*: the browser's per-session `X-Client-Key` if it sent one, otherwise the
order ID. The first order of Red creates the key (1) and wins; the same holder claiming again gets 0 (allowed,
the window is not extended); anyone else gets -1 → `ErrSlotHeld` → HTTP 409. A rejected claim does **not**
extend the window. There are still no accounts: the key is a random value kept in `sessionStorage`, not an
identity.

If saving the order fails, or a later limited product in the same order is refused, the use case calls
`Release`, a second script that deletes the key only if it still holds that holder (compare-and-delete). It only
releases claims that *this request created*, and never one that expired and was taken by someone else.

**Errors have one exit** (`adapter/http/errors.go`): handlers just `return err`; `mapError` turns
domain errors into `{status, code, message}`. Unexpected errors are logged and returned as a generic
500, never leaking internals.

**Frontend** (`frontend/src/`):
- `app/page.tsx` is a Server Component. The static shell renders immediately; the menu streams in
  inside `<Suspense>`. It calls `await connection()` so it is rendered per request (the API does
  not exist at build time), then a `"use cache"` function (`features/calculator/server.ts`) caches
  the menu for a few minutes.
- `features/calculator/components/Calculator.tsx` is the one Client Component island.
- State is a pure reducer (`state.ts`) tested without React; `useCalculator.ts` is a thin hook.
- `api.ts` validates every response with Zod, so types come from runtime-checked data, not `as` casts.
- `calculate` is called from the browser through the Next.js rewrite, so there is one origin and no
  CORS setup; a Server Action would only add a hop.

## 6. Decisions and trade-offs (say these in your own words)

| Decision | Why | Cost / alternative |
|---|---|---|
| Integer satang, whole percents | No float rounding bugs; rounding is explicit and tested; `5` means 5% and reads naturally in the database and API | A rate like 12.5% cannot be stored (use basis points or a decimal type if that is ever needed) |
| Rules as DB rows + Go constructors | New promo of an existing type is an `INSERT`; the rule logic stays unit-testable and pure | A new *kind* of promo still needs Go code; rules are loaded per request (cheap, cacheable later) |
| Atomic Lua claim, not `GET` then `SET` | Two concurrent requests cannot both see an empty slot; the same script also lets the holder claim again | Needs Redis; the key can be lost if Redis crashes between snapshots or is flushed, so the slot frees early (use AOF if that matters) |
| Holder = random per-session client key, no accounts | The customer who ordered Red can re-price it; everyone else is still blocked; nothing to sign up for | Anyone who copies the key (or clears the session and gets a new one) changes who the holder is; a real customer ID would be sturdier |
| Claim after validation, release on failure | An invalid request cannot take the slot; a failed save gives it back (compare-and-delete) | If the release call itself fails the slot just expires at the end of its window (logged) |
| GORM for queries, SQL files for schema (golang-migrate runs them through pgx) | Readable, reviewable migrations and constraints; GORM removes row-scanning boilerplate | Two mechanisms (golang-migrate + GORM); AutoMigrate was the alternative |
| Configuration from environment variables, `.env` only as a local convenience (`config.LoadDotEnv`) | Twelve-factor: the same image runs anywhere; real env vars win over the file; secrets stay out of git and out of images | A missing or wrong variable fails at start-up (`DATABASE_URL is required`) instead of silently using a default |
| Product pictures in MinIO, served through the API | MinIO stays private; the API can swap in a default picture on any failure, so the UI never shows a broken image | One extra hop per picture (mitigated by a short browser cache); the API holds image bytes in memory per request (capped at 5 MB) |
| Only PNG/JPEG/WebP/GIF, `nosniff` + sandbox CSP | Pictures are untrusted bytes served from our own origin; an SVG or HTML upload could run script | An SVG product picture is not allowed (the built-in default is SVG, but it is our own trusted file) |
| Circuit breaker around MinIO (3 s timeout, 10 s cooldown) | A stopped MinIO would otherwise delay every picture by the full timeout | After an outage, pictures stay on the default for up to 10 s even if MinIO is back |
| GORM models separate from domain | Domain stays framework-free | A small mapping layer |
| Server Component + client island | Only the interactive part ships JavaScript | More concepts than a single client page |
| Zod at the API boundary | A changed backend response fails loudly in one place | Extra dependency |

## 7. How it was built (the TDD story)

Order of commits (`git log --oneline --reverse`): domain `Money` → entities → `Calculate` → rules →
use-case ports → `CalculateOrder` → rejection tests → Redis → migrations → repositories → HTTP →
config/wiring → frontend → Docker. Inside-out, each step red → green → commit.

Things worth telling an interviewer:

- **Red first, every time.** Each test was run and seen failing for the right reason (for example
  `undefined: Money`) before any production code existed.
- **Mutation check.** After writing the rejection tests I deliberately broke the code to prove the
  tests could fail: moving the Red claim before the member check turned
  `TestCalculateOrder_RejectsUnknownOrSuspendedCardWithoutClaimingRed` red. One mutation (loosening
  the per-item quantity limit) **survived**, which exposed a weak test: the input I used was also
  caught by a second check. I replaced it with a wrap-around case (four quantities of 2^62 plus 5
  sums to 5 after integer overflow) that only the per-item bound stops. The fix is in commit
  `ec76969`.
- **Concurrency test.** `TestSlotLimiter_ExactlyOneOfManyConcurrentClaimsWins` runs 50 goroutines
  racing for the slot and asserts exactly one wins; run under `go test -race`.
- **Every PDF example is a test** (`domain/examples_test.go`): Orange x2 = 228.00, Pink x4 = 304.00,
  Green x3 = 116.00, and member 10% after pair discount = 205.20.
- **Real dependencies where it matters.** Repositories are tested against a real Postgres (not
  mocked), including "a failed save leaves no partial rows".
- **Real-world snags** that were caught by running the stack: a healthcheck that used `localhost`
  (IPv6 inside Alpine) while Fiber listens on IPv4; the API's default host port 8080 clashing with
  another service on the dev machine (so Compose publishes 18080); and Next.js 16's Cache Components
  mode, which rules out `export const dynamic = "force-dynamic"`.

## 8. Known limitations and what I would do in production

1. **No real identity:** the holder is a random browser-session key, which is easy to lose or share → with accounts, tie the claim to the customer.
2. **Claim at checkout, not at quote:** today, pricing a Red order reserves the slot (as the task implies), so anyone pricing Red can block it for an hour without buying → a separate checkout step that claims, and an expiry.
3. **Release is best effort:** a failed save or a refused second product releases this request's claims, but if Redis is unreachable at that moment the claim simply expires.
4. **Rules/limits loaded on every request** → in-memory cache with a short TTL or invalidation on change.
5. **No rate limiting** on `POST /api/calculate` (the client key is for the Red holder, not an idempotency key) → add rate limiting and real idempotency.
6. **Observability:** structured logs exist for 500s; add request IDs, metrics (latency, 409 rate), tracing.
7. **Members:** no registration or admin; cards are looked up only.
8. **Not verified in a real browser here** (no headless browser was installed); the UI is covered by component tests and a live request check through the proxy.

## 9. Likely questions, with model answers

**Why Clean Architecture for such a small app? When is it overkill?**
The task is scored on readability, maintainability, extensibility and logic, and Clean Architecture
gives a place for each: pricing in a pure domain package, I/O behind interfaces, a thin HTTP layer.
It is overkill for a one-off script or a CRUD admin with no rules. Here the rules are the point, and
I wanted to unit-test them with no database, so the boundary earns its cost.

**How do you guarantee only one Red order per hour under concurrent requests?**
The claim is one Lua script, which Redis executes atomically: it creates the key (with an expiry) only
if it does not exist. Of any number of simultaneous requests exactly one creates it. There is a
test with 50 goroutines racing that asserts exactly one wins, run with the race detector.

**Why no customer ID or cookie?**
The rule is "if Red was ordered in the last hour, nobody can order it". I do not track accounts or
set cookies. To stop the person who just ordered Red from being blocked when they press Recalculate,
the browser sends a random key from `sessionStorage` in an `X-Client-Key` header and the claim stores it
as the holder, so the holder can re-price and everyone else gets 409. The header is optional; without it
every request is a different holder, which is the plain "nobody can order it" reading. With real
accounts I would use the customer ID instead of a random key.

**What if Redis loses the key?**
With the default image, Redis writes periodic snapshots, so a clean restart usually keeps the slot,
but a crash between snapshots or a flush drops it. Then the slot frees early and a second customer
could order Red within the hour. For this app that is acceptable; if it matters I would enable AOF
persistence, or keep the claim in Postgres with a
unique constraint and an expiry timestamp.

**Why integers for money? How do you round?**
Floats cannot represent most decimal amounts exactly. I store satang as `int64` and compute
discounts as whole percents, rounding half-up per discount line (`ApplyPercent`). The rule is tested at the
boundary: 0.5 satang rounds up, 0.45 rounds down.

**How would you add "buy 3, get 1 free"? A new limited product?**
New limited product: insert a row in `product_limits`, no code change, because the Redis key is per
product. A promotion of the existing "percent off complete groups" type is an `INSERT` into `rules`
and `rule_products`. A genuinely new kind of promotion needs a new `Rule` implementation in
`domain/rules.go` with its own tests and one new case in `buildRule`; no existing rule changes.

**How would you scale this to several API instances?**
The API is stateless: the Red window is in Redis and data is in Postgres. So I can add instances behind a load balancer. The shared pieces to size are the Postgres
connection pool and Redis; migrations run at start-up and are idempotent, though in production I
would run them as a separate job.

**How do you test code that depends on time, randomness, Redis and Postgres?**
Time and IDs are injected (`Clock`, `IDGenerator`), so use cases are deterministic. Ports have
in-memory fakes for fast tests. Adapters are tested against the real thing: Redis behavior with
miniredis (including TTL expiry via `FastForward`), and Postgres with a real database in Docker.

**What are the security or abuse risks?**
Anyone can price an order with Red and thereby block it for everyone for an hour, since pricing is
what records the order. In production I would separate "get a quote" from "place an order", record
Red only when an order is placed, and add authentication and rate limiting.

**What happens when MinIO is down or an image is missing?**
The user always gets a picture. `GetProductImage` (a use case) asks the `ImageStore` port; any failure
(not configured, unreachable, missing, wrong type, too large) returns the built-in "no image" picture
instead of an error, and logs a warning. The response for a stand-in is `Cache-Control: no-store`, so the
real picture appears as soon as MinIO recovers. A small circuit breaker in the MinIO adapter makes the
fallback instant during an outage instead of waiting for the timeout on each of the seven pictures.
There is a second fallback in the browser (`onError` swaps to `/no-image.svg`) for the case where the
API itself cannot be reached.

**Why serve pictures through the API instead of giving the browser a MinIO link?**
It keeps MinIO private (no public bucket, no CORS, no signed-URL expiry), and it lets the server decide
what to show when storage fails. The price is one more hop; a short `max-age` keeps that cheap. For
large traffic I would put a CDN in front of the endpoint or switch to signed URLs.

**Why Server Components here, and what runs on the client?**
The menu is read-mostly data, so it is fetched on the server and streamed, and no JavaScript is
shipped for it. Only the calculator (quantities, card input, result) is a Client Component because it
holds state.

**What would you monitor? What breaks first under load?**
Latency and error rate per endpoint, the 409 rate, and Redis and Postgres connection saturation.
The first thing to hurt is the per-request loading of products, rules, and limits from Postgres;
I would cache those briefly.

**What would you do with another week?**
Authentication and a separate checkout step that claims the Red slot, a cache for rules and limits,
request IDs and metrics, and an end-to-end browser test of the Red scenario (CI already runs the unit,
integration and frontend tests).

**How do you review a junior's change in this codebase?**
First the dependency rule: does anything inward import something outward? Then: is there a test that
failed first, does it assert behavior rather than implementation, and are the error paths covered?
Then naming and whether the change belongs in the domain, a use case or an adapter.

## 10. Cheat sheet

1. Domain is pure; use cases own the interfaces; adapters implement them; `main.go` wires.
2. Money = int satang; discounts = whole percents (5 = 5%); round half-up.
3. Rules run by priority: pair (10) then member (100), member on the running subtotal.
4. Pair = same colour, `qty / 2` groups.
5. Red rule = one Lua script (atomic); first Red order in the hour wins, the window is never extended; recorded only after validation and released (compare-and-delete) if saving fails.
6. No accounts: the holder is a random per-session `X-Client-Key`; the browser reaches the API through a Next.js rewrite.
7. Errors: handlers return errors; one `mapError`; 500s never leak details.
8. Frontend: RSC shell + `<Suspense>` menu + one client island; reducer for state; Zod at the boundary.
9. TDD: red first, mutation check, concurrency test, PDF examples as tests.
10. Known gaps: quote vs order (pricing records Red), no accounts, rule caching.

Commands:

```bash
docker compose up --build                       # run everything (UI :3000, API :18080)
cd backend && go test -race -cover $(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)   # add DATABASE_URL=... for Postgres tests
cd frontend && npm test && npm run typecheck && npm run lint
```
