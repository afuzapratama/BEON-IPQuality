#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
beon_test_suffix="$$-${RANDOM}"
beon_test_postgres="beon-integration-postgres-${beon_test_suffix}"
beon_test_redis="beon-integration-redis-${beon_test_suffix}"
cleanup() {
  docker rm -f "$beon_test_postgres" "$beon_test_redis" >/dev/null 2>&1 || true
}
trap cleanup EXIT
# Temporary containers, no host volumes; ports bind only to loopback.
docker run -d --name "$beon_test_postgres" -e POSTGRES_PASSWORD=integration-only -e POSTGRES_DB=beon_test -p 127.0.0.1::5432 postgres:17-alpine >/dev/null
docker run -d --name "$beon_test_redis" -p 127.0.0.1::6379 redis:7-alpine >/dev/null
beon_test_ready=false
for beon_test_attempt in {1..30}; do
  if docker exec "$beon_test_postgres" pg_isready -h 127.0.0.1 -U postgres -d beon_test >/dev/null 2>&1 && docker exec "$beon_test_redis" redis-cli ping >/dev/null 2>&1; then
    beon_test_ready=true
    break
  fi
  sleep 1
done
if [[ "$beon_test_ready" != true ]]; then
  echo "Integration services did not become ready" >&2
  exit 1
fi
beon_test_pg_port="$(docker port "$beon_test_postgres" 5432/tcp | cut -d: -f2)"
beon_test_redis_port="$(docker port "$beon_test_redis" 6379/tcp | cut -d: -f2)"
export BEON_INTEGRATION_DATABASE_URL="postgresql://postgres:integration-only@127.0.0.1:${beon_test_pg_port}/beon_test?sslmode=disable"
export BEON_INTEGRATION_REDIS_URL="redis://127.0.0.1:${beon_test_redis_port}"
go test -race -count=1 ./tests/integration
