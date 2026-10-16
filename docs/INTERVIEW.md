# Interview walkthrough

How this project works, why it is built this way, and how to talk about it. Every file path below
exists in the repo; every code excerpt is copied from it.

## 1. The 60-second pitch

> I built a food-store price calculator as a Go Fiber API and a Next.js UI. The backend follows
> Clean Architecture: the pricing rules are a pure domain function with no I/O, the use cases talk
> to Postgres (through GORM) and Redis only through interfaces, and Fiber is just a thin delivery
> layer. Discounts and the "one customer per hour" limit on the Red set live in database tables, so
> a new promotion is a row, not a deploy. The Red-set lock is an atomic Redis Lua script, so two
> simultaneous customers can never both win. I wrote it test-first: 100% coverage on the HTTP and
> Redis adapters, 97% on the domain, 95% on the use cases, plus 37 frontend tests, and the whole
> thing starts with `docker compose up`.

## 2. What the project does

- 7 products. **Pair discount:** each pair of the *same* colour among Orange, Pink, Green gets 5% off that pair.
  **Member discount:** 10% off the total (after pair discounts) for a valid member card.
  **Red set:** only one customer may order it per hour.
- Endpoints: `GET /api/products`, `POST /api/calculate`, `GET /healthz`.
- UI: product list with +/-, card input, Calculate; result screen with total, item discounts, member discount, final total.

## 3. Follow one request end to end

`POST /api/calculate` with Orange x2, card `mem0001`:

```
Browser ──POST /api/calculate──▶ Next.js (:3000)
                                   │ rewrite /api/* (next.config.ts)
                                   ▼
                              Go API (:8080)
  adapter/http/cid.go            cidMiddleware: read or issue the `cid` cookie → customer ID
  adapter/http/handlers.go       calculate(): parse JSON → usecase.CalculateInput
  usecase/calculate_order.go     Execute():
        1. mergeItems       validate quantities, merge duplicate products
        2. resolveLines     load products, reject unknown ones            ──▶ Postgres (products)
        3. checkMember      normalize card, must exist and be ACTIVE      ──▶ Postgres (members)
        4. ActiveRules      build domain rules from table rows            ──▶ Postgres (rules)
        5. domain.Calculate pure pricing → Quote
        6. claimLimited     claim Red slot if ordered                     ──▶ Redis (Lua)
        7. Orders.Save      order + items + discounts, one transaction    ──▶ Postgres
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
  `SlotLimiter`, `Clock`, `IDGenerator`). The adapters implement them. This is "accept interfaces,
  return structs" and it is what makes everything testable with in-memory fakes
  (`internal/usecase/fakes_test.go`).
- **Composition root:** `cmd/server/main.go` is the only place that knows every concrete type.
- **Persistence models are separate from domain types:** `adapter/postgres/models.go` holds the
  GORM-tagged structs; each repository maps to/from `domain` types. The domain has no `gorm:"..."` tags.
- **What changes if Postgres is replaced?** Only `adapter/postgres` (and `migrations/`). The use
  cases and domain do not change.

## 5. The core code, explained

**Money** (`domain/money.go`): integer satang, discounts in basis points (500 = 5%), half-up rounding.

```go
func (m Money) ApplyBP(bp int) Money {
	return Money((int64(m)*int64(bp) + 5000) / 10000)
}
```

**Rules are an interface** (`domain/pricing.go`), run in priority order on the running subtotal, so
"member 10% comes after the pair discount" is data (`priority 10` vs `100`), not an `if`:

```go
type Rule interface {
	Code() string
	Priority() int // lower runs first
	Apply(lines []Line, subtotal Money, isMember bool) []Discount
}
```

`BundlePercent` (the pair rule) and `MemberPercent` are in `domain/rules.go`. A pair is
`quantity / bundleSize` complete groups of the *same* product, so Pink x4 is 2 pairs and Green x3 is 1 pair.

**Rules and limits come from tables** (`migrations/000002_seed.up.sql`): `rules`, `rule_products`,
`product_limits`. `adapter/postgres/rules.go` turns a row into a domain rule with `buildRule`; an
unknown rule type fails loudly instead of being ignored.

**The Red lock** (`adapter/redis/slot_limiter.go`) is one Lua script, so check-and-set is atomic:

```lua
local holder = redis.call('GET', KEYS[1])
if not holder then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if holder == ARGV[1] then
  return 1
end
return 0
```

Free → claim with a 1-hour TTL. Same owner → allowed, TTL **not** extended (so the Recalculate button
works but a customer cannot hold the slot forever). Different owner → `ErrSlotHeld` → HTTP 409.

**Customer identity** (`adapter/http/cid.go`): the API issues a random UUID in an HttpOnly cookie
`cid`. The frontend never sees or sends it; the browser attaches it. The Next.js rewrite keeps the
browser on one origin, so no CORS setup is needed.

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
- `calculate` is called from the browser (not a Server Action) because the `cid` cookie must travel
  with the browser's own request.

## 6. Decisions and trade-offs (say these in your own words)

| Decision | Why | Cost / alternative |
|---|---|---|
| Integer satang, basis points | No float rounding bugs; rounding is explicit and tested | Slightly less readable numbers |
| Rules as DB rows + Go constructors | New promo of an existing type is an `INSERT`; the rule logic stays unit-testable and pure | A new *kind* of promo still needs Go code; rules are loaded per request (cheap, cacheable later) |
| Atomic Lua claim, not `GET` then `SET` | Two concurrent requests cannot both see an empty slot | Needs Redis; if Redis restarts and loses the key, the slot frees early (use persistence/AOF if that matters) |
| Slot holder may re-claim, TTL not extended | Recalculate must not lock the customer out of their own order | Holder keeps the slot even if they remove Red and recalculate |
| Claim after validation | An invalid request cannot take the slot | If saving the order then fails, the slot stays held (documented limitation) |
| Anonymous `cid` cookie | No accounts in scope; unguessable; JS cannot read it | Clearing cookies gives a new identity. Real fix: authentication |
| GORM for queries, SQL files for schema | Readable, reviewable migrations and constraints; GORM removes row-scanning boilerplate | Two mechanisms (golang-migrate + GORM); AutoMigrate was the alternative |
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
- **Concurrency test.** `TestSlotLimiter_ExactlyOneOfManyConcurrentOwnersWins` runs 50 goroutines
  racing for the slot and asserts exactly one wins; run under `go test -race`.
- **Every PDF example is a test** (`domain/pdf_examples_test.go`): Orange x2 = 228.00, Pink x4 = 304.00,
  Green x3 = 116.00, and member 10% after pair discount = 205.20.
- **Real dependencies where it matters.** Repositories are tested against a real Postgres (not
  mocked), including "a failed save leaves no partial rows".
- **Real-world snags** that were caught by running the stack: a healthcheck that used `localhost`
  (IPv6 inside Alpine) while Fiber listens on IPv4; the API's default host port 8080 clashing with
  another service on the dev machine (so Compose publishes 18080); and Next.js 16's Cache Components
  mode, which rules out `export const dynamic = "force-dynamic"`.

## 8. Known limitations and what I would do in production

1. **Identity:** anonymous cookie can be reset by clearing cookies → add real authentication.
2. **Claim at checkout, not at quote:** today, pricing a Red order reserves the slot (as the task implies), so someone can hold it without buying → a separate checkout step that claims, and an expiry.
3. **Slot not released** if the customer removes Red and recalculates, or if saving the order fails.
4. **Rules/limits loaded on every request** → in-memory cache with a short TTL or invalidation on change.
5. **No rate limiting, no idempotency key** on `POST /api/calculate` → add both.
6. **Observability:** structured logs exist for 500s; add request IDs, metrics (latency, 409 rate), tracing.
7. **Members:** no registration or admin; cards are looked up only.
8. **Not verified in a real browser here** (no headless browser was installed); the UI is covered by component tests and a live request check through the proxy.

## 9. Likely questions, with model answers

**Why Clean Architecture for such a small app? When is it overkill?**
The task is scored on readability, maintainability, extensibility and logic, and Clean Architecture
gives a place for each: pricing in a pure domain package, I/O behind interfaces, a thin HTTP layer.
It is overkill for a one-off script or a CRUD admin with no rules. Here the rules are the point, and
I wanted to unit-test them with no database, so the boundary earns its cost.

**How do you guarantee only one customer gets the Red set under concurrent requests?**
The claim is a single Lua script that Redis executes atomically: it checks the key and sets it in
one step, so two requests cannot both see an empty slot. There is a test with 50 goroutines racing
that asserts exactly one wins, run with the race detector.

**Why Lua instead of `SETNX` plus `GET`?**
`SET key value NX PX ttl` alone covers "first wins", but I also need "the same owner may re-claim
without extending the TTL", which is a read-then-conditional-write. Doing that from Go would be two
round trips with a race in between; the script makes it one atomic operation.

**What if Redis restarts and loses the key?**
The slot frees early and a second customer could order Red within the hour. For this app that is
acceptable; if it matters I would enable AOF persistence, or keep the claim in Postgres with a
unique constraint and an expiry timestamp.

**Why integers for money? How do you round?**
Floats cannot represent most decimal amounts exactly. I store satang as `int64` and compute
discounts in basis points, rounding half-up per discount line (`ApplyBP`). The rule is tested at the
boundary: 0.5 satang rounds up, 0.45 rounds down.

**How would you add "buy 3, get 1 free"? A new limited product?**
New limited product: insert a row in `product_limits`, no code change, because the Redis key is per
product. A promotion of the existing "percent off complete groups" type is an `INSERT` into `rules`
and `rule_products`. A genuinely new kind of promotion needs a new `Rule` implementation in
`domain/rules.go` with its own tests and one new case in `buildRule`; no existing rule changes.

**How would you scale this to several API instances?**
The API is stateless: customer identity is in the cookie, the slot is in Redis, data is in
Postgres. So I can add instances behind a load balancer. The shared pieces to size are the Postgres
connection pool and Redis; migrations run at start-up and are idempotent, though in production I
would run them as a separate job.

**How do you test code that depends on time, randomness, Redis and Postgres?**
Time and IDs are injected (`Clock`, `IDGenerator`), so use cases are deterministic. Ports have
in-memory fakes for fast tests. Adapters are tested against the real thing: Redis behavior with
miniredis (including TTL expiry via `FastForward`), and Postgres with a real database in Docker.

**What are the security issues with the anonymous cookie?**
It is unguessable (a random UUID) and HttpOnly, so a script cannot read it, but it is not
authentication: clearing cookies creates a new identity, and anyone with the value can act as that
customer. In production I would add real accounts, set `Secure` (there is a `COOKIE_SECURE` setting),
and rate-limit the endpoint.

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
request IDs and metrics, an end-to-end browser test of the two-customer Red scenario, and CI running
the unit, integration and frontend tests.

**How do you review a junior's change in this codebase?**
First the dependency rule: does anything inward import something outward? Then: is there a test that
failed first, does it assert behavior rather than implementation, and are the error paths covered?
Then naming and whether the change belongs in the domain, a use case or an adapter.

## 10. Cheat sheet

1. Domain is pure; use cases own the interfaces; adapters implement them; `main.go` wires.
2. Money = int satang; discounts = basis points; round half-up.
3. Rules run by priority: pair (10) then member (100), member on the running subtotal.
4. Pair = same colour, `qty / 2` groups.
5. Red lock = one Lua script; holder may re-claim; TTL never extended; claim after validation.
6. Identity = HttpOnly `cid` cookie issued by the API; reached via a Next.js rewrite.
7. Errors: handlers return errors; one `mapError`; 500s never leak details.
8. Frontend: RSC shell + `<Suspense>` menu + one client island; reducer for state; Zod at the boundary.
9. TDD: red first, mutation check, concurrency test, PDF examples as tests.
10. Known gaps: auth, checkout-time claim, slot release, rule caching.

Commands:

```bash
docker compose up --build                       # run everything (UI :3000, API :18080)
cd backend && go test -race -cover ./...        # add DATABASE_URL=... for Postgres tests
cd frontend && npm test && npm run typecheck && npm run lint
```
