#!/usr/bin/env python3
"""Prepare the container namespace only; the host IPv6 channel is operator-owned."""
import ipaddress
import os
from pathlib import Path
import socket
import stat
import subprocess
import sys
from nat46 import DNAT6Backend


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True).stdout


def ensure_rule(binary, table, chain, *rule):
    args = [binary, '-w', '-t', table]
    if subprocess.run([*args, '-C', chain, *rule], capture_output=True).returncode:
        run(*args, '-A', chain, *rule)


def main(action):
    enabled = os.getenv('NAT46_ENABLED', '0')
    if enabled not in ('0', '1'):
        raise ValueError('NAT46_ENABLED must be 0 or 1')
    if enabled == '0':
        return
    backend = DNAT6Backend(os.getenv('NAT46_PREFIX', 'fd46:64::/96'))
    subnet = ipaddress.IPv4Network(os.environ['AZ_SUBNET'])
    state = Path('/root/antizapret/iptables') / (socket.gethostname().split('.')[0] + '.nat46.rules')
    persistent = os.getenv('IPTABLES_SAVE_DISABLED') != '1'
    if action == 'save':
        if persistent:
            backend.save(state, subnet)
        return
    if action == 'check':
        pid = int(Path('/dev/shm/.nat46.pid').read_text())
        os.kill(pid, 0)
        run('ip', 'link', 'show', 'nat46')
        backend.mappings()
        return
    if action != 'start':
        raise ValueError('Expected start, save or check')
    if os.getenv('WARP_ENABLED', '0') == '1':
        raise ValueError('NAT46 with WARP is not supported')
    for path in ('/proc/sys/net/ipv4/ip_forward', '/proc/sys/net/ipv6/conf/all/forwarding'):
        if Path(path).read_text().strip() != '1':
            raise ValueError('Set forwarding through container sysctls: ' + path)
    gateway = str(ipaddress.IPv4Address(os.environ['NAT46_IPV4_GATEWAY']))
    if subnet.overlaps(ipaddress.IPv4Network('192.0.2.0/24')):
        raise ValueError('AZ_SUBNET conflicts with TAYGA internal addresses')
    if backend.prefix.overlaps(ipaddress.IPv6Network('fd46:65::/64')):
        raise ValueError('NAT46_PREFIX conflicts with TAYGA internal addresses')
    entries = backend.read_file(state, subnet) if persistent and state.exists() else {}
    for real in entries.values():
        if real.version == 6:
            backend.validate_target(real)
    run('ip', '-4', 'route', 'replace', 'default', 'via', gateway)
    run('ip', '-6', 'route', 'get', '2606:4700:4700::1111')
    tun = Path('/dev/net/tun')
    tun.parent.mkdir(parents=True, exist_ok=True)
    if not tun.exists():
        os.mknod(tun, stat.S_IFCHR | 0o600, os.makedev(10, 200))
    info = tun.stat()
    if not stat.S_ISCHR(info.st_mode) or info.st_rdev != os.makedev(10, 200):
        raise ValueError('Unexpected /dev/net/tun')
    conf = Path('/dev/shm/nat46/tayga.conf')
    conf.parent.mkdir(parents=True, exist_ok=True)
    conf.write_text('tun-device nat46\nipv4-addr 192.0.2.1\nipv6-addr fd46:65::1\nprefix {}\n'.format(backend.prefix))
    run('tayga', '-c', str(conf), '--mktun')
    run('ip', 'link', 'set', 'nat46', 'up')
    run('ip', '-4', 'addr', 'replace', '192.0.2.2/32', 'dev', 'nat46')
    run('ip', '-6', 'addr', 'replace', 'fd46:65::2/128', 'dev', 'nat46')
    run('ip', '-4', 'route', 'replace', str(subnet), 'dev', 'nat46')
    run('ip', '-6', 'route', 'replace', str(backend.prefix), 'dev', 'nat46')
    for table, chain in (('nat', 'nat46'), ('filter', 'nat46_unmapped')):
        run('ip6tables', '-w', '-t', table, '-N', chain)
    ensure_rule('ip6tables', 'nat', 'PREROUTING', '-i', 'nat46', '-j', 'nat46')
    ensure_rule('ip6tables', 'nat', 'POSTROUTING', '-s', str(backend.prefix), '!', '-o', 'nat46', '-j', 'MASQUERADE')
    encoded_pool = ipaddress.IPv6Network((int(backend.encoded(subnet.network_address)), 96 + subnet.prefixlen))
    ensure_rule('ip6tables', 'filter', 'nat46_unmapped', '-j', 'DROP')
    ensure_rule('ip6tables', 'filter', 'FORWARD', '-i', 'nat46', '-d', str(encoded_pool), '-j', 'nat46_unmapped')
    if persistent and state.exists():
        # The combined snapshot is authoritative, including an empty IPv4 table.
        rules = ['*nat', '-F dnsmap']
        rules.extend('-A dnsmap -d {} -j DNAT --to-destination {}'.format(fake, real)
                     for fake, real in entries.items() if real.version == 4)
        subprocess.run(['iptables-restore', '--wait', '--noflush'],
                       input='\n'.join([*rules, 'COMMIT', '']), text=True, check=True)
    for fake, real in entries.items():
        if real.version == 6:
            backend.add(fake, real)


if __name__ == '__main__':
    try:
        main(sys.argv[1])
    except Exception as error:
        print('NAT46: ' + str(error), file=sys.stderr)
        sys.exit(1)
