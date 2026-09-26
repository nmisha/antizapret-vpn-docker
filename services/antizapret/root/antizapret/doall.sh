#!/bin/bash -e

HERE="$(dirname "$(readlink -f "${0}")")"
cd "$HERE"

if [ -s /etc/default/antizapret ]; then
    set -a
    source /etc/default/antizapret
    set +a
fi

function reload_dnsmap () {
    pkill -HUP -f '[d]nsmap' 2>/dev/null || true
}

source "$HERE/list-cache.sh"

LOCAL_OWNER_FILE="/tmp/.doall_owner"
RESULT_OWNER_FILE="/root/antizapret/result/.doall_owner"
LOCAL_OWNER="$(cat "$LOCAL_OWNER_FILE" 2>/dev/null || true)"
RESULT_OWNER="$(cat "$RESULT_OWNER_FILE" 2>/dev/null || true)"

if [ -n "$DOALL_DISABLED" ] || { [ -n "$RESULT_OWNER" ] && [ "$LOCAL_OWNER" != "$RESULT_OWNER" ]; }; then
    echo "DoAll disabled or owner mismatch. Reloading dnsmap only..."
    reload_dnsmap
    exit 0
fi

# Lock the open file, not its existence. Never unlink it: waiters must all
# refer to the same inode. Children inherit fd 9 so an interrupted parent
# cannot let another refresh start while its download/parse child still runs.
exec 9>/tmp/.doall_lock
flock -x 9

download_failed=false
echo "run download.sh" && ./download.sh || download_failed=true
if [ "$download_failed" = true ] && ! cached_downloads_available; then
  echo 'Error: Cant download some lists and no cached downloads are available'
  exit 1
fi
echo "run parse.sh" && ./parse.sh || exit 2
for kind in ips ips-world asn asn-world; do
    if list_cache_available "config/include-$kind-dist.txt"; then
        touch "result/.$kind.txt.ready"
    fi
done

# dnsmap applies the new ASN list on the next lookup without a restart.
reload_dnsmap

echo "Rules updated"
if [ "$download_failed" = true ]; then
  echo 'Warning: Cant download some lists'
fi
exit 0
