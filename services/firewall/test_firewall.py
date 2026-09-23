import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
import sys
from unittest.mock import patch
from types import SimpleNamespace

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('firewall', Path(__file__).with_name('firewall.py'))
fw = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fw)


class FirewallTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        for name, text in [('v4', '192.0.2.0/24\n'), ('v6', '2001:db8::/32\n'),
                           ('exceptions', 'ens3 192.0.2.1 tcp 25,443\nens3 2001:db8::1 udp 443\n')]:
            (self.root / name).write_text(text)
        env = patch.dict(os.environ, {'V4_FILE': str(self.root / 'v4'),
                                    'V6_FILE': str(self.root / 'v6'),
                                    'EXCEPTIONS_FILE': str(self.root / 'exceptions')})
        env.start()
        self.addCleanup(env.stop)

    def test_invalid_inputs_never_touch_live_state(self):
        for name, bad in [('v4', ''), ('v6', '192.0.2.1'),
                          ('exceptions', 'ens3 192.0.2.1 tcp 70000')]:
            path = self.root / name
            old = path.read_text()
            path.write_text(bad)
            with patch.object(fw, 'run') as run:
                with self.assertRaises(ValueError):
                    fw.apply()
                run.assert_not_called()
            path.write_text(old)

    def test_exception_is_destination_scoped_before_drop(self):
        calls = []
        def run(*args, **kwargs):
            calls.append((args, kwargs))
            return SimpleNamespace(returncode=1 if '-C' in args else 0)
        with patch.object(fw, 'run', side_effect=run):
            fw.apply()
        for cmd in ['iptables-restore', 'ip6tables-restore']:
            args, kwargs = next(c for c in calls if c[0][0] == cmd)
            self.assertIn('--noflush', args)
            text = kwargs['data']
            self.assertIn('--ctdir ORIGINAL --ctorigdst ', text)
            self.assertLess(text.index('--ctorigdstport'), text.index('-j DROP'))
            self.assertNotIn('-j ACCEPT', text)
        stages = [i for i, c in enumerate(calls) if c[0][:2] == ('ipset', 'restore')]
        swaps = [i for i, c in enumerate(calls) if c[0][:2] == ('ipset', 'swap')]
        self.assertLess(max(stages), min(swaps))

    def test_failed_staging_does_not_swap_or_rewrite_rules(self):
        calls = []
        def run(*args, **kwargs):
            calls.append(args)
            if args[:2] == ('ipset', 'restore'):
                raise OSError('simulated ipset failure')
            return SimpleNamespace(returncode=0)
        with patch.object(fw, 'run', side_effect=run):
            with self.assertRaises(OSError):
                fw.apply()
        self.assertFalse(any(c[:2] == ('ipset', 'swap') for c in calls))
        self.assertFalse(any(c[0].endswith('-restore') for c in calls))


if __name__ == '__main__':
    unittest.main()
