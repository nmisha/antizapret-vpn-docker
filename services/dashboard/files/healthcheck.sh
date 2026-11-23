#!/bin/sh
set -eu
case "${DASHBOARD_AUTH_ENABLED:-1}" in
    1) set -- --user "${DASHBOARD_USERNAME:-}:${DASHBOARD_PASSWORD:-}" ;;
    0) set -- ;;
    *) exit 1 ;;
esac
exec curl -fsSI --connect-timeout 2 --max-time 3 "$@" \
    http://127.0.0.1/config.json >/dev/null
