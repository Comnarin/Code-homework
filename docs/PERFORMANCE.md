# Performance

These are real measurements from one run on a laptop (see "Environment"). They show how the API behaves
at this size, not how it would behave in production. Cells that say `not measured` were not run.

Everything below runs against an isolated copy of the stack (compose project `foodstore-load`: its own
Postgres database, Redis DB and host ports), driven by k6 in a container. See `loadtest/` and the README
section "Load testing".

## Environment

| Item | Value |
|---|---|
| Commit | `3aa1468` plus the `loadtest/` additions on top |
| Host | Apple M1, 8 cores, 16 GB, macOS (Docker Desktop, 7.65 GiB given to Docker) |
| Load generator | k6 1.2.0 in a container on the same machine, so it competes with the stack for CPU |
| Go / DB pool | go1.26, `MaxOpenConns=20`, `MaxIdleConns=5` (`internal/platform/db/db.go`) |
| Redis | `redis:7-alpine`, client timeouts 250/200/200 ms, circuit breaker cooldown 5 s |
| Postgres | `postgres:17-alpine`, default settings |
| Container limits | none set in `docker-compose.yml` |

## Results

### Steady and spike scenarios

Traffic mix: 60% menu, 10% promotions, 30% calculate (about half with a member card, about 2% of the
calculates ask for Red with a random client key).

| Scenario | Rate | Requests | p50 | p95 | p99 | Max | Errors | 5xx |
|---|---|---|---|---|---|---|---|---|
| smoke (2 VUs, 30 s) | about 19 req/s | 562 | 1.58 ms | 9.25 ms | not recorded | 153 ms | 0 | 0 |
| load (50 req/s, 5 min) | 50.0 req/s (target held) | 15,002 | 1.55 ms | 8.46 ms | 25.63 ms | 744 ms | 0 | 0 |
| spike (10 to 300 req/s, 80 s) | about 264 req/s average | 21,101 | 0.53 ms | 4.02 ms | not recorded | 228 ms | 0 | 0 |
| stress (50 to 400 req/s, 10 min) | | | not measured | not measured | not measured | not measured | | |
| soak (30 req/s, 30 min) | | | not measured | not measured | not measured | not measured | | |

All k6 thresholds passed for smoke, load and spike. Memory after the load run: api 26 MiB,
Postgres 47 MiB, Redis 10 MiB, MinIO 101 MiB.

A first attempt at the load scenario was discarded: the laptop went to sleep mid-run, which made k6 report a
16-minute request and a failed p99 threshold. The numbers above come from a rerun with the machine kept awake
(`caffeinate`).

### Red contention: 100 buyers, distinct client keys, all at the same instant

| Case | Winners | 409 | 5xx | p95 | Max |
|---|---|---|---|---|---|
| Redis up | 1 | 99 | 0 | 221 ms | 236 ms |
| Redis stopped (`redis-down.sh`) | 1 | 99 | 0 | 225 ms | 251 ms |

With Redis stopped, `/readyz` answered `200 {"status":"degraded"}`. After Redis was started again and
`/readyz` was `ok`, a new buyer got 409 and the winner got 200, so Postgres still remembered the window.
The high p95 here is the cost of 100 requests arriving in one batch, not of a single request.

Related correctness checks that run in `go test` (not load tests): 150 goroutines saving a Red order with
distinct keys give 1 success and 149 `SlotHeldError`; 120 concurrent HTTP requests with Redis stopped give one
200 and 119 409; two goroutines ordering two limited products in opposite order do not deadlock.

## What this shows

- At 50 req/s the API stays around 1.5 ms median and under 10 ms at p95, using about 26 MiB of memory.
  Nothing saturated, so this run does not identify a bottleneck. To find one, run `stress.js` and read the
  CPU of the api and Postgres containers and the 20-connection pool wait time.
- The Red rule holds under contention and through a Redis outage and recovery, with no 5xx.
- Taking Redis away does not change the p95 of the Red race in this test (221 ms vs 225 ms). The Postgres row
  lock decides the race either way.

## Not done, on purpose

- **Catalog caching.** A cache for products, rules and limits was not added. At 50 req/s the median is
  1.5 ms, so there is no measured reason for it. If added later it must not cache errors, and limit
  enforcement is unaffected because `OrderRepository.Save` re-reads `product_limits` with `FOR UPDATE`
  inside the transaction.
- **stress and soak runs.** They are implemented and documented in the README but were not run for this file.

## Known trade-offs of the Red-limit design

- The app clock decides the window in Postgres, and Redis uses its own TTL, so small skew is possible.
- After an outage, a holder who orders again writes the Redis key with a full-length TTL, which can block
  other buyers in Redis for longer than Postgres would. It never causes a double sale.
- A leftover Redis key with no matching order over-blocks until its TTL ends. It never double-sells.
- On the Postgres path, orders for the same limited product are serialized by a row lock.
- Every Calculate saves an order, so one anonymous request for Red can hold the window for an hour.
- Redis is a soft dependency: `/readyz` reports `degraded` instead of failing.
