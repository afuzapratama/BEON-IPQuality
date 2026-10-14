#!/usr/bin/env bash
set -euo pipefail
beon_test_root="$(cd "$(dirname "$0")/../.." && pwd)"
beon_test_dir="$(mktemp -d)"
export BEON_ENV_FILE="$beon_test_dir/runtime.env"
export BEON_LOCK_FILE="$beon_test_dir/deployment-lock.env"
export BEON_COMPOSE_FILE="$beon_test_dir/compose.json"
source "$beon_test_root/scripts/lib/production.sh"
cleanup() {
  if [[ -f "$BEON_COMPOSE_FILE" ]]; then beon_compose down -v --remove-orphans >/dev/null 2>&1 || true; fi
  rm -rf "$beon_test_dir"
}
trap cleanup EXIT
umask 077
mkdir -p "$beon_test_dir/project/configs" "$beon_test_dir/project/migrations" "$beon_test_dir/project/scripts" "$beon_test_dir/mmdb" "$beon_test_dir/feed"
cp "$beon_test_root/configs/config.yaml" "$beon_test_dir/project/configs/"
cp "$beon_test_root/migrations/"[0-9][0-9][0-9]_*.sql "$beon_test_dir/project/migrations/"
cp "$beon_test_root/scripts/migrate.sh" "$beon_test_dir/project/scripts/"
cat > "$beon_test_dir/project/configs/feeds.yaml" <<'FEEDS'
feeds:
  fixture:
    enabled: true
    name: Fixture
    threat_type: proxy
    confidence: 0.8
    weight: 40
    schedule: '@hourly'
    sources:
      - name: fixture
        url: http://fixture:8000/list.txt
        format: plain
formats:
  plain:
    description: Fixture
whitelist:
  enabled: true
  sources:
    - name: fixture
      ranges: [8.8.8.9]
FEEDS
printf '8.8.8.0/24\n2606:4700:4700::1111\n' > "$beon_test_dir/feed/list.txt"
# Public fixture data only; container UID can write/read the temporary bind mount.
chmod 0777 "$beon_test_dir/mmdb"
chmod 0755 "$beon_test_dir/project/configs" "$beon_test_dir/feed"
chmod 0644 "$beon_test_dir/project/configs/"* "$beon_test_dir/feed/list.txt"
cat > "$BEON_ENV_FILE" <<ENV
COMPOSE_PROJECT_NAME=beon-ops-$$-${RANDOM}
PROJECT_ROOT=$beon_test_dir/project
MMDB_HOST_DIR=$beon_test_dir/mmdb
POSTGRES_PASSWORD=fixture-only
REDIS_PASSWORD=fixture-only
API_KEY=01234567890123456789012345678901
ADMIN_API_KEY=admin01234567890123456789012345678
API_PORT=0
API_MEMORY=256m
INGESTOR_MEMORY=512m
COMPILER_MEMORY=512m
POSTGRES_MEMORY=256m
REDIS_MEMORY=64m
REDIS_MAXMEMORY=32mb
ENV
cat > "$BEON_LOCK_FILE" <<'LOCK'
API_IMAGE=beon-phase3-api:local
INGESTOR_IMAGE=beon-phase3-ingestor:local
COMPILER_IMAGE=beon-phase3-compiler:local
JUDGE_IMAGE=beon-phase3-judge:local
LOCK
cat > "$beon_test_dir/override.yml" <<OVERRIDE
services:
  fixture:
    image: python:3.12-slim
    command: [python, -m, http.server, '8000', --directory, /fixture]
    volumes: ['$beon_test_dir/feed:/fixture:ro']
    networks: [beon]
  ingestor-init:
    depends_on:
      fixture: {condition: service_started}
OVERRIDE
# Resolve the production definition once; backup uses exactly this private stack.
docker compose --project-directory "$beon_test_root" --env-file "$BEON_ENV_FILE" --env-file "$BEON_LOCK_FILE" -f "$beon_test_root/deployments/compose.production.yml" -f "$beon_test_dir/override.yml" config --format json > "$BEON_COMPOSE_FILE"
beon_compose up -d >/dev/null
beon_test_healthy=false
for beon_attempt in {1..60}; do
  beon_api_address="$(beon_compose port api 8080 2>/dev/null || true)"
  if [[ -n "$beon_api_address" ]] && curl -fsS "http://$beon_api_address/health" > "$beon_test_dir/health.json" 2>/dev/null; then beon_test_healthy=true; break; fi
  sleep 1
done
if [[ "$beon_test_healthy" != true ]]; then beon_compose logs --tail 60; exit 1; fi
curl -fsS -H 'X-API-Key: 01234567890123456789012345678901' "http://$beon_api_address/api/v1/check/8.8.8.8" > "$beon_test_dir/result.json"
python3 - "$beon_test_dir/result.json" <<'PY'
import json,sys
result=json.load(open(sys.argv[1]))
assert result['score']>0 and result['proxy'] and result['data_version']
PY
[[ "$(curl -s -o /dev/null -w '%{http_code}' "http://$beon_api_address/api/v1/check/8.8.8.8")" == 401 ]]
beon_compose run --rm migrate >/dev/null
cp "$beon_test_dir/project/migrations/002_source_expiry.sql" "$beon_test_dir/original.sql"
printf '\n-- Deliberate checksum mutation in fixture only\n' >> "$beon_test_dir/project/migrations/002_source_expiry.sql"
if beon_compose run --rm migrate >/dev/null 2>&1; then echo 'Changed migration checksum was accepted' >&2; exit 1; fi
cp "$beon_test_dir/original.sql" "$beon_test_dir/project/migrations/002_source_expiry.sql"
BACKUP_DIR="$beon_test_dir/backups" "$beon_test_root/scripts/backup.sh"
beon_backup="$(find "$beon_test_dir/backups" -mindepth 1 -maxdepth 1 -type d)"
# Restore checks must not leave anonymous PostgreSQL volumes behind.
docker volume ls -q | sort > "$beon_test_dir/volumes-before-restore"
"$beon_test_root/scripts/restore-check.sh" "$beon_backup"
docker volume ls -q | sort > "$beon_test_dir/volumes-after-restore"
diff "$beon_test_dir/volumes-before-restore" "$beon_test_dir/volumes-after-restore"
echo 'Production Compose bootstrap, migration rerun/checksum rejection, backup and isolated restore passed.'
