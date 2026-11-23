"""Checks run in disposable containers, without host networking or Docker socket."""
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[3]
IMAGE = os.environ.get("OPENVPN_TEST_IMAGE", "nmisha/antizapret-vpn-openvpn:6.9.0-2")


class OpenvpnTests(unittest.TestCase):
    def run_case(self, script, image=IMAGE):
        result = subprocess.run([
            "docker", "run", "--rm", "--network", "none", "--cap-add", "NET_ADMIN",
            "--entrypoint", "bash", "--mount",
            f"type=bind,source={ROOT / 'services/openvpn'},target=/source,readonly",
            "--mount", f"type=bind,source={ROOT / 'services/antizapret/common/docktinel'},target=/usr/local/lib/docktinel,readonly",
            image, "-ec", "set -euo pipefail\n" + script,
        ], capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_client_rules_use_route_snapshot_and_detect_nic(self):
        self.run_case(r"""
ip link add eth0 type dummy
ip addr add 10.200.0.2/24 dev eth0
ip link set eth0 up
ip route add default dev eth0
mkdir -p /etc/openvpn /opt/antizapret/result
printf 'push "route 192.0.2.0 255.255.255.0"\n' > /etc/openvpn/openvpn-blocked-ranges.txt
printf 'push "route 198.51.100.0 255.255.255.0"\n' > /opt/antizapret/result/openvpn-blocked-ranges.txt
export VPN_CLIENT_FIREWALL=true
bash /source/files/fw-rules.sh
. /etc/environment
test "$NIC" = eth0
iptables -C FORWARD -s 10.1.165.0/24 -j az_clients
iptables -C FORWARD -s 10.1.165.0/24 -p icmp --icmp-type echo-request -j DROP
iptables -C az_clients -d 192.0.2.0/24 -j ACCEPT
! iptables -C az_clients -d 198.51.100.0/24 -j ACCEPT
iptables -C az_clients -j REJECT --reject-with icmp-admin-prohibited
""")

    def test_health_detects_route_generation(self):
        self.run_case(r"""
mkdir -p /opt/antizapret/result
printf 'push "route 192.0.2.0 255.255.255.0"\n' > /opt/antizapret/result/openvpn-blocked-ranges.txt
md5sum /opt/antizapret/result/openvpn-blocked-ranges.txt | cut -d' ' -f1 > /dev/shm/.config_md5
touch /dev/shm/.inited
bash /source/files/healthcheck.sh
printf 'push "route 198.51.100.0 255.255.255.0"\n' >> /opt/antizapret/result/openvpn-blocked-ranges.txt
if bash /source/files/healthcheck.sh; then exit 1; fi
kill -0 1
""")

    def test_crl_renewal_requests_supervised_restart(self):
        self.run_case(r"""
export EASYRSA_BATCH=1 EASYRSA_ALGO=ec EASYRSA_PKI=/etc/openvpn/pki EASYRSA_CRL_DAYS=1
mkdir -p /etc/openvpn
/usr/share/easy-rsa/easyrsa init-pki
/usr/share/easy-rsa/easyrsa build-ca nopass
/usr/share/easy-rsa/easyrsa gen-crl
cp /source/files/update-crl.sh /opt/app/update-crl.sh
chmod +x /opt/app/update-crl.sh
mkdir -p /tmp/bin
cat > /tmp/bin/docktinel <<'EOF'
#!/bin/sh
printf '%s\n' "$*" > /tmp/restart-request
EOF
chmod +x /tmp/bin/docktinel
export PATH="/tmp/bin:$PATH"
bash /source/files/renew-crl.sh
test -f /dev/shm/.crl_renewed
grep -q 'restart --reason OpenVPN CRL renewed' /tmp/restart-request
openssl crl -in /etc/openvpn/pki/crl.pem -noout -nextupdate
""")

    def test_ui_preparation_is_idempotent(self):
        self.run_case(r"""
mkdir -p /opt/openvpn-ui/conf /etc/profile.d /run/docktinel /tmp/bin
cp /source/files-ui/init.db /opt/openvpn-ui/init.db
printf 'OpenVpnPath = /etc/openvpn\nhttpport = 8081\n' > /opt/openvpn-ui/conf/app.conf
# This dependency image lacks sponge; provide its file-replacement behavior.
cat > /tmp/bin/sponge <<'EOF'
#!/bin/sh
cat > /tmp/sponge-output
mv /tmp/sponge-output "$1"
EOF
chmod +x /tmp/bin/sponge
export PATH="/tmp/bin:$PATH" OPENVPN_EXTERNAL_IP=203.0.113.1 DOCKER_SUBNET=10.200.0.0/24
bash /source/files-ui/ui-start.sh
sqlite3 /opt/openvpn-ui/db/data.db "select custom_opt_two from o_v_config where profile='default';" > /tmp/options-before
bash /source/files-ui/ui-start.sh
sqlite3 /opt/openvpn-ui/db/data.db "select custom_opt_two from o_v_config where profile='default';" > /tmp/options-after
cmp /tmp/options-before /tmp/options-after
test "$(cat /run/docktinel/ui-health-url)" = http://127.0.0.1:8081/login
test "$(sqlite3 /opt/openvpn-ui/db/data.db "select server_address from o_v_client_config where profile='default';")" = 203.0.113.1
""", image=os.environ.get("OPENVPN_UI_TEST_IMAGE", "nmisha/antizapret-vpn-wireguard:6.9.0-1"))


    def test_optional_client_firewall(self):
        for value in ('false', '0', 'unset', 'invalid'):
            with self.subTest(value=value):
                self.run_case(r"""
ip link add eth0 type dummy
ip addr add 10.200.0.2/24 dev eth0
ip link set eth0 up
ip route add default dev eth0
mkdir -p /etc/openvpn
""" + (f'export VPN_CLIENT_FIREWALL={value}\n' if value != 'unset' else 'unset VPN_CLIENT_FIREWALL\n') + r"""
if [ "${VPN_CLIENT_FIREWALL:-}" = invalid ]; then
    if bash /source/files/fw-rules.sh; then exit 1; fi
else
    bash /source/files/fw-rules.sh
    iptables -t nat -C POSTROUTING -s 10.1.165.0/24 -j masq_not_local
fi
! iptables -S az_clients
! iptables -C FORWARD -s 10.1.165.0/24 -p icmp --icmp-type echo-request -j DROP
""")


if __name__ == "__main__":
    unittest.main()
