"""TAYGA uses a static prefix; destination mappings live in kernel DNAT6."""
import ipaddress
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import threading


class MappingError(RuntimeError):
    """Reserve the synthetic address until ambiguous backend state is reconciled."""


class DNAT6Backend:
    def __init__(self, prefix='fd46:64::/96', chain='nat46', runner=None):
        self.prefix = ipaddress.IPv6Network(prefix)
        if self.prefix.prefixlen != 96:
            raise ValueError('NAT46_PREFIX must be /96')
        self.chain = chain
        self.runner = runner or subprocess.run
        self.lock = threading.RLock()

    def command(self, args, **kwargs):
        result = self.runner(args, capture_output=True, text=True, timeout=10, **kwargs)
        if result.returncode:
            raise MappingError(result.stderr.strip() or 'DNAT6 command failed')
        return result.stdout

    @staticmethod
    def pair(fake, real):
        return ipaddress.IPv4Address(fake), ipaddress.IPv6Address(real)

    def validate_target(self, real):
        real = ipaddress.IPv6Address(real)
        if real in self.prefix or real in ipaddress.IPv6Network('fd46:65::/64'):
            raise MappingError('IPv6 target conflicts with translator internal routes')

    def encoded(self, fake):
        return ipaddress.IPv6Address(int(self.prefix.network_address) | int(ipaddress.IPv4Address(fake)))

    def rule(self, fake, real):
        return ['-d', str(self.encoded(fake)) + '/128', '-j', 'DNAT', '--to-destination', str(ipaddress.IPv6Address(real))]

    def mappings(self):
        result, targets = {}, set()
        output = self.command(['ip6tables', '-w', '-t', 'nat', '-S', self.chain])
        for line in output.splitlines():
            row = shlex.split(line)
            if row == ['-N', self.chain]:
                continue
            if (len(row) != 8 or row[:3] != ['-A', self.chain, '-d']
                    or row[4:7] != ['-j', 'DNAT', '--to-destination']):
                raise MappingError('Unexpected rule in owned DNAT6 chain')
            encoded = ipaddress.IPv6Network(row[3])
            if encoded.prefixlen != 128 or encoded.network_address not in self.prefix:
                raise MappingError('DNAT6 destination outside translation prefix')
            fake = ipaddress.IPv4Address(int(encoded.network_address) & 0xffffffff)
            real = ipaddress.IPv6Address(row[7])
            self.validate_target(real)
            if fake in result or real in targets:
                raise MappingError('Conflicting DNAT6 mapping')
            result[fake] = real
            targets.add(real)
        return result

    def add(self, fake, real):
        fake, real = self.pair(fake, real)
        self.validate_target(real)
        with self.lock:
            existing = self.mappings()
            if fake in existing:
                if existing[fake] == real:
                    return
                raise MappingError('Synthetic address already belongs to another target')
            if real in existing.values():
                raise MappingError('IPv6 target already has a synthetic address')
            rule = ' '.join(['-A', self.chain, *self.rule(fake, real)])
            try:
                self.command(['ip6tables-restore', '--wait', '--noflush'], input='*nat\n' + rule + '\nCOMMIT\n')
            except (OSError, subprocess.TimeoutExpired, MappingError):
                if self.mappings().get(fake) == real:
                    return
                raise
            if self.mappings().get(fake) != real:
                raise MappingError('DNAT6 installation could not be confirmed')

    def remove(self, fake, real):
        fake, real = self.pair(fake, real)
        with self.lock:
            existing = self.mappings()
            if fake not in existing:
                return
            if existing[fake] != real:
                raise MappingError('Refusing to delete another target')
            try:
                self.command(['ip6tables', '-w', '-t', 'nat', '-D', self.chain, *self.rule(fake, real)])
            except (OSError, subprocess.TimeoutExpired, MappingError):
                if fake not in self.mappings():
                    return
                raise
            if fake in self.mappings():
                raise MappingError('DNAT6 deletion could not be confirmed')

    def ipv4_mappings(self):
        entries = {}
        output = self.command(['iptables', '-w', '-t', 'nat', '-S', 'dnsmap'])
        for line in output.splitlines():
            row = shlex.split(line)
            if row == ['-N', 'dnsmap']:
                continue
            if (len(row) != 8 or row[:3] != ['-A', 'dnsmap', '-d']
                    or row[4:7] != ['-j', 'DNAT', '--to-destination']):
                raise MappingError('Unexpected rule in owned IPv4 chain')
            fake = ipaddress.IPv4Network(row[3])
            real = ipaddress.IPv4Address(row[7])
            if fake.prefixlen != 32 or fake.network_address in entries:
                raise MappingError('Conflicting IPv4 mapping')
            entries[fake.network_address] = real
        return entries

    def save(self, path, subnet):
        subnet = ipaddress.IPv4Network(subnet)
        with self.lock:
            entries = self.ipv4_mappings()
            for fake, real in self.mappings().items():
                if fake in entries:
                    raise MappingError('IPv4 and IPv6 share a synthetic address')
                entries[fake] = real
            if any(fake not in subnet for fake in entries):
                raise MappingError('DNAT6 contains destinations outside this exit pool')
            path = Path(path)
            path.parent.mkdir(parents=True, exist_ok=True)
            fd, temporary = tempfile.mkstemp(prefix=path.name + '.', dir=path.parent)
            try:
                with os.fdopen(fd, 'w', encoding='ascii') as output:
                    output.write('# nat46-state-v1\n')
                    for fake, real in sorted(entries.items()):
                        output.write('{} {}\n'.format(fake, real))
                    output.flush()
                    os.fsync(output.fileno())
                os.replace(temporary, path)
                directory_fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
                try:
                    os.fsync(directory_fd)
                finally:
                    os.close(directory_fd)
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
            if source.readline().strip() != '# nat46-state-v1':
                raise MappingError('Unknown NAT46 state format; migrate with stopped exits and expired DNS caches')
            for line in source:
                if not line.strip():
                    continue
                fields = line.split()
                if len(fields) != 2:
                    raise MappingError('Invalid NAT46 persistence record')
                fake, real = ipaddress.IPv4Address(fields[0]), ipaddress.ip_address(fields[1])
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
