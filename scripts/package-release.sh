#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
beon_package_arch="${GOARCH:-arm64}"
beon_package_version="${VERSION:-dev}"
beon_package_output="${RELEASE_OUTPUT_DIR:-dist}"
[[ "$beon_package_arch" == arm64 || "$beon_package_arch" == amd64 ]] || exit 2
[[ "$beon_package_version" =~ ^[a-zA-Z0-9._-]+$ ]] || exit 2
mkdir -p "$beon_package_output"
beon_package_output="$(cd "$beon_package_output" && pwd)"
beon_package_stage="$(mktemp -d)"
trap 'rm -rf "$beon_package_stage"' EXIT
beon_package_time="${BUILD_TIME:-$(git show -s --format=%cI HEAD)}"
for beon_binary in api ingestor compiler judge; do
  beon_flags='-s -w'
  if [[ "$beon_binary" == api ]]; then beon_flags+=" -X main.version=${beon_package_version} -X main.buildTime=${beon_package_time}"; fi
  if [[ "$beon_binary" == ingestor ]]; then beon_flags+=" -X main.Version=${beon_package_version} -X main.BuildTime=${beon_package_time}"; fi
  CGO_ENABLED=0 GOOS=linux GOARCH="$beon_package_arch" go build -trimpath -ldflags "$beon_flags" -o "$beon_package_stage/$beon_binary" "./cmd/$beon_binary"
done
# Only tracked source/support files are packaged. Ignored credentials, database
# files, host .env and backups cannot accidentally enter release archives.
while IFS= read -r -d '' beon_file; do
  mkdir -p "$beon_package_stage/$(dirname "$beon_file")"
  cp "$beon_file" "$beon_package_stage/$beon_file"
done < <(git ls-files -z -- configs migrations deployments scripts docs .env.example README.md LICENSE)
beon_package_name="beon-ipquality_${beon_package_version}_linux_${beon_package_arch}.tar.gz"
tar -C "$beon_package_stage" -czf "$beon_package_output/$beon_package_name" .
(cd "$beon_package_output" && sha256sum "$beon_package_name" > "${beon_package_name%.tar.gz}.sha256")
echo "$beon_package_output/$beon_package_name"
