#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/production.sh"
umask 077
beon_backup_root="${BACKUP_DIR:-${beon_project_root}/backups}"
beon_backup="${beon_backup_root}/$(date -u +%Y%m%dT%H%M%SZ)-${RANDOM}"
mkdir -p "$beon_backup"
beon_mmdb_dir="$(beon_mmdb_directory)"
beon_compose exec -T postgres sh -c 'export PGPASSWORD="$POSTGRES_PASSWORD"; exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc --no-owner --no-acl' > "$beon_backup/postgres.dump"
cp "$beon_env_file" "$beon_backup/runtime.env"
cp "$beon_lock_file" "$beon_backup/deployment-lock.env"
tar -C "$beon_project_root" --exclude=configs/GeoIP.conf --exclude=configs/GeoIP.conf.local --exclude='*.key' --exclude='*.pem' -czf "$beon_backup/config.tar.gz" configs migrations deployments scripts README.md docs
beon_mmdb_files=(reputation.mmdb)
for beon_optional in GeoLite2-City.mmdb GeoLite2-ASN.mmdb; do
  if [[ -f "$beon_mmdb_dir/$beon_optional" ]]; then beon_mmdb_files+=("$beon_optional"); fi
done
tar -C "$beon_mmdb_dir" -czf "$beon_backup/mmdb.tar.gz" "${beon_mmdb_files[@]}"
if [[ -n "${GEOIP_CONFIG_FILE:-}" ]]; then cp "$GEOIP_CONFIG_FILE" "$beon_backup/GeoIP.conf"; fi
(
  cd "$beon_backup"
  sha256sum postgres.dump runtime.env deployment-lock.env config.tar.gz mmdb.tar.gz > SHA256SUMS
  if [[ -f GeoIP.conf ]]; then sha256sum GeoIP.conf >> SHA256SUMS; fi
)
# A marker distinguishes finished bundles from interrupted backups.
printf '%s\n' 'complete' > "$beon_backup/COMPLETE"
echo "Backup complete: $beon_backup"
echo 'Bundle includes credentials; encrypt before transferring to offsite storage.'
