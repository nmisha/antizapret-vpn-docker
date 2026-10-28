#!/usr/bin/env bash
set -e

# Count failures ourselves: Compose's retries/start_period do not delay a kill.
started=$(cat /dev/shm/.coredns_started)
now=$(date +%s)
failed() {
    if [ "$((now - started))" -lt 90 ]; then
        echo 'CoreDNS is starting'
        exit 1
    fi
    failures=$(cat /dev/shm/.coredns_failures 2>/dev/null || echo 0)
    failures=$((failures + 1))
    echo "$failures" > /dev/shm/.coredns_failures
    echo "CoreDNS healthcheck failed ($failures/3)"
    if [ "$failures" -ge 3 ]; then
        kill -TERM 1
    fi
    exit 1
}

[ -s /Corefile ] || failed
# Discovery/configuration maintenance must not suppress the liveness probe.
timeout 15s /root/config.sh || echo 'CoreDNS configuration update deferred' >&2
answer=$(dig @127.0.0.1 healthcheck.invalid A +short +time=2 +tries=1) || failed
[ "$answer" = 127.0.0.1 ] || failed
rm -f /dev/shm/.coredns_failures
