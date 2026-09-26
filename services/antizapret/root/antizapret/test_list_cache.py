"""Run with python3 -B -m unittest discover -s services/antizapret/root/antizapret."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent


class ListCacheTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        for name in ('config', 'result', 'bin'):
            (self.root / name).mkdir()
        for name in ('list-cache.sh', 'download.sh', 'doall.sh'):
            shutil.copyfile(SOURCE / name, self.root / name)
        # Avoid dependencies on the host's /etc/default, process table and /tmp lock.
        doall = self.root / 'doall.sh'
        doall.write_text(doall.read_text().replace('/etc/default/antizapret',
                         str(self.root / 'defaults')).replace('/tmp/.doall',
                         str(self.root / '.doall')).replace('/root/antizapret/result',
                         str(self.root / 'result')))
        for path in self.root.glob('*.sh'):
            path.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.root / 'bin') + ':' + os.environ['PATH'],
                        IPS_URL='ips', IPS_WORLD_URL='', ASN_URL='asn', ASN_WORLD_URL='',
                        DOALL_DISABLED='')
        self.script('bin/curl', '[ "${FAIL_DOWNLOAD:-}" != 1 ] || exit 22\n'
                    'case "${@: -1}" in ips) echo 192.0.2.0/24;; asn) :;; esac')
        self.script('bin/pkill', 'exit 0')
        self.script('bin/sponge', 'data=$(cat); printf "%s" "$data" > "$1"')
        # Exercise doall's success/failure contract independently of route generation.
        self.script('parse.sh', '[ "${FAIL_PARSE:-}" != 1 ] || exit 2\n'
                    'cp config/include-ips-dist.txt result/ips.txt\n'
                    'cp config/include-asn-dist.txt result/asn.txt')

    def script(self, name, body):
        path = self.root / name
        path.write_text('#!/bin/bash\n' + body + '\n')
        path.chmod(0o755)

    def run_shell(self, command, **env):
        return subprocess.run(['bash', '-c', command], cwd=self.root,
                              env=dict(self.env, **env), capture_output=True, text=True)

    def test_empty_download_survives_later_network_failure(self):
        initial = self.run_shell('bash doall.sh')
        self.assertEqual(initial.returncode, 0, initial.stderr)
        self.assertEqual((self.root / 'result/asn.txt').read_text(), '')
        self.assertTrue((self.root / 'result/.asn.txt.ready').exists())
        offline = self.run_shell('bash doall.sh', FAIL_DOWNLOAD='1')
        self.assertEqual(offline.returncode, 0, offline.stderr)
        check = self.run_shell('source list-cache.sh; required_lists_available result ""')
        self.assertEqual(check.returncode, 0, check.stderr)

    def test_initial_empty_placeholders_are_not_a_cache(self):
        for kind in ('ips', 'asn'):
            (self.root / f'result/{kind}.txt').touch()
        self.assertNotEqual(self.run_shell('bash doall.sh', FAIL_DOWNLOAD='1').returncode, 0)
        self.assertNotEqual(self.run_shell(
            'source list-cache.sh; required_lists_available result ""').returncode, 0)

    def test_legacy_nonempty_files_are_accepted_but_missing_files_are_not(self):
        for kind in ('ips', 'asn'):
            (self.root / f'result/{kind}.txt').write_text('legacy\n')
        command = 'source list-cache.sh; required_lists_available result ""'
        self.assertEqual(self.run_shell(command).returncode, 0)
        (self.root / 'result/asn.txt').unlink()
        (self.root / 'result/.asn.txt.ready').touch()
        self.assertNotEqual(self.run_shell(command).returncode, 0)

    def test_failed_generation_does_not_leave_success_markers(self):
        self.assertEqual(self.run_shell('bash doall.sh').returncode, 0)
        self.assertNotEqual(self.run_shell('bash doall.sh', FAIL_PARSE='1').returncode, 0)
        self.assertFalse((self.root / 'result/.asn.txt.ready').exists())
