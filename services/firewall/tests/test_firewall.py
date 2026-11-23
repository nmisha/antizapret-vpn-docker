"""Integration checks in an isolated Docker network namespace (never host networking)."""
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGE = os.environ.get("FIREWALL_TEST_IMAGE", "nmisha/antizapret-vpn-firewall:6.4.0-3")
SETUP = r"""
set -euo pipefail
cp /source/*.sh /root/
chmod +x /root/*.sh
cd /root
iptables -N DOCKER-USER
ip6tables -N DOCKER-USER
export V4_FILE=/tmp/local4 V6_FILE=/tmp/local6
export V4_URL=file:///tmp/input4 V6_URL=file:///tmp/input6
printf '192.0.2.0/24\n' > /tmp/input4
printf '2001:db8::/32\n' > /tmp/input6
./refresh.sh
./healthcheck.sh
"""


class FirewallTests(unittest.TestCase):
    def run_case(self, script):
        result = subprocess.run([
            "docker", "run", "--rm", "--network", "none", "--cap-add", "NET_ADMIN",
            "--entrypoint", "bash", "--mount", f"type=bind,source={ROOT},target=/source,readonly",
            IMAGE, "-ec", SETUP + script,
        ], capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_rules_exceptions_and_health(self):
        self.run_case(r"""
printf 'eth0 203.0.113.10 tcp 25,443\n' > /tmp/exceptions
export EXCEPTIONS_FILE=/tmp/exceptions
./refresh.sh
iptables -C AZ-FIREWALL -i eth0 -p tcp -m conntrack --ctdir ORIGINAL --ctorigdst 203.0.113.10 --ctorigdstport 443 -j RETURN
! iptables -C DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
./healthcheck.sh
iptables -D AZ-FIREWALL -m set --match-set az_firewall_v4 src -j DROP
if ./healthcheck.sh; then exit 1; fi
""")

    def test_independent_families_and_cache_restore(self):
        self.run_case(r"""
cp /var/lib/antizapret-firewall/v4 /tmp/saved4
printf '198.51.100.0/24\n' > /tmp/input4
printf '::::\n' > /tmp/input6
if ./refresh.sh; then exit 1; fi
ipset test az_firewall_v4 198.51.100.1
ipset test az_firewall_v6 2001:db8::1
printf '2001:db8:1::/48\n' > /tmp/input6
rm /tmp/input4
if ./refresh.sh; then exit 1; fi
ipset test az_firewall_v4 198.51.100.1
ipset test az_firewall_v6 2001:db8:1::1
./healthcheck.sh
./block.sh clear
rm /tmp/input6
./refresh.sh --restore
./healthcheck.sh
ipset test az_firewall_v4 198.51.100.1
ipset test az_firewall_v6 2001:db8:1::1
""")

    def test_invalid_exceptions_and_empty_lists_keep_state(self):
        self.run_case(r"""
cp /var/lib/antizapret-firewall/v4 /tmp/saved4
cp /var/lib/antizapret-firewall/v6 /tmp/saved6
iptables-save > /tmp/rules-before
export EXCEPTIONS_FILE=/tmp/exceptions
printf 'eth0 invalid tcp 443\n' > /tmp/exceptions
if ./refresh.sh; then exit 1; fi
iptables-save > /tmp/rules-after
cmp /tmp/rules-before /tmp/rules-after
cmp /tmp/saved4 /var/lib/antizapret-firewall/v4
cmp /tmp/saved6 /var/lib/antizapret-firewall/v6
unset EXCEPTIONS_FILE
printf '# no networks\n' > /tmp/input4
: > /tmp/input6
if ./refresh.sh; then exit 1; fi
cmp /tmp/saved4 /var/lib/antizapret-firewall/v4
cmp /tmp/saved6 /var/lib/antizapret-firewall/v6
./healthcheck.sh
""")


if __name__ == "__main__":
    unittest.main()
