#!/bin/sh
set -eu
. /certificates.sh
state_dir="${DOCKTINEL_RUNTIME_DIR:-/run/docktinel}"
mkdir -p "$state_dir"
pending="$state_dir/certificate-reload-pending"
# Mark pending before publishing: an interrupted copy or failed reload must retry.
if [ -f "$pending.in-progress" ]; then touch "$pending"; fi
touch "$pending.in-progress"
sync_all_certificates
if [ "$certificates_changed" -eq 1 ] || [ -f "$pending" ]; then
    mv -f "$pending.in-progress" "$pending"
else
    rm -f "$pending.in-progress"
fi
if [ "${1:-}" != --initial ] && [ -f "$pending" ]; then
    caddy reload --force --config "$CADDYFILE" --adapter caddyfile
    rm -f "$pending"
fi
