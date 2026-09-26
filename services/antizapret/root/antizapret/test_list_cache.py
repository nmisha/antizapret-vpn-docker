"""Run with python3 -B -m unittest discover -s services/antizapret/root/antizapret."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
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
        if not shutil.which('flock'):
            # macOS has no util-linux CLI; exercise the same inherited-fd lock
            # through the native flock syscall. Docker tests use real flock.
            wrapper = self.root / 'bin/flock'
            wrapper.write_text('#!' + sys.executable + '\nimport fcntl,sys\n'
                               'fcntl.flock(int(sys.argv[-1]), fcntl.LOCK_EX)\n')
            wrapper.chmod(0o755)
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

    def test_failed_generation_preserves_previous_success_markers(self):
        self.assertEqual(self.run_shell('bash doall.sh').returncode, 0)
        self.assertNotEqual(self.run_shell('bash doall.sh', FAIL_PARSE='1').returncode, 0)
        self.assertTrue((self.root / 'result/.asn.txt.ready').exists())

    def prepare_real_parser(self):
        shutil.copyfile(SOURCE / 'parse.sh', self.root / 'parse.sh')
        shutil.copytree(SOURCE / 'scripts', self.root / 'scripts')
        (self.root / 'config/custom').mkdir()
        for kind in ('ips', 'ips-world', 'asn', 'asn-world'):
            (self.root / f'config/include-{kind}-dist.txt').write_text('')
            for prefix in ('include', 'exclude'):
                (self.root / f'config/custom/{prefix}-{kind}-custom.txt').touch()
            (self.root / f'result/{kind}.txt').write_text('old-data\n')
            (self.root / f'result/.{kind}.txt.ready').touch()
        (self.root / 'config/include-ips-dist.txt').write_text('192.0.2.0/24\n')
        self.env['DOCKER_SUBNET'] = ''
        self.script('bin/sipcalc', 'echo "Network mask - 255.255.255.0"')

    def test_invalid_regex_keeps_published_lists_and_markers(self):
        self.prepare_real_parser()
        (self.root / 'config/custom/exclude-ips-custom.txt').write_text('[\n')
        result = self.run_shell('bash parse.sh')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / 'result/ips.txt').read_text(), 'old-data\n')
        self.assertTrue((self.root / 'result/.ips.txt.ready').exists())

    def test_missing_input_keeps_published_lists(self):
        self.prepare_real_parser()
        (self.root / 'config/include-asn-dist.txt').unlink()
        self.assertNotEqual(self.run_shell('bash parse.sh').returncode, 0)
        self.assertEqual((self.root / 'result/ips.txt').read_text(), 'old-data\n')

    def test_excluding_all_addresses_is_successful(self):
        self.prepare_real_parser()
        (self.root / 'config/custom/exclude-ips-custom.txt').write_text('.*\n')
        result = self.run_shell('bash parse.sh')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'result/ips.txt').read_text(), '')

    def test_parallel_refreshes_are_serialized_and_stale_file_is_harmless(self):
        self.script('download.sh', 'exit 0')
        self.env.update(IPS_URL='', ASN_URL='')
        self.script('parse.sh', 'echo start >> events; sleep 0.2; echo end >> events')
        (self.root / '.doall_lock').touch()
        processes = [subprocess.Popen(['bash', 'doall.sh'], cwd=self.root,
                     env=self.env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                     for _ in range(3)]
        try:
            for process in processes:
                self.assertEqual(process.wait(timeout=5), 0)
        finally:
            for process in processes:
                if process.poll() is None:
                    process.kill()
                process.wait()
        self.assertEqual((self.root / 'events').read_text().splitlines(),
                         ['start', 'end'] * 3)

    def test_killed_lock_holder_does_not_block_next_refresh(self):
        self.script('download.sh', 'exit 0')
        self.script('parse.sh', 'exit 0')
        self.env.update(IPS_URL='', ASN_URL='')
        holder = subprocess.Popen(['bash', '-c',
            'exec 9>.doall_lock; flock -x 9; touch locked; exec sleep 30'],
            cwd=self.root, env=self.env)
        try:
            deadline = time.monotonic() + 3
            while not (self.root / 'locked').exists() and time.monotonic() < deadline:
                time.sleep(0.01)
            self.assertTrue((self.root / 'locked').exists())
        finally:
            holder.kill()
            holder.wait()
        result = subprocess.run(['bash', 'doall.sh'], cwd=self.root,
                                env=self.env, capture_output=True, timeout=3)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_disabled_source_can_be_missing_on_first_start(self):
        self.prepare_real_parser()
        (self.root / 'config/include-ips-world-dist.txt').unlink()
        result = self.run_shell('bash parse.sh')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'result/ips-world.txt').read_text(), '')

    def test_failed_first_source_preserves_cache_and_updates_later_lists(self):
        self.script('bin/curl', 'case "${@: -1}" in bad) echo partial; exit 22;; '
                    'world) echo 198.51.100.0/24;; asn) echo 64500;; '
                    'asn-world) echo 64501;; esac')
        cached = self.root / 'config/include-ips-dist.txt'
        cached.write_text('# cached comment\n192.0.2.0/24\n')
        result = self.run_shell('bash download.sh', IPS_URL='bad',
                                IPS_WORLD_URL='world', ASN_WORLD_URL='asn-world')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(cached.read_text(), '# cached comment\n192.0.2.0/24\n')
        for kind, expected in [('ips-world', '198.51.100.0/24'),
                               ('asn', '64500'), ('asn-world', '64501')]:
            self.assertEqual((self.root / f'config/include-{kind}-dist.txt').read_text().strip(), expected)
            self.assertTrue((self.root / f'config/.include-{kind}-dist.txt.ready').exists())
        self.assertFalse(list((self.root / 'config').glob('*.tmp*')))

    def test_doall_generates_with_failed_source_cache_and_fresh_asn(self):
        (self.root / 'config/include-ips-dist.txt').write_text('192.0.2.0/24\n')
        self.script('bin/curl', 'case "${@: -1}" in ips) exit 22;; asn) echo 64500;; esac')
        result = self.run_shell('bash doall.sh')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'result/ips.txt').read_text(), '192.0.2.0/24\n')
        self.assertEqual((self.root / 'result/asn.txt').read_text().strip(), '64500')

    def test_missing_failed_source_blocks_generation_but_not_other_downloads(self):
        self.script('bin/curl', 'case "${@: -1}" in ips) exit 22;; asn) echo 64500;; esac')
        result = self.run_shell('bash doall.sh')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / 'config/include-asn-dist.txt').read_text().strip(), '64500')
        self.assertFalse((self.root / 'result/asn.txt').exists())
