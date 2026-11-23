#!/bin/bash
set -euo pipefail
cd /root
cache=/var/lib/antizapret-firewall
mkdir -p "$cache"
exec 8>"$cache/lock"
flock 8
staged=$(mktemp -d "$cache/.refresh.XXXXXX")
trap 'rm -rf -- "$staged"' EXIT

apply_family() {
    local family="$1" candidate="$2"
    if [[ "$family" == 4 ]]; then
        FIREWALL_FAMILY=4 V4_FILE="$candidate" ./block.sh
    else
        FIREWALL_FAMILY=6 V6_FILE="$candidate" ./block.sh
    fi
}

refresh_family() {
    local family="$1" source_url="$2" local_file="$3" restore="$4"
    local saved="$cache/v$family" candidate="$staged/v$family"
    if [[ "$restore" == --restore ]]; then
        if [[ -s "$saved" ]] && apply_family "$family" "$saved"; then
            return 0
        fi
        if [[ -s "$local_file" ]]; then
            cp -- "$local_file" "$candidate" || return 1
            if apply_family "$family" "$candidate"; then
                mv -f -- "$candidate" "$saved" || return 1
                return 0
            fi
        fi
    fi
    ./download.sh "$source_url" "$candidate" || return 1
    apply_family "$family" "$candidate" || return 1
    # Cache only lists successfully applied to this family; keep the old cache on error.
    mv -f -- "$candidate" "$saved" || return 1
}

status=0
refresh_family 4 "$V4_URL" "$V4_FILE" "${1:-}" || status=1
refresh_family 6 "$V6_URL" "$V6_FILE" "${1:-}" || status=1
exit "$status"
