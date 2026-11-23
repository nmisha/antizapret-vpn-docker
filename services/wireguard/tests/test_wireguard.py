"""SQLite and iptables checks in an isolated Docker network namespace."""
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[3]
IMAGE = os.environ.get("WIREGUARD_TEST_IMAGE", "nmisha/antizapret-vpn-wireguard:6.9.0-1")
SETUP = r"""
set -euo pipefail
mkdir -p /etc/wireguard /opt/antizapret/result /tmp/bin
for migration in /app/server/database/migrations/*.sql; do
    sqlite3 -bail /etc/wireguard/wg-easy.db < "$migration" > /dev/null
done
ip link add eth0 type dummy
ip addr add 10.200.0.2/24 dev eth0
ip link set eth0 up
ip route add default dev eth0
printf '192.0.2.0/24\n' > /opt/antizapret/result/ips.txt
cat > /tmp/bin/node <<'EOF'
#!/bin/sh
touch /tmp/node-started
EOF
chmod +x /tmp/bin/node
export PATH="/tmp/bin:$PATH" WIREGUARD_PASSWORD=integration-password WG_HOST=203.0.113.1
export WG_ALLOWED_IPS=10.1.166.0/24,192.0.2.0/24 WG_PERSISTENT_KEEPALIVE=25
"""


class WireguardTests(unittest.TestCase):
    def run_case(self, script):
        result = subprocess.run([
            "docker", "run", "--rm", "--network", "none", "--cap-add", "NET_ADMIN",
            "--entrypoint", "bash", "--mount",
            f"type=bind,source={ROOT / 'services/wireguard'},target=/source,readonly",
            "--mount", f"type=bind,source={ROOT / 'services/antizapret/common/docktinel'},target=/usr/local/lib/docktinel,readonly",
            IMAGE, "-ec", SETUP + script,
        ], capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_named_interface_hooks_mtu_and_amnezia_settings(self):
        self.run_case(r"""
sqlite3 /etc/wireguard/wg-easy.db "PRAGMA foreign_keys=ON; UPDATE interfaces_table SET name='existing0';"
export VPN_CLIENT_FIREWALL=true
export WG_INTERFACE=vpn1 WG_MTU=1337 MTU=1280 EXPERIMENTAL_AWG=true JC=4 JMIN=30 JMAX=90
bash /source/init.sh
test -f /tmp/node-started
test -f /dev/shm/.wireguard_started
test "$(sqlite3 /etc/wireguard/wg-easy.db "select mtu from interfaces_table where name='existing0';")" = 1337
test "$(sqlite3 /etc/wireguard/wg-easy.db "select firewall_enabled from interfaces_table where name='existing0';")" = 0
test "$(sqlite3 /etc/wireguard/wg-easy.db "select j_c from interfaces_table where name='existing0';")" = 4
up=$(sqlite3 /etc/wireguard/wg-easy.db "select post_up from hooks_table where id='existing0';")
down=$(sqlite3 /etc/wireguard/wg-easy.db "select post_down from hooks_table where id='existing0';")
bash -ec "$up"
iptables -C FORWARD -i vpn1 -j ACCEPT
iptables -C FORWARD -s 10.1.166.0/24 -j az_clients
iptables -C az_clients -d 192.0.2.0/24 -j ACCEPT
iptables -C az_clients -j REJECT --reject-with icmp-admin-prohibited
iptables -t mangle -C FORWARD -s 10.1.166.0/24 -p icmp --icmp-type echo-request -j DROP
bash -ec "$down"
! iptables -S az_clients
bash /source/healthcheck.sh
printf '198.51.100.0/24\n' >> /opt/antizapret/result/ips.txt
if bash /source/healthcheck.sh; then exit 1; fi
""")

    def test_failed_ipv4_detection_preserves_saved_host(self):
        self.run_case(r"""
sqlite3 /etc/wireguard/wg-easy.db "UPDATE user_configs_table SET host='saved.example.org';"
cat > /tmp/bin/curl <<'EOF'
#!/bin/sh
echo 999.999.999.999
EOF
chmod +x /tmp/bin/curl
export WG_HOST=
bash /source/init.sh
test "$(sqlite3 /etc/wireguard/wg-easy.db 'select host from user_configs_table limit 1;')" = saved.example.org
""")

    def test_invalid_interface_rejects_start(self):
        self.run_case(r"""
if WG_INTERFACE='bad/interface' bash /source/init.sh; then exit 1; fi
test ! -f /tmp/node-started
""")


    def test_optional_client_firewall(self):
        for value in ('false', '0', 'unset', 'invalid'):
            with self.subTest(value=value):
                self.run_case((f'export VPN_CLIENT_FIREWALL={value}\n' if value != 'unset' else 'unset VPN_CLIENT_FIREWALL\n') + r"""
sqlite3 /etc/wireguard/wg-easy.db "UPDATE interfaces_table SET firewall_enabled=1;"
if [ "${VPN_CLIENT_FIREWALL:-}" = invalid ]; then
    if bash /source/init.sh; then exit 1; fi
    test ! -f /tmp/node-started
else
    bash /source/init.sh
    test "$(sqlite3 /etc/wireguard/wg-easy.db 'select firewall_enabled from interfaces_table limit 1;')" = 1
    up=$(sqlite3 /etc/wireguard/wg-easy.db 'select post_up from hooks_table limit 1;')
    down=$(sqlite3 /etc/wireguard/wg-easy.db 'select post_down from hooks_table limit 1;')
    bash -ec "$up"
    iptables -C FORWARD -i wg0 -j ACCEPT
    ! iptables -S az_clients
    ! iptables -t mangle -C FORWARD -s 10.1.166.0/24 -p icmp --icmp-type echo-request -j DROP
    bash -ec "$down"
fi
""")


if __name__ == "__main__":
    unittest.main()
