#!/usr/bin/env bash
set -euo pipefail
beon_root="$(cd "$(dirname "$0")/../.." && pwd)"
beon_nginx_tmp="$(mktemp -d)"
trap 'rm -rf "$beon_nginx_tmp"' EXIT
openssl req -x509 -nodes -newkey rsa:2048 -days 1 -subj /CN=ipq.example.com -keyout "$beon_nginx_tmp/privkey.pem" -out "$beon_nginx_tmp/fullchain.pem" >/dev/null 2>&1
docker run --rm --network none -v "$beon_root/configs/nginx/beon-production.conf:/etc/nginx/conf.d/beon.conf:ro" -v "$beon_nginx_tmp:/etc/letsencrypt/live/ipq.example.com:ro" nginx:alpine nginx -t
