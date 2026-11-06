import ipaddress
from pathlib import Path
import subprocess
import tempfile
import unittest
from nat46 import DNAT6Backend, HostPolicy, MappingError


class Kernel:
    def __init__(self):
        self.rules = []
        self.ipv4_rules = []
        self.uncertain = False

    def __call__(self, args, **kwargs):
        if args[0] == 'iptables':
            return subprocess.CompletedProcess(args, 0, '-N dnsmap\n' + '\n'.join(self.ipv4_rules), '')
        if args[0] == 'ip6tables-restore':
            self.rules += [line for line in kwargs['input'].splitlines() if line.startswith('-A ')]
            if self.uncertain:
                raise subprocess.TimeoutExpired(args, 10)
        elif '-D' in args:
            row = '-A nat46 ' + ' '.join(args[args.index('-D') + 2:])
            self.rules.remove(row)
        return subprocess.CompletedProcess(args, 0, '-N nat46\n' + '\n'.join(self.rules), '')


class BackendTests(unittest.TestCase):
    def setUp(self):
        self.kernel = Kernel()
        self.backend = DNAT6Backend(runner=self.kernel)

    def test_roundtrip_and_idempotence(self):
        b = self.backend
        b.add('14.18.0.2', '2001:db8::2')
        b.add('14.18.0.2', '2001:db8::2')
        self.assertEqual(len(self.kernel.rules), 1)
        self.assertEqual(str(b.encoded('14.18.0.2')), 'fd46:64::e12:2')
        b.remove('14.18.0.2', '2001:db8::2')
        self.assertEqual(b.mappings(), {})

    def test_ambiguous_install_confirmed_by_readback(self):
        self.kernel.uncertain = True
        self.backend.add('14.18.0.2', '2001:db8::2')
        self.assertEqual(len(self.backend.mappings()), 1)

    def test_conflicts_not_overwritten(self):
        self.backend.add('14.18.0.2', '2001:db8::2')
        for fake, real in [('14.18.0.2', '2001:db8::3'), ('14.18.0.3', '2001:db8::2')]:
            with self.assertRaises(MappingError):
                self.backend.add(fake, real)
        with self.assertRaises(MappingError):
            self.backend.remove('14.18.0.2', '2001:db8::3')

    def test_translator_internal_targets_rejected(self):
        for address in ('fd46:64::e12:2', 'fd46:65::1'):
            with self.assertRaises(MappingError):
                self.backend.add('14.18.0.2', address)
        self.assertEqual(self.kernel.rules, [])

    def test_unowned_rules_fail_closed(self):
        self.kernel.rules = ['-A nat46 -j ACCEPT']
        with self.assertRaises(MappingError):
            self.backend.mappings()

    def test_checkpoint_roundtrip_and_invalid_records(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'state'
            self.backend.add('14.18.0.2', '2001:db8::2')
            self.backend.save(path, '14.18.0.0/15')
            self.assertEqual(self.backend.read_file(path, '14.18.0.0/15'), self.backend.mappings())
            for record in ('14.18.0.1 2001:db8::2', '10.0.0.2 2001:db8::2',
                           '14.18.0.2 2001:db8::2\n14.18.0.3 2001:db8::2'):
                path.write_text('# nat46-state-v1\n' + record)
                with self.assertRaises(MappingError):
                    self.backend.read_file(path, '14.18.0.0/15')

    def test_combined_snapshot_reserves_both_families(self):
        self.kernel.ipv4_rules = ['-A dnsmap -d 14.18.0.3/32 -j DNAT --to-destination 192.0.2.20']
        self.backend.add('14.18.0.2', '2001:db8::2')
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'state'
            self.backend.save(path, '14.18.0.0/15')
            entries = self.backend.read_file(path, '14.18.0.0/15')
            self.assertEqual(str(entries[ipaddress.IPv4Address('14.18.0.3')]), '192.0.2.20')
            self.assertEqual(str(entries[ipaddress.IPv4Address('14.18.0.2')]), '2001:db8::2')
            self.kernel.ipv4_rules = ['-A dnsmap -d 14.18.0.2/32 -j DNAT --to-destination 192.0.2.20']
            with self.assertRaises(MappingError):
                self.backend.save(path, '14.18.0.0/15')
            self.assertEqual(entries, self.backend.read_file(path, '14.18.0.0/15'))

    def test_policy_exclude_and_reload(self):
        with tempfile.TemporaryDirectory() as directory:
            inc = Path(directory) / 'include-hosts-ipv6-custom.txt'
            exc = Path(directory) / 'exclude-hosts-ipv6-custom.txt'
            inc.write_text('example.com\n/^ipv6\\./\n')
            exc.write_text('private.example.com\n')
            policy = HostPolicy(directory)
            self.assertTrue(policy.selected('www.example.com.'))
            self.assertFalse(policy.selected('private.example.com'))
            self.assertTrue(policy.selected('ipv6.test'))
            inc.write_text('other.example\n')
            self.assertFalse(policy.selected('example.com'))


if __name__ == '__main__':
    unittest.main()
