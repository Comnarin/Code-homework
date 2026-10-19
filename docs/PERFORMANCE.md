# Performance

Status: **template. No load-test numbers have been measured yet** (see "Why nothing is filled in").
Every cell that says `not measured` is waiting for a real run; do not replace it with an estimate.

## Why nothing is filled in

The scenarios and tooling are implemented and CI-wired (`loadtest/`, job `loadtest-smoke`), but
they could not be run where this was written: the development machine's disk was full, which wedged
the Docker daemon (builds failed with an I/O error and `docker ps` hung), so the isolated
`foodstore-load` stack could not be started. What *was* verified without Docker, against real
Postgres and an in-memory Redis, is correctness under concurrency (not speed):

| Check | Result |
|---|---|
| 150 goroutines save a Red order with distinct keys at the same instant (Postgres only) | 1 success, 149 `SlotHeldError`, 0 other errors, 1 window anchor row |
| 120 concurrent HTTP requests for Red with Redis stopped (`internal/e2e`) | exactly one 200, 119 409, no 5xx, p99 under the 1 s assertion |
| Two goroutines x 50 orders with two limited products in opposite item order | no deadlock |

## Environment

Fill this in per run.

| Item | Value |
|---|---|
| Commit SHA | not measured (record `git rev-parse --short HEAD`) |
| Host | the template was prepared on an Apple M1, 8 cores, 16 GB; record the machine you actually measure on |
| Docker Desktop CPU / memory limit | not measured |
| Container limits (`docker stats`, any `deploy.resources`) | none set in `docker-compose.yml` |
| Go version | go1.26 (`backend/go.mod`) |
| DB pool | `MaxOpenConns=20`, `MaxIdleConns=5` (`internal/platform/db/db.go`) |
| Redis | `redis:7-alpine`, client timeouts 250/200/200 ms, breaker cooldown 5 s |
| Postgres | `postgres:17-alpine`, default settings |

## How to run

```bash
./loadtest/run.sh smoke.js
./loadtest/run.sh load.js
./loadtest/run.sh stress.js
./loadtest/run.sh spike.js
./loadtest/run.sh soak.js                 # manual, 30 minutes
./loadtest/run.sh red-contention.js       # Redis up
./loadtest/redis-down.sh                  # Redis down, then recovery
OBSERVE=1 ./loadtest/run.sh load.js       # same, with Grafana at http://localhost:23001
./loadtest/run.sh down                    # delete the load stack and its data
```

Read the p50/p95/p99 and error rate from the k6 summary. Read CPU, pool and Redis figures from the
Grafana dashboard "Food store API" (run with `OBSERVE=1`) or `docker stats`. Take `go_sql_wait_count_total`
and `go_sql_wait_duration_seconds_total` before and after the run and record the difference.

## Results

Run each scenario against a fresh stack. "Before" is the baseline you measure first; "after" is the
same scenario after a change you want to evaluate. Leave unmeasured cells as `not measured`.

### Steady scenarios

| Scenario | Target rps | Achieved rps | p50 | p95 | p99 | Error % | 5xx | api CPU | postgres CPU | go_sql wait count / duration delta | Peak in-use conns | Redis p95 | Heap after soak |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| smoke (2 VUs, 30 s) before | n/a | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| smoke after | n/a | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| load (5 min) before | 50 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| load after | 50 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| stress (50 to 400, 10 min) before | 50 to 400 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| stress after | 50 to 400 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| spike (10 to 300) before | 10 to 300 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| spike after | 10 to 300 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | n/a |
| soak (30 rps, 30 min) before | 30 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured |
| soak after | 30 | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured | not measured |

### Red contention (100 buyers, distinct keys, one instant)

| Case | Winners | 409 | 5xx | p95 | Notes |
|---|---|---|---|---|---|
| Redis up, before | not measured | not measured | not measured | not measured | |
| Redis up, after | not measured | not measured | not measured | not measured | |
| Redis down (`redis-down.sh`), before | not measured | not measured | not measured | not measured | threshold: p95 < 1 s |
| Redis down, after | not measured | not measured | not measured | not measured | |
| After Redis recovery: new buyer / winner | not measured | | | | expect 409 / 200 |

## Conclusion and bottleneck

Not measured yet. After the first real run, write here which resource saturates first (CPU of the API,
Postgres, the 20-connection pool, Redis) and what the evidence is (for example `go_sql_wait_duration_seconds_total`
rising, or p99 climbing while CPU is idle).

## Catalog caching (decision)

A read-through cache for products, rules and limits (`CATALOG_CACHE_TTL`, off by default) was **not**
implemented. It is only justified if a measured baseline shows pool waits or catalog queries dominating
latency, and there is no baseline yet. If it is added later, limit enforcement is unaffected: `OrderRepository.Save`
re-reads `product_limits` with `FOR UPDATE` inside the transaction. Any cache must not cache errors.

## Known trade-offs of the Red-limit design

- The app clock decides the window in Postgres, Redis uses its own TTL: small skew is possible.
- After an outage Redis can block the same holder slightly longer than Postgres would.
- A leftover Redis key with no matching order over-blocks until its TTL ends; it never double-sells.
- On the Postgres path, orders for the same limited product are serialized by a row lock.
- `OrderRepository.Save` enforces limits (documented on the port).
- Redis is a soft dependency: `/readyz` reports `degraded` instead of failing.
