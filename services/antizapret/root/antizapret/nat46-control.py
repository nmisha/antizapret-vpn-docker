#!/usr/bin/env python3
"""Container-local setup/export. Never loads modules or changes host networking."""
import ipaddress
import os
from pathlib import Path
import subprocess
import sys

from nat46 import JoolBackend


def run(*args):
    subprocess.run(args, check=True, timeout=15)


def main():
    if os.getenv('NAT46_ENABLED', '0') != '1':
        return
    instance = os.getenv('NAT46_INSTANCE', 'antizapret')
    subnet = ipaddress.IPv4Network(os.environ['AZ_SUBNET'])
    path = Path('/root/antizapret/iptables') / (os.environ['HOSTNAME'] + '.nat46.rules')
    backend = JoolBackend(instance)
    if sys.argv[1] == 'save':
        if os.getenv('IPTABLES_SAVE_DISABLED') != '1':
            backend.save(path, subnet)
        return
    if sys.argv[1] != 'start':
        raise ValueError('Expected start or save')
    prefix = ipaddress.IPv6Network(os.environ['NAT46_SOURCE_PREFIX'])
    if prefix.prefixlen != 96:
        raise ValueError('NAT46_SOURCE_PREFIX must be /96')
    if os.getenv('NAT46_NAT66_ENABLED', '1') not in ('0', '1'):
        raise ValueError('NAT46_NAT66_ENABLED must be 0 or 1')
    gateway = ipaddress.IPv4Address(os.environ['NAT46_IPV4_GATEWAY'])
    clients = [ipaddress.IPv4Network(item.strip()) for item in
               os.environ['NAT46_CLIENT_SUBNETS'].split(';') if item.strip()]
    if not clients or any(n.overlaps(subnet) or n.prefixlen == 32 for n in clients):
        raise ValueError('Client ranges must be nonempty, not /32 and outside the synthetic pool')
    restored = {}
    if os.getenv('IPTABLES_SAVE_DISABLED') != '1' and path.exists():
        restored = backend.read_file(path, subnet)
    # All local settings must be valid before creating an active instance.
    run('ip', 'route', 'replace', 'default', 'via', str(gateway))
    run('ip', '-6', 'route', 'get', '2606:4700:4700::1111')
    run('sysctl', '-w', 'net.ipv6.conf.all.forwarding=1')
    run('jool_siit', 'instance', 'add', instance, '--netfilter')
    for network in clients:
        source = ipaddress.IPv6Network((int(prefix.network_address) |
                                       int(network.network_address), 96 + network.prefixlen))
        backend.command('eamt', 'add', str(network), str(source))
    for fake, real in restored.items():
        backend.add(fake, real)


if __name__ == '__main__':
    main()
