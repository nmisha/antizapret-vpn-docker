#!/bin/bash
set -exo pipefail

HERE="$(dirname "$(readlink -f "${0}")")"
cd "$HERE"

function download_list() {
    local urls="$1"
    local output_file="$2"

    if [ -z "$urls" ]; then
        return 0
    fi

    echo "Downloading URLs to $output_file"
    local tmp_file="${output_file}.tmp"
    rm -f "$tmp_file"

    local success=true
    for url in ${urls//;/ }; do
        url=$(echo "$url" | tr -d '[:space:]')
        if [ -z "$url" ]; then continue; fi
        if ! curl --connect-timeout 5 --max-time 10 -L -f -s "$url" >> "$tmp_file"; then
            echo "Failed to download $url"
            success=false
            break
        fi
        printf '\n' >> "$tmp_file"
    done

    if [ "$success" = true ] && [ -s "$tmp_file" ]; then
        # Normalize only a successfully downloaded list, before publishing it.
        # Never rewrite another list's cached data after its download failed.
        if ! sed -E '/^(#.*)?[[:space:]]*$/d' "$tmp_file" | sort | uniq > "${tmp_file}.clean"; then
            rm -f "$tmp_file" "${tmp_file}.clean"
            return 1
        fi
        rm -f "$tmp_file"
        mv -f "${tmp_file}.clean" "$output_file" || return 1
        touch "$(dirname "$output_file")/.$(basename "$output_file").ready" || return 1
    else
        echo "Failed to download some URLs or resulting file is empty, keeping old file"
        rm -f "$tmp_file"
        if [ ! -f "$output_file" ]; then
          echo "File not found. Creating empty: $output_file"
          touch "$output_file"
        fi
        return 1
    fi
}

# Attempt every independent list even if an earlier source is unavailable.
# doall still receives a failure and decides whether the remaining cache is
# sufficient for generation; missing required data must not be called success.
status=0
download_list "$IPS_URL" "config/include-ips-dist.txt" || status=1
download_list "$IPS_WORLD_URL" "config/include-ips-world-dist.txt" || status=1
download_list "$ASN_URL" "config/include-asn-dist.txt" || status=1
download_list "$ASN_WORLD_URL" "config/include-asn-world-dist.txt" || status=1

exit "$status"
