#!/bin/bash
# Exit immediately if a command exits with a non-zero status
set -e
set -x

DOCKER_SUBNET="$(ipcalc "$(ip -4 addr show dev eth0 | awk '$1=="inet" {print $2; exit}')" | awk '/Network:/ {print $2}')"

cat << EOF | sponge /etc/environment
OPENVPN_LOCAL_IP_RANGE='${OPENVPN_LOCAL_IP_RANGE:-"10.1.165.0"}'
OPENVPN_DNS='${OPENVPN_DNS:-"14.16.0.1"}'
AZ_SUBNET=${AZ_SUBNET:-"14.16.0.0/14"}
DOCKER_SUBNET=${DOCKER_SUBNET}
NIC='$(ip -4 route ls | grep default | grep -Po '(?<=dev )(\S+)' | head -1)'
OVDIR='${OVDIR:-"/etc/openvpn"}'
EOF
source /etc/environment
ln -sf /etc/environment /etc/profile.d/environment.sh

OPENVPN_ALLOWED_IPS="${OPENVPN_LOCAL_IP_RANGE}/24,${AZ_SUBNET},${DOCKER_SUBNET}"
if [ -f /opt/antizapret/result/openvpn-blocked-ranges.txt ]; then
    blocked_ranges=$(grep -v '^[[:space:]]*$' /opt/antizapret/result/openvpn-blocked-ranges.txt | tr '\n' ',' | sed 's/,$//')
    if [ -n "${blocked_ranges}" ]; then
        OPENVPN_ALLOWED_IPS="${OPENVPN_ALLOWED_IPS},${blocked_ranges}"
    fi
fi

build_forward_allow_rules() {
    local cidr
    while IFS= read -r cidr; do
        [ -z "$cidr" ] && continue
        printf 'iptables -A ovpn_allowed_destinations -d %s -j ACCEPT;\n' "$cidr"
    done < <(echo "$OPENVPN_ALLOWED_IPS" | tr ',' '\n' | awk '{gsub(/^[[:space:]]+|[[:space:]]+$/, ""); if (length) print}')
}

FORWARD_ALLOW_RULES="$(build_forward_allow_rules)"

iptables -t nat -N masq_not_local;
iptables -t nat -A POSTROUTING -s ${OPENVPN_LOCAL_IP_RANGE}/24 -j masq_not_local;
iptables -t nat -A masq_not_local -d ${DOCKER_SUBNET} -p tcp --dport 53 -j RETURN;
iptables -t nat -A masq_not_local -d ${DOCKER_SUBNET} -p udp --dport 53 -j RETURN;
iptables -t nat -A masq_not_local -d ${DOCKER_SUBNET} -j MASQUERADE;
iptables -t nat -A masq_not_local -d ${AZ_SUBNET} -j RETURN;
iptables -t nat -A masq_not_local -j MASQUERADE;
iptables -N ovpn_allowed_destinations;
iptables -I FORWARD 1 -s ${OPENVPN_LOCAL_IP_RANGE}/24 -j ovpn_allowed_destinations;
iptables -A ovpn_allowed_destinations -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT;
${FORWARD_ALLOW_RULES}iptables -A ovpn_allowed_destinations -j DROP;

./routes.sh --vpn --dns-file /opt/antizapret/result/dns.txt &
