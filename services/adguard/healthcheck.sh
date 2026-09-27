#!/usr/bin/env bash
set -e

INIT_FILE="/dev/shm/.inited"
[ ! -f "$INIT_FILE" ] && exit 0;

ADGUARDHOME_USERNAME=${ADGUARDHOME_USERNAME:-"admin"}
ADGUARDHOME_PORT=${ADGUARDHOME_PORT:-"3000"}

# curl -u builds the header itself: busybox base64 wraps long credentials.
# Without a plain password (only a hash configured) the API is probed
# unauthenticated: a 401 still proves liveness, maintenance is skipped.
AUTH_ARGS=()
if [ -n "${ADGUARDHOME_PASSWORD:-}" ]; then
    AUTH_ARGS=(-u "$ADGUARDHOME_USERNAME:$ADGUARDHOME_PASSWORD")
fi

# resolve domain address to ip address
function resolve () {
    # $1 domain/ip address, $2 fallback ip address
    res="$(timeout 3s getent hosts "$1" | head -n1 | awk '{print $1}')"
    if [[ "$res" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        echo "$res"
    else
        echo "$2"
    fi
}

# Track applied metadata independently. Keep legacy startup state as a fallback
# during upgrades; an unavailable exit must not suppress the other exit's work.
OLD_LOCAL=$(cat /dev/shm/.config_md5.local 2>/dev/null || awk '{print $1}' /dev/shm/.config_md5 2>/dev/null || true)
OLD_WORLD=$(cat /dev/shm/.config_md5.world 2>/dev/null || awk '{print $2}' /dev/shm/.config_md5 2>/dev/null || true)
CONFIG_LOCAL=$(curl --connect-timeout 2 --max-time 3 -fsS "http://az-local.antizapret/config-md5/" || echo "")
CONFIG_WORLD=''
NEW_WORLD=''
LOCAL_CHANGED=0
WORLD_CHANGED=0
if [ -z "$CONFIG_LOCAL" ]; then
    touch /dev/shm/.config_md5.local_pending
elif [ "$CONFIG_LOCAL" != "$OLD_LOCAL" ] || [ -f /dev/shm/.config_md5.local_pending ]; then
    LOCAL_CHANGED=1
fi
if [ "$AZ_WORLD_ENABLED" = "1" ]; then
    CONFIG_WORLD=$(curl --connect-timeout 2 --max-time 3 -fsS "http://az-world.antizapret/config-md5/" || echo "")
    NEW_WORLD=$(resolve 'az-world' '')
    if [ -z "$CONFIG_WORLD" ]; then
        touch /dev/shm/.config_md5.world_pending
    elif [ "$CONFIG_WORLD" != "$OLD_WORLD" ] || [ -f /dev/shm/.config_md5.world_pending ]; then
        WORLD_CHANGED=1
    fi
fi

# Unlike exit metadata, the local API must respond for this check to succeed.
# A transport error fails liveness; any HTTP answer means AdGuard is alive.
RESPONSE=$(curl --connect-timeout 2 --max-time 5 -sS -w '\n%{http_code}' "${AUTH_ARGS[@]}" \
    "http://127.0.0.1:$ADGUARDHOME_PORT/control/clients")
HTTP_CODE=${RESPONSE##*$'\n'}
CLIENTS=${RESPONSE%$'\n'*}
if [ "$HTTP_CODE" = 401 ] || [ "$HTTP_CODE" = 403 ]; then
    echo "AdGuard API rejected credentials (HTTP $HTTP_CODE): set ADGUARDHOME_PASSWORD to the current password; skipping filter and client maintenance" >&2
    exit 0
fi
if [ "$HTTP_CODE" != 200 ]; then
    echo "AdGuard API not ready (HTTP $HTTP_CODE)"
    exit 0
fi

# Updating remote filters is maintenance, not a liveness requirement.
# Partial failure retains the checksum and must not restart working DNS.
refresh_filters() {
    echo "Config files changed"

    # The API refreshes a whole category, not individual exit URLs. Failed
    # downloads retain their cached filters in AdGuard. Do not disable them.
    # Run categories sequentially: concurrent refreshes may be rejected as busy.
    local refresh_failed=0 whitelist
    for whitelist in false true; do
        curl --connect-timeout 2 --max-time 30 -fsS "http://127.0.0.1:$ADGUARDHOME_PORT/control/filtering/refresh" \
            -X POST -H 'Content-Type: application/json' "${AUTH_ARGS[@]}" \
            --data-raw "{\"whitelist\":$whitelist}" || refresh_failed=1
    done
    [ "$refresh_failed" = 0 ] || return 1

    curl --connect-timeout 2 --max-time 5 -fsS "http://127.0.0.1:$ADGUARDHOME_PORT/control/cache_clear" -X 'POST' "${AUTH_ARGS[@]}" || return 1
    # Only acknowledge reachable exits. Pending flags also force a refresh on
    # recovery with an unchanged checksum (e.g. filters missing at cold start).
    if [ "$LOCAL_CHANGED" = 1 ]; then
        printf '%s\n' "$CONFIG_LOCAL" > /dev/shm/.config_md5.local || return 1
        rm -f /dev/shm/.config_md5.local_pending
    fi
    if [ "$WORLD_CHANGED" = 1 ]; then
        printf '%s\n' "$CONFIG_WORLD" > /dev/shm/.config_md5.world || return 1
        rm -f /dev/shm/.config_md5.world_pending
    fi
}
if [ "$LOCAL_CHANGED" = 1 ] || [ "$WORLD_CHANGED" = 1 ]; then
    refresh_filters || echo "Filter refresh deferred; keeping DNS running" >&2
fi

update_client() {
    client_name=$1
    new_ip=$2
    client_id=${3:-}
    client_updated=0

    [ -z "$new_ip" ] && return 0

    FULL_CLIENT=$(echo "$CLIENTS" | jq --arg name "$client_name" '.clients[] | select(.name==$name)' || echo "error response: $CLIENTS")
    if [ -n "$FULL_CLIENT" ] && [ "$FULL_CLIENT" != "null" ]; then
        CURRENT_IDS=$(echo "$FULL_CLIENT" | jq -c '.ids // []')
        DESIRED_IDS=$(jq -nc --arg id "$client_id" --arg ip "$new_ip" '[$id, $ip] | map(select(. != ""))')
        CURRENT_IDS_NORMALIZED=$(echo "$CURRENT_IDS" | jq -c 'sort | unique')
        DESIRED_IDS_NORMALIZED=$(echo "$DESIRED_IDS" | jq -c 'sort | unique')

        if [ "$CURRENT_IDS_NORMALIZED" != "$DESIRED_IDS_NORMALIZED" ]; then
            UPDATED_CLIENT=$(echo "$FULL_CLIENT" | jq --argjson ids "$DESIRED_IDS" '.ids = $ids')
            UPDATE_BODY=$(printf '{"name":"%s","data":%s}' "$client_name" "$(echo "$UPDATED_CLIENT" | jq -c .)")
            echo "Updating $client_name ids to $DESIRED_IDS"
            curl --connect-timeout 2 --max-time 5 -fsS -X POST "http://127.0.0.1:$ADGUARDHOME_PORT/control/clients/update" -H 'Content-Type: application/json' "${AUTH_ARGS[@]}" --data "$UPDATE_BODY"
            client_updated=1
        fi
    fi

    if [ "$client_updated" = "1" ]; then
        echo "Reset adguard DNS cache"
        curl --connect-timeout 2 --max-time 5 -fsS "http://127.0.0.1:$ADGUARDHOME_PORT/control/cache_clear" -X 'POST' "${AUTH_ARGS[@]}"
    fi
}

NEW_LOCAL=$(resolve 'az-local' '')
NEW_COREDNS=$(resolve 'coredns' '')

update_client "az-local" "$NEW_LOCAL" "az-local"
update_client "az-world" "$NEW_WORLD" "az-world"
update_client "coredns" "$NEW_COREDNS"
