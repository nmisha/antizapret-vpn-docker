"""Confirmed Jool SIIT destination mappings; no host module management.

All commands operate in the caller's network namespace. The instance and its
source translation configuration must already exist before DNS starts.
"""
import csv
import io
import ipaddress
import os
from pathlib import Path
import subprocess
import tempfile
import threading


class MappingError(RuntimeError):
    """The caller must quarantine its reserved IPv4 until reconciliation."""


class JoolBackend:
    def __init__(self, instance='antizapret', runner=None):
        self.instance = instance
        self.runner = runner or subprocess.run
        self.lock = threading.RLock()

    def command(self, *args):
        result = self.runner(
            ['jool_siit', '-i', self.instance, *args],
            capture_output=True, text=True, timeout=10,
        )
        if result.returncode:
            raise MappingError(result.stderr.strip() or 'Jool command failed')
        return result.stdout

    @staticmethod
    def pair(fake, real):
        fake = ipaddress.IPv4Address(fake)
        real = ipaddress.IPv6Address(real)
        return fake, real

    def mappings(self):
        """Return only host mappings. Reject malformed/ambiguous output."""
        output = self.command('eamt', 'display', '--csv')
        result = {}
        reverse = {}
        for number, row in enumerate(csv.reader(io.StringIO(output))):
            if not row:
                continue
            if number == 0 and row == ['IPv6 Prefix', 'IPv4 Prefix']:
                continue
            if len(row) != 2:
                raise MappingError('Unexpected EAMT CSV row: {!r}'.format(row))
            try:
                networks = [ipaddress.ip_network(item.strip()) for item in row]
            except ValueError as error:
                raise MappingError('Invalid EAMT CSV row: {!r}'.format(row)) from error
            v4 = next((n for n in networks if n.version == 4), None)
            v6 = next((n for n in networks if n.version == 6), None)
            if v4 is None or v6 is None:
                raise MappingError('EAMT row has incompatible families')
            # Infrastructure prefix entries are not destination records.
            if v4.prefixlen != 32 or v6.prefixlen != 128:
                continue
            fake, real = v4.network_address, v6.network_address
            if fake in result or real in reverse:
                raise MappingError('Duplicate EAMT destination mapping')
            result[fake] = real
            reverse[real] = fake
        return result

    def add(self, fake, real):
        fake, real = self.pair(fake, real)
        with self.lock:
            existing = self.mappings()
            if fake in existing:
                if existing[fake] == real:
                    return
                raise MappingError('Synthetic address already belongs to another target')
            if real in existing.values():
                raise MappingError('IPv6 target already has a synthetic address')
            try:
                self.command('eamt', 'add', str(fake) + '/32', str(real) + '/128')
            except (OSError, subprocess.TimeoutExpired, MappingError):
                # A timed-out command may have succeeded. Only an exact readback
                # can turn this into success; otherwise keep the address reserved.
                if self.mappings().get(fake) == real:
                    return
                raise
            if self.mappings().get(fake) != real:
                raise MappingError('EAMT installation could not be confirmed')

    def remove(self, fake, real):
        fake, real = self.pair(fake, real)
        with self.lock:
            existing = self.mappings()
            if fake not in existing:
                return
            if existing[fake] != real:
                raise MappingError('Refusing to remove another destination')
            try:
                self.command('eamt', 'remove', str(fake) + '/32', str(real) + '/128')
            except (OSError, subprocess.TimeoutExpired, MappingError):
                if fake not in self.mappings():
                    return
                raise
            if fake in self.mappings():
                raise MappingError('EAMT deletion could not be confirmed')

    def save(self, path, subnet):
        subnet = ipaddress.IPv4Network(subnet)
        with self.lock:
            entries = self.mappings()
            if any(fake not in subnet for fake in entries):
                raise MappingError('EAMT contains destinations outside this exit pool')
            path = Path(path)
            path.parent.mkdir(parents=True, exist_ok=True)
            fd, temporary = tempfile.mkstemp(prefix=path.name + '.', dir=path.parent)
            try:
                with os.fdopen(fd, 'w', encoding='ascii') as output:
                    for fake, real in sorted(entries.items()):
                        output.write('{} {}\n'.format(fake, real))
                    output.flush()
                    os.fsync(output.fileno())
                os.replace(temporary, path)
            finally:
                if os.path.exists(temporary):
                    os.unlink(temporary)

    @classmethod
    def read_file(cls, path, subnet):
        """Validate the complete file before changing the kernel table."""
        subnet = ipaddress.IPv4Network(subnet)
        result = {}
        targets = set()
        with open(path, encoding='ascii') as source:
            for line in source:
                if not line.strip():
                    continue
                fields = line.split()
                if len(fields) != 2:
                    raise MappingError('Invalid NAT46 persistence record')
                fake, real = cls.pair(*fields)
                if (fake not in subnet or fake in (subnet.network_address,
                        subnet.broadcast_address, subnet.network_address + 1)
                        or fake in result or real in targets):
                    raise MappingError('Conflicting or reserved NAT46 record')
                result[fake] = real
                targets.add(real)
        return result


class HostPolicy:
    """Atomic, lazy reload; invalid edits fail closed for IPv6 queries."""
    def __init__(self, directory):
        self.directory = Path(directory)
        self.lock = threading.Lock()
        self.signature = None
        self.rules = None

    @staticmethod
    def load(path):
        import re
        rules = []
        for line in path.read_text(encoding='utf-8').splitlines():
            line = line.strip()
            if not line or line.startswith('#'):
                continue
            if line.startswith('/') and line.endswith('/') and len(line) > 2:
                rules.append(re.compile(line[1:-1], re.IGNORECASE))
            else:
                name = line.rstrip('.').encode('idna').decode('ascii').lower()
                if not name or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789-.' for c in name):
                    raise ValueError('Invalid IPv6 hostname rule: ' + line)
                rules.append(name)
        return rules

    @staticmethod
    def matches(rules, name):
        return any((name == rule or name.endswith('.' + rule)) if isinstance(rule, str)
                   else rule.search(name) is not None for rule in rules)

    def selected(self, name):
        name = name.rstrip('.').encode('idna').decode('ascii').lower()
        with self.lock:
            paths = [self.directory / (kind + '-hosts-ipv6-custom.txt')
                     for kind in ('include', 'exclude')]
            signature = [(p.stat().st_mtime_ns, p.stat().st_size) for p in paths]
            if signature != self.signature:
                rules = [self.load(p) for p in paths]
                self.rules, self.signature = rules, signature
            include, exclude = self.rules
            return self.matches(include, name) and not self.matches(exclude, name)
