#!/usr/bin/env bash
set -euo pipefail
beon_monitor_root="$(cd "$(dirname "$0")/../.." && pwd)"
beon_monitor_tmp="$(mktemp -d)"
trap 'rm -rf "$beon_monitor_tmp"' EXIT
python3 - "$beon_monitor_root" "$beon_monitor_tmp" <<'PY'
import json,sys,yaml
from pathlib import Path
root=Path(sys.argv[1]);tmp=Path(sys.argv[2])
compose=yaml.safe_load((root/'deployments/compose.monitoring.yml').read_text())
image=compose['services']['prometheus']['image']
(tmp/'image').write_text(image)
dashboard=json.loads((root/'configs/grafana/dashboards/production.json').read_text())
# promtool parses every dashboard expression as a recording rule.
rules={'groups':[{'name':'dashboard-validation','rules':[{'record':'beon_dashboard_panel_'+str(panel['id']),'expr':target['expr']} for panel in dashboard['panels'] for target in panel['targets']]}]}
(tmp/'dashboard-rules.yml').write_text(yaml.safe_dump(rules))
PY
beon_prometheus_image="$(cat "$beon_monitor_tmp/image")"
docker run --rm --network none --entrypoint /bin/promtool -v "$beon_monitor_root/configs/prometheus/production.yml:/etc/prometheus/prometheus.yml:ro" "$beon_prometheus_image" check config /etc/prometheus/prometheus.yml
docker run --rm --network none --entrypoint /bin/promtool -v "$beon_monitor_tmp/dashboard-rules.yml:/rules.yml:ro" "$beon_prometheus_image" check rules /rules.yml
# Syntax validation also catches mismatched Caddy handle nesting.
docker run --rm --network none --entrypoint caddy -v "$beon_monitor_root/configs/caddy/beon-production.caddyfile:/etc/caddy/Caddyfile:ro" caddy:2-alpine adapt --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
