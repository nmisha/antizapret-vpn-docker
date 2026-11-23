#!/bin/bash
set -euo pipefail
for family in 4 6; do
    command=iptables
    [[ "$family" == 4 ]] || command=ip6tables
    setname="az_firewall_v$family"
    ipset list -terse "$setname" | awk '/Number of entries:/ {present=($4>0)} END {exit !present}'
    "$command" -w 1 -C DOCKER-USER -m comment --comment antizapret-firewall -j AZ-FIREWALL
    "$command" -w 1 -C AZ-FIREWALL -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
    "$command" -w 1 -C AZ-FIREWALL -m set --match-set "$setname" src -j DROP
done
