#!/bin/bash
# Empty files are valid only after a successful download or generation.
# Nonempty files remain usable when upgrading installations without markers.
list_cache_available() {
    local file="$1"
    [ -f "$file" ] && { [ -s "$file" ] || [ -f "$(dirname "$file")/.$(basename "$file").ready" ]; }
}

required_lists_available() {
    local directory="$1" prefix="$2"
    [ -z "${IPS_URL:-}" ] || list_cache_available "$directory/${prefix}ips.txt" || return 1
    [ -z "${IPS_WORLD_URL:-}" ] || list_cache_available "$directory/${prefix}ips-world.txt" || return 1
    [ -z "${ASN_URL:-}" ] || list_cache_available "$directory/${prefix}asn.txt" || return 1
    [ -z "${ASN_WORLD_URL:-}" ] || list_cache_available "$directory/${prefix}asn-world.txt" || return 1
}

cached_downloads_available() {
    [ -z "${IPS_URL:-}" ] || list_cache_available config/include-ips-dist.txt || return 1
    [ -z "${IPS_WORLD_URL:-}" ] || list_cache_available config/include-ips-world-dist.txt || return 1
    [ -z "${ASN_URL:-}" ] || list_cache_available config/include-asn-dist.txt || return 1
    [ -z "${ASN_WORLD_URL:-}" ] || list_cache_available config/include-asn-world-dist.txt || return 1
}
