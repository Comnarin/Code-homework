#!/usr/bin/env bash
# Runs load tests against an isolated copy of the stack (compose project
# "foodstore-load", its own database, Redis DB and host ports), so your
# development data is never touched.
#
#   ./loadtest/run.sh smoke.js            start the stack if needed, run a scenario
#   ./loadtest/run.sh up                  only start the stack
#   ./loadtest/run.sh down                stop it and delete its data (down -v)
#   OBSERVE=1 ./loadtest/run.sh load.js   also start Prometheus/Grafana/Jaeger and push k6 metrics
#
# Extra arguments go to k6, for example: ./loadtest/run.sh red-contention.js -e N=200
set -euo pipefail
cd "$(dirname "$0")/.."

PROJECT="${COMPOSE_PROJECT-foodstore-load}"
dc() {
  docker compose ${PROJECT:+-p "$PROJECT"} --env-file .env --env-file loadtest/loadtest.env "$@"
}

case "${1:-}" in
  "" | -h | --help)
    sed -n '2,11p' "$0"
    exit 0
    ;;
  down)
    dc --profile loadtest --profile observability down -v --remove-orphans
    exit 0
    ;;
esac

if [ "${OBSERVE:-}" = "1" ]; then
  export OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_EXPORTER_OTLP_ENDPOINT:-http://jaeger:4318}"
  dc --profile observability up -d --build --wait postgres redis minio api prometheus jaeger grafana
else
  dc up -d --build --wait postgres redis minio api
fi

if [ "$1" = "up" ]; then
  exit 0
fi

script="$1"
shift
mkdir -p loadtest/out
chmod 777 loadtest/out # the k6 container runs as a non-root user

docker_env=()
k6_args=()
if [ "${OBSERVE:-}" = "1" ]; then
  docker_env+=(-e K6_PROMETHEUS_RW_SERVER_URL=http://prometheus:9090/api/v1/write)
  k6_args+=(-o experimental-prometheus-rw)
fi
for var in N DEGRADED; do
  if [ -n "${!var:-}" ]; then
    k6_args+=(-e "$var=${!var}")
  fi
done

# ${arr[@]+...} keeps empty arrays working on macOS's old bash.
dc --profile loadtest run --rm --no-deps ${docker_env[@]+"${docker_env[@]}"} k6 run ${k6_args[@]+"${k6_args[@]}"} "$@" "/scripts/${script}"
