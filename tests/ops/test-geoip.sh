#!/usr/bin/env bash
set -euo pipefail
beon_test_root="$(cd "$(dirname "$0")/../.." && pwd)"
beon_test_dir="$(mktemp -d)"
trap 'rm -rf "$beon_test_dir"' EXIT
mkdir -p "$beon_test_dir/bin" "$beon_test_dir/private folder"
printf 'AccountID 123\nLicenseKey fixture-only\nEditionIDs GeoLite2-City GeoLite2-ASN\nDatabaseDirectory /unused\n' > "$beon_test_dir/private folder/GeoIP.conf"
beon_original_hash="$(sha256sum "$beon_test_dir/private folder/GeoIP.conf" | cut -d' ' -f1)"
cat > "$beon_test_dir/bin/geoipupdate" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == -f && -r "$2" && "$3" == -d ]] || exit 1
printf 'fixture-city' > "$4/GeoLite2-City.mmdb"
printf 'fixture-asn' > "$4/GeoLite2-ASN.mmdb"
MOCK
chmod +x "$beon_test_dir/bin/geoipupdate"
PATH="$beon_test_dir/bin:$PATH" "$beon_test_root/scripts/update-geoip.sh" --config "$beon_test_dir/private folder/GeoIP.conf" --data-dir "$beon_test_dir/project data/mmdb"
[[ -s "$beon_test_dir/project data/mmdb/GeoLite2-ASN.mmdb" ]]
[[ "$beon_original_hash" == "$(sha256sum "$beon_test_dir/private folder/GeoIP.conf" | cut -d' ' -f1)" ]]
if PATH="$beon_test_dir/bin:$PATH" "$beon_test_root/scripts/update-geoip.sh" --config "$beon_test_dir/missing" --data-dir "$beon_test_dir/project data/mmdb" >/dev/null 2>&1; then exit 1; fi
echo 'GeoIP credential path/output path checks passed; no MaxMind downloads were performed.'
