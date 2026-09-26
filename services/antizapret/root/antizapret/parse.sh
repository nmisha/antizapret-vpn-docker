#!/bin/bash
set -exo pipefail

HERE="$(dirname "$(readlink -f "${0}")")"
cd "$HERE"
export LC_ALL=C.UTF-8

# Build all outputs privately. A bad exclude expression or a failed reader
# must leave the previously published lists and their readiness markers intact.
mkdir -p temp
STAGING=$(mktemp -d temp/parse.XXXXXXXX)
trap 'rm -rf "$STAGING"' EXIT

filter_exclusions() {
    local status=0
    grep "$@" || status=$?
    # grep 1 means a valid empty result; 2+ means an error, not an empty list.
    [ "$status" -le 1 ] || return "$status"
}

for kind in ips ips-world asn asn-world; do
    if [[ "$kind" == asn* ]]; then
        options=(-v -i -F -x)
        sort_options=(-f)
        uniq_options=(-i)
    else
        options=(-v -E)
        sort_options=()
        uniq_options=()
    fi
    # Write inputs separately so a failed cat cannot be masked by echo.
    cat "config/custom/include-$kind-custom.txt" > "$STAGING/input"
    echo >> "$STAGING/input"
    case "$kind" in
        ips) source_url=${IPS_URL:-} ;;
        ips-world) source_url=${IPS_WORLD_URL:-} ;;
        asn) source_url=${ASN_URL:-} ;;
        asn-world) source_url=${ASN_WORLD_URL:-} ;;
    esac
    # A disabled source has no downloaded file on a fresh installation.
    # Configured sources and existing files must still be readable.
    if [ -n "$source_url" ] || [ -e "config/include-$kind-dist.txt" ]; then
        cat "config/include-$kind-dist.txt" >> "$STAGING/input"
    fi
    awk -f scripts/sanitize-lists.awk "$STAGING/input" |
        filter_exclusions "${options[@]}" -f "config/custom/exclude-$kind-custom.txt" |
        sort "${sort_options[@]}" | uniq "${uniq_options[@]}" > "$STAGING/$kind.txt"
done

# Generate OpenVPN route file
echo -n > "$STAGING/openvpn-blocked-ranges.txt"
cat "$STAGING/ips.txt" "$STAGING/ips-world.txt" > "$STAGING/routes"
printf '%s\n' "${DOCKER_SUBNET:-}" >> "$STAGING/routes"
set +x
while read -r line
do
    [ -z "$line" ] && continue
    C_NET="$(echo $line | awk -F '/' '{print $1}')"
    C_NETMASK="$(sipcalc -- "$line" | awk '/Network mask/ {print $4}')"
    [ -n "$C_NETMASK" ] || { echo "Invalid route: $line" >&2; exit 1; }
    echo $"push \"route ${C_NET} ${C_NETMASK}\"" >> "$STAGING/openvpn-blocked-ranges.txt"
done < "$STAGING/routes"
set -x


# Only invalidate markers once every output has been generated successfully.
# Publishing several files is not a single transaction; keep this phase short.
rm -f result/.{ips,ips-world,asn,asn-world}.txt.ready
mv -f "$STAGING/ips.txt" "$STAGING/ips-world.txt" \
    "$STAGING/asn.txt" "$STAGING/asn-world.txt" \
    "$STAGING/openvpn-blocked-ranges.txt" result/

exit 0
