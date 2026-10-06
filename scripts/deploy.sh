#!/usr/bin/env bash
# Usage: sudo scripts/deploy.sh dist/mangasync /absolute/path/mangasync [service] [health-url]
set -euo pipefail
if (( $# < 2 )); then echo "Usage: $0 BINARY ABSOLUTE_TARGET [SERVICE] [HEALTH_URL]" >&2; exit 2; fi
binary=$1
target=$2
service=${3:-mangasync}
health=${4:-http://127.0.0.1:8787/api/health}
[[ "$target" = /* && -f "$binary" ]] || { echo 'Need an existing binary and an absolute target path' >&2; exit 2; }
backup="${target}.bak-$(date +%Y%m%d%H%M%S)"
had_old=false
if [[ -f "$target" ]]; then cp -p -- "$target" "$backup"; had_old=true; fi
install -m 0755 -- "$binary" "${target}.new"
mv -- "${target}.new" "$target"
healthy=false
if systemctl restart "$service"; then
  for attempt in {1..15}; do
    if curl -fsS --max-time 3 "$health" >/dev/null; then healthy=true; break; fi
    sleep 1
  done
fi
if "$healthy"; then echo "Deployment healthy. Backup: $backup"; exit 0; fi
if "$had_old"; then
  cp -p -- "$backup" "${target}.rollback"
  mv -- "${target}.rollback" "$target"
  systemctl restart "$service"
  echo "Deployment failed; restored $backup. Check service logs." >&2
else
  systemctl stop "$service" || true
  echo 'Deployment failed; service stopped (no previous binary).' >&2
fi
exit 1
