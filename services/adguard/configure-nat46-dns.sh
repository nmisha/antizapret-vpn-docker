#!/usr/bin/env bash
# Preserve IPv4-only client DNS while allowing exit-internal AAAA lookups.
set -euo pipefail
config=${1:-/opt/adguardhome/conf/AdGuardHome.yaml}
NAT46_ENABLED=${NAT46_ENABLED:-0}
case "$NAT46_ENABLED" in 0|1) ;; *) echo 'NAT46_ENABLED must be 0 or 1' >&2; exit 1;; esac
export NAT46_AAAA_RULE='||*^$dnstype=AAAA,dnsrewrite=NOERROR;;,client=~az-local|~az-world|~az-resolver,important'
yq -i '.user_rules = ((.user_rules // []) | map(select(. != strenv(NAT46_AAAA_RULE))))' "$config"
if [ "$NAT46_ENABLED" = 1 ]; then
    yq -i '.dns.aaaa_disabled = false | .user_rules += [strenv(NAT46_AAAA_RULE)]' "$config"
else
    yq -i '.dns.aaaa_disabled = true' "$config"
fi
