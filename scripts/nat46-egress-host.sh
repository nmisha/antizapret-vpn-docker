#!/bin/bash
# Apply on an exit host, as root, after installing Jool. Never run during inspection.
set -euo pipefail
[ "${1:-}" = apply ] || { echo "Usage: $0 apply (settings from /etc/default/antizapret-nat46)" >&2; exit 2; }
[ "$(id -u)" = 0 ] || { echo 'Run as root' >&2; exit 1; }
source /etc/default/antizapret-nat46
: "${NAT46_SOURCE_PREFIX:?Set the /96 client source prefix}"
: "${NAT46_WAN_INTERFACE:?Set the real IPv6 uplink}"
NAT46_NAT66_ENABLED=${NAT46_NAT66_ENABLED:-1}
NAT46_NETWORK=${NAT46_NETWORK:-antizapret-nat46-egress}
NAT46_EGRESS_SUBNET=${NAT46_EGRESS_SUBNET:-fd46:ee::/64}
NAT46_EGRESS_GATEWAY=${NAT46_EGRESS_GATEWAY:-fd46:ee::1}
NAT46_EGRESS_ADDRESS=${NAT46_EGRESS_ADDRESS:-fd46:ee::2}
case "$NAT46_NAT66_ENABLED" in 0|1) ;; *) echo 'Invalid NAT66 flag' >&2; exit 1;; esac
export NAT46_ROUTED_PREFIX_CONFIRMED=${NAT46_ROUTED_PREFIX_CONFIRMED:-0}
export NAT46_SOURCE_PREFIX NAT46_EGRESS_SUBNET NAT46_EGRESS_GATEWAY NAT46_EGRESS_ADDRESS NAT46_NAT66_ENABLED
python3 - <<'PY'
import ipaddress, os
prefix=ipaddress.IPv6Network(os.environ['NAT46_SOURCE_PREFIX'])
network=ipaddress.IPv6Network(os.environ['NAT46_EGRESS_SUBNET'])
gateway=ipaddress.IPv6Address(os.environ['NAT46_EGRESS_GATEWAY'])
address=ipaddress.IPv6Address(os.environ['NAT46_EGRESS_ADDRESS'])
assert prefix.prefixlen == 96 and not prefix.overlaps(network)
assert gateway in network and address in network and gateway != address
if os.environ['NAT46_NAT66_ENABLED'] == '0':
    assert prefix.network_address.is_global, 'No-NAT66 requires a public routed /96'
    assert os.getenv('NAT46_ROUTED_PREFIX_CONFIRMED') == '1', 'Confirm provider routing first'
PY
[ "$(sysctl -n net.ipv6.conf.all.forwarding)" = 1 ] || {
    echo 'Enable persistent host IPv6 forwarding after checking RA/default-route renewal; see guide.' >&2
    exit 1
}
ip -6 route show default | grep -q "dev $NAT46_WAN_INTERFACE " || {
    echo 'Expected IPv6 default route on the configured WAN interface' >&2; exit 1;
}
modinfo jool_siit >/dev/null
# The operator loads modules at boot. This script does not install or reload them.
lsmod | grep '^jool_siit ' >/dev/null || { echo 'Load jool_siit first' >&2; exit 1; }
if ! ip link show az46-parent >/dev/null 2>&1; then
    ip link add az46-parent type dummy
fi
ip -d link show az46-parent | grep -q dummy || { echo 'az46-parent has another owner/type' >&2; exit 1; }
ip link set az46-parent up
if ! ip link show az46-shim >/dev/null 2>&1; then
    ip link add az46-shim link az46-parent type macvlan mode bridge
fi
ip -d link show az46-shim | grep -q 'macvlan.*mode bridge' || { echo 'Unexpected az46-shim type' >&2; exit 1; }
ip link set az46-shim up
ip -6 addr replace "$NAT46_EGRESS_GATEWAY/${NAT46_EGRESS_SUBNET#*/}" dev az46-shim nodad
ip -6 route replace "$NAT46_SOURCE_PREFIX" via "$NAT46_EGRESS_ADDRESS" dev az46-shim

# One exit per host. The IPv6 /128 allocation pool gives it a stable next hop.
config_name="${NAT46_NETWORK}-config"
NAT46_NETWORK_SIGNATURE="172.31.46.0/29|172.31.46.1|172.31.46.2/32|$NAT46_EGRESS_SUBNET|$NAT46_EGRESS_GATEWAY|$NAT46_EGRESS_ADDRESS/128"
export NAT46_NETWORK_SIGNATURE
if ! docker network inspect "$config_name" >/dev/null 2>&1; then
    docker network create --config-only --ipv6 \
        --label "org.antizapret.nat46.config=$NAT46_NETWORK_SIGNATURE" \
        --subnet 172.31.46.0/29 --gateway 172.31.46.1 --ip-range 172.31.46.2/32 \
        --subnet "$NAT46_EGRESS_SUBNET" --gateway "$NAT46_EGRESS_GATEWAY" \
        --ip-range "$NAT46_EGRESS_ADDRESS/128" -o parent=az46-parent "$config_name"
fi
# Existing configuration must exactly match; never silently reuse a conflicting network.
docker network inspect "$config_name" | python3 -c '
import json,os,sys
n=json.load(sys.stdin)[0]
assert n["ConfigOnly"] and n["EnableIPv6"]
assert n["Options"].get("parent")=="az46-parent"
expected={("172.31.46.0/29","172.31.46.1","172.31.46.2/32"),
(os.environ["NAT46_EGRESS_SUBNET"],os.environ["NAT46_EGRESS_GATEWAY"],os.environ["NAT46_EGRESS_ADDRESS"]+"/128")}
assert n["Labels"].get("org.antizapret.nat46.config")==os.environ["NAT46_NETWORK_SIGNATURE"]
# Some Engine/API combinations omit IPAM.Config for config-only networks.
# The immutable creation label still identifies the owned configuration.
if n["IPAM"]["Config"] is not None:
    assert {(p["Subnet"],p["Gateway"],p["IPRange"]) for p in n["IPAM"]["Config"]}==expected
'

# Dedicated owned chains; do not alter existing Docker/mail-stack NAT rules.
ip6tables -w -t nat -N AZ_NAT46 2>/dev/null || ip6tables -w -t nat -S AZ_NAT46 >/dev/null
ip6tables -w -t nat -F AZ_NAT46
if [ "$NAT46_NAT66_ENABLED" = 1 ]; then
    ip6tables -w -t nat -A AZ_NAT46 -s "$NAT46_SOURCE_PREFIX" -o "$NAT46_WAN_INTERFACE" -j MASQUERADE
fi
ip6tables -w -t nat -C POSTROUTING -j AZ_NAT46 2>/dev/null || ip6tables -w -t nat -A POSTROUTING -j AZ_NAT46
ip6tables -w -N AZ_NAT46_FWD 2>/dev/null || ip6tables -w -S AZ_NAT46_FWD >/dev/null
ip6tables -w -F AZ_NAT46_FWD
ip6tables -w -A AZ_NAT46_FWD -i az46-shim -o "$NAT46_WAN_INTERFACE" -s "$NAT46_SOURCE_PREFIX" -j ACCEPT
ip6tables -w -A AZ_NAT46_FWD -i "$NAT46_WAN_INTERFACE" -o az46-shim -d "$NAT46_SOURCE_PREFIX" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
# Docker creates DOCKER-USER when IPv6 filtering is active; otherwise hook FORWARD.
chain=FORWARD
if ip6tables -w -S DOCKER-USER >/dev/null 2>&1; then chain=DOCKER-USER; fi
ip6tables -w -C "$chain" -j AZ_NAT46_FWD 2>/dev/null || ip6tables -w -I "$chain" 1 -j AZ_NAT46_FWD

echo "Prepared local config-only network $config_name."
echo "On the Swarm manager, create the external network once:"
echo "docker network create --driver macvlan --scope swarm --config-from $config_name $NAT46_NETWORK"
