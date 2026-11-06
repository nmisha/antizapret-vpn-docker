"""Host helper smoke test, only inside the marked disposable VM."""
import json
import os
from pathlib import Path
import subprocess


def run(*args, check=True):
    return subprocess.run(args, check=check, capture_output=True, text=True).stdout.strip()


config = Path('/etc/default/antizapret-nat46')
if os.geteuid() or not Path('/etc/antizapret-nat46-test-vm').exists():
    raise SystemExit('Disposable marked VM, as root, only')
if config.exists() or Path('/sys/class/net/az46-parent').exists():
    raise SystemExit('Host helper resources already exist')
if run('ip', '-6', 'route', 'show', 'default'):
    raise SystemExit('Test expects no preexisting IPv6 default route')
network = 'n46-host-helper-config'
if network in run('docker', 'network', 'ls', '--format', '{{.Name}}').splitlines():
    raise SystemExit('Reserved test network exists')
helper = Path(__file__).with_name('nat46-egress-host.sh')
previous = run('sysctl', '-n', 'net.ipv6.conf.all.forwarding')
try:
    run('ip', 'link', 'add', 'n46-host-wan', 'type', 'dummy')
    run('ip', 'link', 'set', 'n46-host-wan', 'up')
    run('ip', '-6', 'addr', 'add', 'fd46:47::1/64', 'dev', 'n46-host-wan', 'nodad')
    run('ip', '-6', 'route', 'add', 'default', 'dev', 'n46-host-wan')
    run('sysctl', '-w', 'net.ipv6.conf.all.forwarding=1')
    config.write_text('NAT46_WAN_INTERFACE=n46-host-wan\nNAT46_NETWORK=n46-host-helper\n')
    for _ in range(2):
        run('bash', str(helper), 'apply')
    rules = run('ip6tables', '-t', 'nat', '-S', 'AZ_NAT46')
    assert rules.count('MASQUERADE') == 1, rules
    assert 'fd46:ee::/64' in rules
    hooks = run('ip6tables', '-t', 'nat', '-S', 'POSTROUTING')
    assert hooks.count('-j AZ_NAT46') == 1
    assert 'fd46:ee::1/64' in run('ip', '-6', 'addr', 'show', 'dev', 'az46-shim')
    assert not run('ip', '-6', 'route', 'show', 'fd46:64::/96')
    # Mismatched IPAM refuses reuse; no-NAT requires a routed public subnet.
    for extra in ('NAT46_EGRESS_SUBNET=fd46:ef::/64\nNAT46_EGRESS_GATEWAY=fd46:ef::1\n',
                  'NAT46_HOST_NAT66=0\n'):
        config.write_text('NAT46_WAN_INTERFACE=n46-host-wan\nNAT46_NETWORK=n46-host-helper\n' + extra)
        result = subprocess.run(['bash', str(helper), 'apply'], capture_output=True, text=True)
        assert result.returncode != 0
    print('Host helper: setup/idempotence/NAT66/subnet guards OK; no client /96 route')
finally:
    config.unlink(missing_ok=True)
    run('docker', 'network', 'rm', network, check=False)
    run('ip6tables', '-t', 'nat', '-D', 'POSTROUTING', '-j', 'AZ_NAT46', check=False)
    run('ip6tables', '-t', 'nat', '-F', 'AZ_NAT46', check=False)
    run('ip6tables', '-t', 'nat', '-X', 'AZ_NAT46', check=False)
    for chain in ('FORWARD', 'DOCKER-USER'):
        run('ip6tables', '-D', chain, '-j', 'AZ_NAT46_FWD', check=False)
    run('ip6tables', '-F', 'AZ_NAT46_FWD', check=False)
    run('ip6tables', '-X', 'AZ_NAT46_FWD', check=False)
    for interface in ('az46-shim', 'az46-parent', 'n46-host-wan'):
        run('ip', 'link', 'del', interface, check=False)
    run('sysctl', '-w', 'net.ipv6.conf.all.forwarding=' + previous, check=False)
