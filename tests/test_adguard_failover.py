"""Exercise the current maintenance job, separately from DNS liveness.

Run: python -B -m unittest discover -s tests -v.
Uses an existing image, no network, host firewall or persistent volumes.
"""
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGE = os.environ.get("ADGUARD_TEST_IMAGE", "nmisha/antizapret-vpn-adguard:6.9.0-1")
SETUP = r"""
set -euo pipefail
mkdir -p /tmp/bin
cp /source/dns-reload.sh /root/dns-reload.sh
printf 'old-local old-world\n' > /dev/shm/.config_md5
touch /dev/shm/.inited
export AZ_WORLD_ENABLED=1 ADGUARDHOME_USERNAME=admin ADGUARDHOME_PASSWORD=test
cat > /tmp/bin/getent <<'SH'
#!/bin/bash
case "$2" in
az-local.antizapret) echo '10.0.0.2 az-local';;
az-world.antizapret) [ "${WORLD_DOWN:-0}" = 1 ] || echo '10.0.0.4 az-world';;
coredns.antizapret) echo '10.0.0.3 coredns';;
esac
SH
cat > /tmp/bin/curl <<'SH'
#!/bin/bash
printf '%s\n' "$*" >> /tmp/requests
case "$*" in
*config-md5*)
 case "$*" in
 *az-world*) [ "${WORLD_DOWN:-0}" != 1 ] || exit 28; echo new-world;;
 *) echo new-local;;
 esac;;
*/control/dns_info*) echo '{"upstream_dns":["coredns.antizapret"]}';;
*/control/clients/update*) [ "${CLIENT_UPDATE_FAIL:-0}" != 1 ] || exit 22;;
*/control/clients*)
 [ "${API_DOWN:-0}" != 1 ] || exit 7
 echo '{"clients":[{"name":"az-local","ids":["az-local","10.0.0.2"]},{"name":"az-world","ids":["az-world"]},{"name":"coredns","ids":["coredns","10.0.0.3"]}]}';;
*/control/filtering/refresh*)
 if [ "${REFRESH_FAIL:-0}" = 1 ]; then printf 'busy\n500'; else printf '{}\n200'; fi;;
*/control/cache_clear*) [ "${CACHE_CLEAR_FAIL:-0}" != 1 ] || exit 22;;
*/control/dns_config*) :;;
*) exit 99;;
esac
SH
chmod +x /tmp/bin/*
export PATH="/tmp/bin:$PATH"
"""


class AdguardFailoverTests(unittest.TestCase):
    def run_case(self, script):
        result = subprocess.run([
            "docker", "run", "--rm", "--network", "none", "--entrypoint", "bash",
            "--mount", f"type=bind,source={ROOT / 'services/adguard'},target=/source,readonly",
            IMAGE, "-ec", SETUP + script,
        ], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_missing_world_defers_refresh_until_recovery(self):
        self.run_case(r"""
if WORLD_DOWN=1 bash /source/refresh-state.sh; then exit 1; fi
test "$(cat /dev/shm/.config_md5)" = 'old-local old-world'
! grep -q /filtering/refresh /tmp/requests
bash /source/refresh-state.sh
test "$(cat /dev/shm/.config_md5)" = 'new-local new-world'
grep -q /filtering/refresh /tmp/requests
kill -0 1
""")

    def test_failed_refresh_keeps_checksum_and_retries(self):
        self.run_case(r"""
if REFRESH_FAIL=1 bash /source/refresh-state.sh; then exit 1; fi
test "$(cat /dev/shm/.config_md5)" = 'old-local old-world'
bash /source/refresh-state.sh
test "$(cat /dev/shm/.config_md5)" = 'new-local new-world'
: > /tmp/requests
bash /source/refresh-state.sh
! grep -q /filtering/refresh /tmp/requests
""")

    def test_api_failure_is_reported_by_maintenance(self):
        self.run_case(r"""
if API_DOWN=1 bash /source/refresh-state.sh; then exit 1; fi
test "$(cat /dev/shm/.config_md5)" = 'old-local old-world'
kill -0 1
""")

    def test_single_node_does_not_probe_world(self):
        self.run_case(r"""
AZ_WORLD_ENABLED=0 bash /source/refresh-state.sh
test "$(cat /dev/shm/.config_md5)" = new-local
! grep -q az-world.antizapret /tmp/requests
""")

    def test_client_update_failure_retries_dirty_cache(self):
        self.run_case(r"""
if CLIENT_UPDATE_FAIL=1 bash /source/refresh-state.sh; then exit 1; fi
test -f /dev/shm/.clients_cache_dirty
bash /source/refresh-state.sh
test ! -f /dev/shm/.clients_cache_dirty
grep -q /control/clients/update /tmp/requests
""")

    def test_cache_clear_retries_without_client_changes(self):
        self.run_case(r"""
if CACHE_CLEAR_FAIL=1 bash /source/refresh-state.sh; then exit 1; fi
test -f /dev/shm/.clients_cache_dirty
sed -i 's/"ids":\["az-world"\]/"ids":["az-world","10.0.0.4"]/' /tmp/bin/curl
: > /tmp/requests
bash /source/refresh-state.sh
test ! -f /dev/shm/.clients_cache_dirty
grep -q /control/cache_clear /tmp/requests
! grep -q /control/clients/update /tmp/requests
""")
