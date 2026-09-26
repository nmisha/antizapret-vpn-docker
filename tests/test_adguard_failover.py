"""Isolated shell regression tests; run: python3 -B -m unittest discover -s tests."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parents[1] / 'services/adguard'


class AdguardFailoverTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = dict(os.environ, PATH=str(self.root) + ':' + os.environ['PATH'],
                        AZ_WORLD_ENABLED='1', ADGUARDHOME_PASSWORD='test',
                        TEST_LOG=str(self.root / 'requests'))
        (self.root / '.inited').touch()
        (self.root / '.config_md5').write_text('old-local old-world\n')
        self.command('timeout', 'shift; exec "$@"')
        self.command('getent', '''case "$2" in
az-local) echo '10.0.0.2 az-local';;
coredns) echo '10.0.0.3 coredns';;
az-world) [ "${WORLD_DOWN:-0}" = 1 ] || echo '10.0.0.4 az-world';;
esac''')
        self.command('curl', '''echo "$*" >> "$TEST_LOG"
case "$*" in
*az-local.antizapret*) [ "${LOCAL_DOWN:-0}" != 1 ] || exit 28; echo "${LOCAL_HASH:-new-local}";;
*az-world.antizapret*) [ "${WORLD_DOWN:-0}" != 1 ] || exit 28; echo "${WORLD_HASH:-new-world}";;
*/control/clients/update*) :;;
*/control/clients*) [ "${API_DOWN:-0}" != 1 ] || exit 7
 echo '{"clients":[{"name":"az-local","ids":["az-local","10.0.0.2"]},{"name":"az-world","ids":["az-world"]},{"name":"coredns","ids":["10.0.0.3"]}]}' ;;
*/filtering/refresh*) [ "${REFRESH_FAIL:-0}" != 1 ] || exit 22;;
*/cache_clear*) :;;
*) exit 99;;
esac''')

    def command(self, name, body):
        path = self.root / name
        path.write_text('#!/bin/bash\n' + body + '\n')
        path.chmod(0o755)

    def run_script(self, text, **environment):
        text = text.replace('/.inited', str(self.root / '.inited')).replace(
            '/.config_md5', str(self.root / '.config_md5'))
        return subprocess.run(['bash', '-c', text], env=dict(self.env, **environment),
                              capture_output=True, text=True, timeout=15)

    def healthcheck(self, **environment):
        return self.run_script((SOURCE / 'healthcheck.sh').read_text(), **environment)

    def test_missing_world_allows_local_refresh(self):
        result = self.healthcheck(WORLD_DOWN='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / '.config_md5').read_text(), 'old-local old-world\n')
        self.assertIn('/filtering/refresh', (self.root / 'requests').read_text())
        self.assertEqual((self.root / '.config_md5.local').read_text(), 'new-local\n')
        self.assertFalse((self.root / '.config_md5.world').exists())
        self.assertTrue((self.root / '.config_md5.world_pending').exists())

    def test_filter_failure_keeps_dns_healthy_and_retries_after_recovery(self):
        failed = self.healthcheck(REFRESH_FAIL='1')
        self.assertEqual(failed.returncode, 0, failed.stderr)
        self.assertEqual((self.root / '.config_md5').read_text(), 'old-local old-world\n')
        recovered = self.healthcheck()
        self.assertEqual(recovered.returncode, 0, recovered.stderr)
        self.assertEqual((self.root / '.config_md5.local').read_text(), 'new-local\n')
        self.assertEqual((self.root / '.config_md5.world').read_text(), 'new-world\n')
        self.assertIn('/control/clients/update', (self.root / 'requests').read_text())

    def test_local_api_failure_still_fails_healthcheck(self):
        self.assertNotEqual(self.healthcheck(API_DOWN='1').returncode, 0)

    def test_startup_dependency_phase_does_not_wait_for_world(self):
        text = (SOURCE / 'entrypoint.sh').read_text()
        # Run the actual dependency/config-metadata phase without iptables or yq.
        phase = text[text.index('function resolve ()'):text.index('function ensure_filter ()')]
        result = self.run_script(phase + '\nprintf "%s" "$AZ_WORLD_CLIENT_IDS"', WORLD_DOWN='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), ['az-world'])

    def test_world_recovery_with_same_checksum_still_refreshes(self):
        self.assertEqual(self.healthcheck(WORLD_DOWN='1').returncode, 0)
        (self.root / 'requests').write_text('')
        result = self.healthcheck(WORLD_HASH='old-world')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('/filtering/refresh', (self.root / 'requests').read_text())
        self.assertFalse((self.root / '.config_md5.world_pending').exists())
        (self.root / 'requests').write_text('')
        self.assertEqual(self.healthcheck(WORLD_HASH='old-world').returncode, 0)
        self.assertNotIn('/filtering/refresh', (self.root / 'requests').read_text())

    def test_missing_local_allows_world_refresh(self):
        result = self.healthcheck(LOCAL_DOWN='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / '.config_md5.world').read_text(), 'new-world\n')
        self.assertFalse((self.root / '.config_md5.local').exists())

    def test_both_exits_down_skip_refresh(self):
        result = self.healthcheck(LOCAL_DOWN='1', WORLD_DOWN='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('/filtering/refresh', (self.root / 'requests').read_text())

    def test_single_node_does_not_probe_world(self):
        result = self.healthcheck(AZ_WORLD_ENABLED='0')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('az-world.antizapret', (self.root / 'requests').read_text())
        self.assertEqual((self.root / '.config_md5.local').read_text(), 'new-local\n')
