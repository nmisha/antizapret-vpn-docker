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
    # Sanitize the exclude list first: an empty line is a pattern that matches
    # every input line, so `grep -v` would silently publish an empty list.
    exclude="config/custom/exclude-$kind-custom.txt"
    if [[ "$kind" == asn* ]]; then
        sanitize_options=(-v whole_line=1)
        sort_options=(-f)
        uniq_options=(-i)
        awk "${sanitize_options[@]}" -f scripts/sanitize-lists.awk "$exclude" > "$STAGING/exclude-exact"
        : > "$STAGING/exclude-regex"
        exact_options=(-v -i -F -x)
    else
        sanitize_options=()
        sort_options=()
        uniq_options=()
        awk -f scripts/sanitize-lists.awk "$exclude" > "$STAGING/exclude"
        # Plain addresses/CIDRs are compared exactly (10.0.0.0/8 must not also
        # exclude 110.0.0.0/8); anything else stays an ERE for compatibility.
        awk -v mode=exact -f scripts/validate-ipv4.awk "$STAGING/exclude" > "$STAGING/exclude-exact"
        awk -v mode=regex -f scripts/validate-ipv4.awk "$STAGING/exclude" > "$STAGING/exclude-regex"
        exact_options=(-v -F -x)
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
    awk "${sanitize_options[@]}" -f scripts/sanitize-lists.awk "$STAGING/input" > "$STAGING/sanitized"
    if [[ "$kind" != asn* ]]; then
        # Skip IPv6/typos with a warning instead of failing every list.
        awk -v list="$kind" -f scripts/validate-ipv4.awk "$STAGING/sanitized" > "$STAGING/valid"
        mv -f "$STAGING/valid" "$STAGING/sanitized"
    fi
    filter_exclusions "${exact_options[@]}" -f "$STAGING/exclude-exact" < "$STAGING/sanitized" |
        filter_exclusions -v -E -f "$STAGING/exclude-regex" |
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
    C_NET="${line%%/*}"
    # Ignore sipcalc's separate "Network mask (bits)" and "(hex)" rows.
    C_NETMASK="$(sipcalc -- "$line" | awk '$1 == "Network" && $2 == "mask" && $3 == "-" {print $4}')"
    [[ "$C_NETMASK" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || {
        echo "Invalid route mask: $line" >&2
        exit 1
    }
    printf 'push "route %s %s"\n' "$C_NET" "$C_NETMASK" >> "$STAGING/openvpn-blocked-ranges.txt"
done < "$STAGING/routes"
set -x


# Only invalidate markers once every output has been generated successfully.
# Publishing several files is not a single transaction; keep this phase short.
rm -f result/.{ips,ips-world,asn,asn-world}.txt.ready
mv -f "$STAGING/ips.txt" "$STAGING/ips-world.txt" \
    "$STAGING/asn.txt" "$STAGING/asn-world.txt" \
    "$STAGING/openvpn-blocked-ranges.txt" result/

exit 0
