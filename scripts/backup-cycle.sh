#!/usr/bin/env bash
set -euo pipefail
beon_cycle_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
beon_cycle_state="${OPS_STATE_DIR:-/var/lib/beon-ipquality}"
install -d -m 0700 "$beon_cycle_state"
exec 9>"$beon_cycle_state/backup-cycle.lock"
flock -n 9
"$beon_cycle_root/scripts/backup.sh"
exec /usr/bin/python3 "$beon_cycle_root/scripts/operations.py" finalize-latest
