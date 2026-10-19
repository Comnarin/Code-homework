#!/usr/bin/env bash
# Proves the Red rule survives a Redis outage and recovery:
#   1. reset, stop Redis
#   2. 100 buyers race for Red: exactly one 200, the rest 409, no 5xx, p95 < 1s
#   3. start Redis, wait for /readyz "ok"
#   4. a new buyer must get 409 (Postgres remembers the winner) and the winner 200
# Exits non-zero on any mismatch. Uses the isolated "foodstore-load" project;
# override with COMPOSE_PROJECT (empty string = the default project).
set -euo pipefail
cd "$(dirname "$0")/.."

PROJECT="${COMPOSE_PROJECT-foodstore-load}"
dc() {
  docker compose ${PROJECT:+-p "$PROJECT"} --env-file .env --env-file loadtest/loadtest.env "$@"
}
set -a
# shellcheck disable=SC1091
. loadtest/loadtest.env
set +a
API_URL="${API_URL:-http://localhost:${API_HOST_PORT}}"

fail() { echo "FAIL: $*" >&2; exit 1; }
red_status() {
  curl -s -o /dev/null -w '%{http_code}' -X POST "$API_URL/api/calculate" \
    -H 'Content-Type: application/json' -H "X-Client-Key: $1" \
    -d '{"items":[{"productId":"red","quantity":1}]}'
}

mkdir -p loadtest/out
chmod 777 loadtest/out
rm -f loadtest/out/winner.txt

echo "== reset limits"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API_URL/api/test/reset-limits")
[ "$code" = "204" ] || fail "reset-limits returned $code (is the stack up with ENABLE_TEST_TOOLS=true?)"

restore_redis() { dc start redis >/dev/null 2>&1 || true; }
trap restore_redis EXIT

echo "== stop redis"
dc stop redis

echo "== red race with Redis down"
dc --profile loadtest run --rm --no-deps -e N=100 k6 run -e N=100 -e DEGRADED=1 /scripts/red-contention.js \
  || fail "red-contention with Redis down"
[ -s loadtest/out/winner.txt ] || fail "no winner key recorded"
winner=$(cat loadtest/out/winner.txt)
echo "winner key: $winner"

echo "== readyz while Redis is down (expect degraded, still 200)"
body=$(curl -s -w ' %{http_code}' "$API_URL/readyz")
echo "$body"
case "$body" in
  *degraded*200) ;;
  *) fail "readyz should be 200 degraded while Redis is down" ;;
esac

echo "== start redis"
dc start redis
for _ in $(seq 1 60); do
  if curl -s "$API_URL/readyz" | grep -q '"ok"'; then
    ready=1
    break
  fi
  sleep 1
done
[ "${ready:-}" = "1" ] || fail "readyz never returned ok after Redis came back"

echo "== after recovery"
[ "$(red_status "late-$RANDOM-$RANDOM")" = "409" ] || fail "a new buyer must be rejected after Redis recovers"
[ "$(red_status "$winner")" = "200" ] || fail "the winner must still be accepted after Redis recovers"
echo "PASS: Red rule held through the Redis outage and recovery"
