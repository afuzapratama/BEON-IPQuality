#!/usr/bin/env bash
set -euo pipefail
beon_migrations_dir="${MIGRATIONS_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../migrations" && pwd)}"
beon_migration_sql="$(mktemp)"
trap 'rm -f "$beon_migration_sql"' EXIT
chmod 600 "$beon_migration_sql"
cat > "$beon_migration_sql" <<'SQL'
SELECT pg_advisory_lock(741852963);
CREATE TABLE IF NOT EXISTS schema_migrations (
 version TEXT PRIMARY KEY,
 checksum TEXT NOT NULL,
 applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SQL
for beon_migration in "$beon_migrations_dir"/[0-9][0-9][0-9]_*.sql; do
  [[ -f "$beon_migration" ]] || continue
  beon_migration_name="$(basename "$beon_migration")"
  [[ "$beon_migration_name" =~ ^[0-9]{3}_[a-z0-9_]+\.sql$ ]] || { echo 'Invalid migration filename' >&2; exit 1; }
  beon_migration_checksum="$(sha256sum "$beon_migration" | cut -d' ' -f1)"
  cat >> "$beon_migration_sql" <<SQL
DO \$check\$ BEGIN
 IF EXISTS (SELECT 1 FROM schema_migrations WHERE version='$beon_migration_name' AND checksum<>'$beon_migration_checksum') THEN
 RAISE EXCEPTION 'Migration checksum mismatch: $beon_migration_name';
 END IF;
END \$check\$;
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version='$beon_migration_name') AS already_applied \gset
\if :already_applied
\else
BEGIN;
SQL
  cat "$beon_migration" >> "$beon_migration_sql"
  cat >> "$beon_migration_sql" <<SQL

INSERT INTO schema_migrations(version,checksum) VALUES ('$beon_migration_name','$beon_migration_checksum');
COMMIT;
\endif
SQL
done
printf 'SELECT pg_advisory_unlock(741852963);\n' >> "$beon_migration_sql"
if [[ -n "${DATABASE_URL:-}" ]]; then
  # libpq reads connection data from the environment instead of process args.
  export PGDATABASE="$DATABASE_URL"
fi
psql --no-psqlrc --set ON_ERROR_STOP=1 --file "$beon_migration_sql"
