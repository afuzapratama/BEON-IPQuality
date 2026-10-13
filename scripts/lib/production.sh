#!/usr/bin/env bash
beon_project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
beon_env_file="${BEON_ENV_FILE:-${beon_project_root}/.env}"
beon_lock_file="${BEON_LOCK_FILE:-${beon_project_root}/deployment-lock.env}"
beon_compose_file="${BEON_COMPOSE_FILE:-${beon_project_root}/deployments/compose.production.yml}"
beon_compose() {
  docker compose --project-directory "$beon_project_root" --env-file "$beon_env_file" --env-file "$beon_lock_file" -f "$beon_compose_file" "$@"
}
beon_mmdb_directory() {
  beon_compose config --format json | python3 -c 'import json,sys; c=json.load(sys.stdin); print(next(v["source"] for v in c["services"]["compiler"]["volumes"] if v["target"]=="/app/data/mmdb"))'
}
