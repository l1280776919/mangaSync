#!/usr/bin/env bash
# Usage: sudo scripts/deploy.sh BINARY ABSOLUTE_TARGET SERVICE HEALTH_URL DATA_DIR
set -euo pipefail
if (( $# < 5 )); then echo "Usage: $0 BINARY ABSOLUTE_TARGET SERVICE HEALTH_URL ABSOLUTE_DATA_DIR" >&2; exit 2; fi
binary=$(realpath "$1")
target=$2
service=$3
health=$4
data_dir=$5
[[ "$target" = /* && "$data_dir" = /* && -f "$binary" && -f "$data_dir/mangasync.db" ]] || { echo 'Need existing binary/database and absolute target/data paths' >&2; exit 2; }
stamp=$(date +%Y%m%d%H%M%S)
old_binary="${target}.bak-${stamp}"
snapshot="${data_dir}.backup-${stamp}"
restore_dir="${data_dir}.restore-${stamp}"
failed_dir="${data_dir}.failed-${stamp}"
[[ -f "$target" ]] || { echo 'This upgrade command requires an existing installation.' >&2; exit 2; }
cp -p -- "$target" "$old_binary"
# Stop writes before the snapshot so an automatic rollback loses no accepted operations.
systemctl stop "$service"
if ! "$binary" -data "$data_dir" -backup-to "$snapshot"; then systemctl start "$service"; exit 1; fi
key_args=()
if [[ -f "$data_dir/secrets.key" ]]; then
  # Separate from the DB backup; use a different protected destination for off-device backups.
  if ! install -m 0600 -- "$data_dir/secrets.key" "${snapshot}.key"; then systemctl start "$service"; exit 1; fi
  key_args=(-key "${snapshot}.key")
fi
healthy=false
if install -m 0755 -- "$binary" "${target}.new" && mv -- "${target}.new" "$target" && systemctl start "$service"; then
  for attempt in {1..15}; do
    if curl -fsS --max-time 3 "$health" >/dev/null; then healthy=true; break; fi
    sleep 1
  done
fi
if "$healthy"; then echo "Deployment healthy. Data snapshot: $snapshot; prior binary: $old_binary"; exit 0; fi
systemctl stop "$service" || true
# Restore using the new maintenance implementation, before putting back the prior binary.
"$binary" -restore-backup "$snapshot" -restore-to "$restore_dir" "${key_args[@]}"
mkdir -m 0700 -- "$failed_dir"
for name in mangasync.db mangasync.db-wal mangasync.db-shm secrets.key config.json; do
  if [[ -f "$data_dir/$name" ]]; then mv -- "$data_dir/$name" "$failed_dir/$name"; fi
  if [[ -f "$restore_dir/$name" ]]; then mv -- "$restore_dir/$name" "$data_dir/$name"; fi
done
cp -p -- "$old_binary" "${target}.rollback"
mv -- "${target}.rollback" "$target"
systemctl start "$service"
echo "Deployment failed; prior database/config/key and binary restored. Failed data preserved: $failed_dir" >&2
exit 1
