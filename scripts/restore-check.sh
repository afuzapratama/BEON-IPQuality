#!/usr/bin/env bash
set -euo pipefail
beon_backup="${1:?Usage: restore-check.sh /absolute/backup-directory}"
[[ -f "$beon_backup/COMPLETE" ]] || { echo 'Backup is incomplete' >&2; exit 1; }
beon_backup="$(cd "$beon_backup" && pwd)"
(cd "$beon_backup" && sha256sum --check SHA256SUMS)
beon_restore_root="$(mktemp -d)"
beon_restore_files="$beon_restore_root/mmdb"
mkdir -p "$beon_restore_files"
beon_restore_container="beon-restore-check-$$-${RANDOM}"
trap 'docker rm -f "$beon_restore_container" >/dev/null 2>&1 || true; rm -rf "$beon_restore_root"' EXIT
beon_compiler_image="$(sed -n 's/^COMPILER_IMAGE=//p' "$beon_backup/deployment-lock.env")"
[[ -n "$beon_compiler_image" ]] || { echo 'Backup has no compiler image reference' >&2; exit 1; }
tar -xzf "$beon_backup/mmdb.tar.gz" -C "$beon_restore_files" --no-same-owner
chmod 0755 "$beon_restore_files"
find "$beon_restore_files" -type f -name '*.mmdb' -exec chmod 0644 {} +
docker run --rm --network none --read-only -v "$beon_restore_files:/snapshot:ro" "$beon_compiler_image" -verify-mmdb /snapshot
docker run -d --name "$beon_restore_container" -e POSTGRES_PASSWORD=restore-check-only -e POSTGRES_DB=beon_restore postgres:17-alpine >/dev/null
beon_restore_ready=false
for beon_attempt in {1..30}; do
  if docker exec "$beon_restore_container" pg_isready -h 127.0.0.1 -U postgres -d beon_restore >/dev/null 2>&1; then beon_restore_ready=true; break; fi
  sleep 1
done
[[ "$beon_restore_ready" == true ]] || { echo 'Restore target did not become ready' >&2; exit 1; }
docker exec -i "$beon_restore_container" pg_restore -U postgres -d beon_restore --exit-on-error --no-owner --no-acl < "$beon_backup/postgres.dump"
docker exec "$beon_restore_container" psql -U postgres -d beon_restore -v ON_ERROR_STOP=1 -c 'SELECT count(*) AS reputation_rows FROM ip_reputation; SELECT version FROM schema_migrations ORDER BY version;'
echo 'Restore check succeeded on a temporary isolated database; production was not modified.'
