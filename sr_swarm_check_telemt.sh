#!/bin/sh
# Run on the Swarm manager hosting https. No configuration changes or restarts.
set -eu
stack=${STACK_NAME:-antizapret}
attempts=${TELEMT_CHECK_ATTEMPTS:-12}
case "$attempts" in ''|*[!0-9]*|0) echo 'TELEMT_CHECK_ATTEMPTS must be a positive integer' >&2; exit 2;; esac

# An absent optional service is different from an unreachable Docker daemon.
services=$(docker service ls --format '{{.Name}}')
if ! printf '%s\n' "$services" | grep -qx "${stack}_telemt"; then
    echo 'telemt is not deployed; skipping overlay check'
    exit 0
fi

i=1
while [ "$i" -le "$attempts" ]; do
    ready=true
    for service in https telemt; do
        state=$(docker service inspect "${stack}_$service" --format '{{if .UpdateStatus}}{{.UpdateStatus.State}}{{end}}')
        case "$state" in ''|completed) ;; *) ready=false;; esac
    done
    containers=$(docker ps -q --filter "label=com.docker.swarm.service.name=${stack}_https" --filter status=running)
    if [ "$ready" = true ] && [ -n "$containers" ]; then
        ok=true
        for cid in $containers; do
            # Resolve through Caddy's Docker DNS and connect from its own network
            # namespace. A healthy telemt alone cannot detect a missing VXLAN FDB.
            if ! docker exec "$cid" timeout 5 nc -z -w 3 telemt.antizapret 8443; then
                ok=false
            fi
        done
        if [ "$ok" = true ]; then
            echo 'OK: https -> telemt.antizapret:8443 over the container network'
            exit 0
        fi
    fi
    echo "Waiting for https -> telemt connectivity ($i/$attempts)..." >&2
    [ "$i" -eq "$attempts" ] || sleep 5
    i=$((i + 1))
done

echo 'ERROR: telemt overlay check failed. Run on the manager hosting https; check DNS, VXLAN/FDB and firewall. No services were restarted.' >&2
docker service ps --no-trunc "${stack}_https" "${stack}_telemt" >&2 || true
for cid in $containers; do
    docker exec "$cid" nslookup telemt.antizapret 127.0.0.11 >&2 || true
done
exit 1
