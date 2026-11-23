"""Run ocserv scripts against real OpenSSL/iptables in an isolated namespace."""
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[3]
IMAGE = os.environ.get("OCSERV_TEST_IMAGE", "nmisha/antizapret-vpn-ocserv:6.9.0-1")
SETUP = r"""
set -euo pipefail
cp /source/certificate.sh /certificate.sh
cp /source/ocserv.tmpl /ocserv.tmpl
cp /source/az.tmpl /az.tmpl
mkdir -p /https-data/ocserv /opt/antizapret/result
printf 'vpn.example.org\n' > /https-data/ocserv/identity
printf 'dns\n' > /https-data/ocserv/identity.type
openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 365 \
    -subj /CN=vpn.example.org -addext subjectAltName=DNS:vpn.example.org \
    -keyout /https-data/ocserv/certificate.key -out /https-data/ocserv/certificate.crt 2>/dev/null
printf '192.0.2.0/24\n' > /opt/antizapret/result/ips.txt
printf '198.51.100.0/24\n' > /opt/antizapret/result/ips-world.txt
rm -f /dev/shm/.ocserv_started
"""


class OcservTests(unittest.TestCase):
    def run_case(self, script):
        result = subprocess.run([
            "docker", "run", "--rm", "--network", "none", "--cap-add", "NET_ADMIN",
            "--entrypoint", "bash", "--mount",
            f"type=bind,source={ROOT / 'services/ocserv'},target=/source,readonly",
            "--mount", f"type=bind,source={ROOT / 'services/antizapret/common/docktinel'},target=/usr/local/lib/docktinel,readonly",
            IMAGE, "-ec", SETUP + script,
        ], capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_configuration_and_client_rules(self):
        self.run_case(r"""
export VPN_CLIENT_FIREWALL=true
bash /source/ocserv.sh /bin/true
ocserv --test-config --config /run/ocserv/ocserv.conf
bash /source/healthcheck.sh
grep -q 'default-domain = vpn.example.org' /run/ocserv/ocserv.conf
iptables -C FORWARD -s 10.1.164.0/24 -j az_clients
iptables -C FORWARD -s 10.1.164.0/24 -p icmp --icmp-type echo-request -j DROP
iptables -C az_clients -d 192.0.2.0/24 -j ACCEPT
iptables -C az_clients -d 198.51.100.0/24 -j ACCEPT
iptables -C az_clients -j REJECT --reject-with icmp-admin-prohibited
""")

    def test_health_detects_routes_and_certificates_without_killing_pid1(self):
        self.run_case(r"""
bash /source/ocserv.sh /bin/true
bash /source/healthcheck.sh
printf '203.0.113.0/24\n' >> /opt/antizapret/result/ips.txt
if bash /source/healthcheck.sh; then exit 1; fi
printf '192.0.2.0/24\n' > /opt/antizapret/result/ips.txt
bash /source/healthcheck.sh
openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 365 \
    -subj /CN=vpn.example.org -addext subjectAltName=DNS:vpn.example.org \
    -keyout /https-data/ocserv/certificate.key -out /https-data/ocserv/certificate.crt 2>/dev/null
if bash /source/healthcheck.sh; then exit 1; fi
kill -0 1
""")

    def test_mismatched_key_and_invalid_address_reject_start(self):
        self.run_case(r"""
if OC_DEFAULT_ADDRESS=999.1.2.x bash /source/ocserv.sh /bin/true; then exit 1; fi
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out /https-data/ocserv/certificate.key 2>/dev/null
if CERT_WAIT_TIMEOUT=0 bash /source/ocserv.sh /bin/true; then exit 1; fi
test ! -f /dev/shm/.ocserv_started
""")


    def test_optional_client_firewall(self):
        for value in ('false', '0', 'unset', 'invalid'):
            with self.subTest(value=value):
                self.run_case((f'export VPN_CLIENT_FIREWALL={value}\n' if value != 'unset' else 'unset VPN_CLIENT_FIREWALL\n') + r"""
if [ "${VPN_CLIENT_FIREWALL:-}" = invalid ]; then
    if bash /source/ocserv.sh /bin/true; then exit 1; fi
else
    bash /source/ocserv.sh /bin/true
    test -f /dev/shm/.ocserv_started
fi
! iptables -S az_clients
! iptables -C FORWARD -s 10.1.164.0/24 -p icmp --icmp-type echo-request -j DROP
""")


if __name__ == "__main__":
    unittest.main()
