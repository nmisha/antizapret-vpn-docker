import ipaddress
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
from unittest.mock import patch
import unittest

from nat46 import JoolBackend, MappingError


class JoolTest(unittest.TestCase):
    def setUp(self):
        self.entries = {}
        self.timeout_after_add = False
        self.backend = JoolBackend(runner=self.run_jool)

    def run_jool(self, args, **kwargs):
        op = args[4:]
        if op[0] == 'display':
            out = 'IPv6 Prefix,IPv4 Prefix\n' + ''.join('{},{}\n'.format(v, k) for k, v in self.entries.items())
            return SimpleNamespace(returncode=0, stdout=out, stderr='')
        fake, real = op[1:]
        if op[0] == 'add':
            self.entries[fake] = real
            if self.timeout_after_add:
                raise subprocess.TimeoutExpired(args, 10)
        elif op[0] == 'remove':
            del self.entries[fake]
        else:
            raise AssertionError(args)
        return SimpleNamespace(returncode=0, stdout='', stderr='')

    def test_lost_acknowledgement_is_reconciled_and_add_is_idempotent(self):
        self.timeout_after_add = True
        self.backend.add('14.18.0.2', '2001:db8::1')
        self.backend.add('14.18.0.2', '2001:db8::1')
        self.assertEqual(len(self.entries), 1)

    def test_conflicting_address_cannot_be_overwritten_or_removed(self):
        self.backend.add('14.18.0.2', '2001:db8::1')
        with self.assertRaises(MappingError):
            self.backend.add('14.18.0.2', '2001:db8::2')
        with self.assertRaises(MappingError):
            self.backend.remove('14.18.0.2', '2001:db8::2')
        self.assertEqual(len(self.entries), 1)

    def test_delete_confirms_absence(self):
        self.backend.add('14.18.0.2', '2001:db8::1')
        self.backend.remove('14.18.0.2', '2001:db8::1')
        self.assertEqual(self.backend.mappings(), {})

    def test_persistence_round_trip_and_reserved_address_rejection(self):
        self.backend.add('14.18.0.2', '2001:db8::1')
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'exit.nat46.rules'
            self.backend.save(path, '14.18.0.0/15')
            self.assertEqual(self.backend.read_file(path, '14.18.0.0/15'),
                             self.backend.mappings())
            path.write_text('14.18.0.1 2001:db8::1\n')
            with self.assertRaises(MappingError):
                self.backend.read_file(path, '14.18.0.0/15')

    def test_failed_atomic_replace_preserves_previous_checkpoint(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'exit.nat46.rules'
            self.backend.add('14.18.0.2', '2001:db8::1')
            self.backend.save(path, '14.18.0.0/15')
            previous = path.read_bytes()
            self.backend.add('14.18.0.3', '2001:db8::2')
            with patch('nat46.os.replace', side_effect=OSError('disk failure')):
                with self.assertRaises(OSError):
                    self.backend.save(path, '14.18.0.0/15')
            self.assertEqual(path.read_bytes(), previous)
            self.assertEqual(list(Path(directory).iterdir()), [path])
            self.backend.save(path, '14.18.0.0/15')
            self.assertEqual(len(self.backend.read_file(path, '14.18.0.0/15')), 2)

    def test_malformed_output_is_not_treated_as_an_empty_table(self):
        self.backend.runner = lambda *a, **k: SimpleNamespace(
            returncode=0, stdout='unexpected output\n', stderr='')
        with self.assertRaises(MappingError):
            self.backend.mappings()


if __name__ == '__main__':
    unittest.main()
