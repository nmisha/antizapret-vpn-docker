"""Exercise recovery decisions without Docker or host firewall mutations."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class RecoveryTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = dict(os.environ, PATH=str(self.root) + ':' + os.environ['PATH'],
                        LOG=str(self.root / 'log'))

    def command(self, name, body):
        path = self.root / name
        path.write_text('#!/bin/bash\n' + body + '\n')
        path.chmod(0o755)

    def run_shell(self, text, **env):
        return subprocess.run(['bash', '-c', text], cwd=self.root,
                              env=dict(self.env, **env), capture_output=True,
                              text=True, timeout=5)

    def test_firewall_attempts_both_families_and_applies_after_download_failure(self):
        self.command('download.sh', 'echo "download $1" >> "$LOG"; [ "$1" != "$FAIL" ]')
        self.command('block.sh', 'echo apply >> "$LOG"')
        self.command('sleep', 'echo "sleep $1" >> "$LOG"')
        script = (ROOT / 'services/firewall/entrypoint.sh').read_text().replace(
            'while true; do', 'for iteration in 1; do')
        for failure in ('v4', 'v6', 'none'):
            with self.subTest(failure=failure):
                result = self.run_shell(script, V4_URL='v4', V6_URL='v6',
                                        V4_FILE='v4.txt', V6_FILE='v6.txt',
                                        INTERVAL='3h', FAIL=failure)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((self.root / 'log').read_text().splitlines(),
                                 ['download v4', 'download v6', 'apply',
                                  'sleep ' + ('3h' if failure == 'none' else '30')])
                (self.root / 'log').unlink()

    def coredns(self, **env):
        script = (ROOT / 'services/coredns/healthcheck.sh').read_text()
        script = script.replace('/dev/shm/', str(self.root) + '/')
        script = script.replace('/Corefile', str(self.root / 'Corefile'))
        script = script.replace('/root/config.sh', str(self.root / 'config.sh'))
        # Override the shell builtin; never send a signal to a real process.
        return self.run_shell('kill() { echo kill >> "$LOG"; };\n' + script, **env)

    def prepare_coredns(self):
        (self.root / '.coredns_started').write_text('0\n')
        (self.root / 'Corefile').write_text('configuration')
        self.command('date', 'echo "${NOW:-100}"')
        self.command('timeout', 'shift; exec "$@"')
        self.command('config.sh', 'exit "${CONFIG_FAIL:-0}"')
        self.command('dig', '[ "${DNS_DOWN:-0}" != 1 ] || exit 9; echo 127.0.0.1')

    def test_coredns_failure_threshold_and_recovery(self):
        self.prepare_coredns()
        for _ in range(2):
            self.assertNotEqual(self.coredns(DNS_DOWN='1').returncode, 0)
            self.assertFalse((self.root / 'log').exists())
        self.assertEqual(self.coredns().returncode, 0)
        self.assertFalse((self.root / '.coredns_failures').exists())
        for _ in range(3):
            self.assertNotEqual(self.coredns(DNS_DOWN='1').returncode, 0)
        self.assertEqual((self.root / 'log').read_text(), 'kill\n')

    def test_coredns_startup_grace_is_bounded(self):
        self.prepare_coredns()
        (self.root / 'Corefile').unlink()
        for _ in range(4):
            self.assertNotEqual(self.coredns(NOW='20').returncode, 0)
        self.assertFalse((self.root / 'log').exists())
        self.assertFalse((self.root / '.coredns_failures').exists())
        for _ in range(3):
            self.assertNotEqual(self.coredns(NOW='100').returncode, 0)
        self.assertEqual((self.root / 'log').read_text(), 'kill\n')

    def test_coredns_discovery_failure_does_not_restart_answering_dns(self):
        self.prepare_coredns()
        self.assertEqual(self.coredns(CONFIG_FAIL='1').returncode, 0)
        self.assertFalse((self.root / 'log').exists())
