#!/usr/bin/env bash
set -ex

sleep_pid=

cleanup() {
    trap - HUP INT QUIT PIPE TERM
    if [ -n "$sleep_pid" ]; then
        kill "$sleep_pid" 2>/dev/null || true
    fi
    ./block.sh clear
    exit 0
}

trap cleanup HUP INT QUIT PIPE TERM

while true; do
    delay=30
    status=0
    ./download.sh "$V4_URL" "$V4_FILE" || status=1
    ./download.sh "$V6_URL" "$V6_FILE" || status=1
    # Apply each available family even when the other download failed.
    ./block.sh || status=1
    if [ "$status" = 0 ]; then
        delay="$INTERVAL"
    else
        echo 'Firewall update incomplete; keeping cached sets and retrying in 30s'
    fi

    sleep "$delay" &
    sleep_pid=$!
    wait "$sleep_pid" || true
    sleep_pid=
done
