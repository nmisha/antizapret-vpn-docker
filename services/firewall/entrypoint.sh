#!/usr/bin/env bash
set -eu
cd /root
sleep_pid=
stop() {
    trap - TERM INT HUP
    if [ -n "$sleep_pid" ]; then kill "$sleep_pid" 2>/dev/null || true; fi
    ./block.sh clear
    exit 0
}
trap stop TERM INT HUP
while true; do
    delay=30
    if ./download.sh "$V4_URL" "$V4_FILE" && ./download.sh "$V6_URL" "$V6_FILE"; then
        if ./block.sh apply; then delay="${INTERVAL:-3h}"; fi
    else
        echo "Download failed; retaining active firewall rules" >&2
        # Use cached files on startup if both are available and valid.
        if [ -f "$V4_FILE" ] && [ -f "$V6_FILE" ]; then ./block.sh apply || true; fi
    fi
    sleep "$delay" &
    sleep_pid=$!
    wait "$sleep_pid" || true
    sleep_pid=
done
