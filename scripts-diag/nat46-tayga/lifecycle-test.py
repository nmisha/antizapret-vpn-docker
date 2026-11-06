"""Run the full production /init.sh on a local disposable Docker network."""
import json
import os
from pathlib import Path
import subprocess
import time

D = os.getenv('DOCKER', '/Applications/Docker.app/Contents/Resources/bin/docker')
IMAGE = os.getenv('NAT46_IMAGE', 'antizapret-tayga:local')
NETWORK = 'n46-init-test'
NAME = 'n46-init'


def d(*args):
    result = subprocess.run([D, *args], capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError(str(args) + result.stdout + result.stderr)
    return (result.stdout + result.stderr if args[0] == 'logs' else result.stdout).strip()


if NETWORK in d('network', 'ls', '--format', '{{.Name}}').splitlines():
    raise SystemExit('Reserved lifecycle network exists')
if d('ps', '-aq', '--filter', 'name=^/' + NAME + '$'):
    raise SystemExit('Reserved lifecycle container exists')
try:
    d('network', 'create', '--ipv6', '--subnet', '172.29.46.0/24', '--subnet', 'fd46:49::/64', NETWORK)
    for enabled in ('0', '1'):
        for failure in (False, True):
            if enabled == '0' and failure:
                continue
            d('run', '-d', '--name', NAME, '--hostname', NAME, '--network', NETWORK,
              '--cap-add', 'NET_ADMIN', '--cap-add', 'MKNOD',
              '--sysctl', 'net.ipv4.ip_forward=1', '--sysctl', 'net.ipv6.conf.all.forwarding=1',
              '-e', 'NAT46_ENABLED=' + enabled, '-e', 'NAT46_IPV4_GATEWAY=172.29.46.1',
              '-e', 'DOALL_DISABLED=1', '-e', 'AZ_SUBNET=14.18.0.0/29',
              '-e', 'ASN_FILES=/root/antizapret/result/asn.txt', IMAGE)
            time.sleep(.5)
            for _ in range(100):
                result = subprocess.run([D, 'exec', NAME, 'test', '-f', '/dev/shm/.dns_started'], capture_output=True)
                if result.returncode == 0:
                    break
                if not json.loads(d('inspect', NAME))[0]['State']['Running']:
                    raise RuntimeError('Init died: ' + d('logs', NAME))
                time.sleep(.1)
            else:
                raise RuntimeError('DNS did not start: ' + d('logs', NAME))
            if enabled == '1':
                d('exec', '-e', 'NAT46_ENABLED=1', NAME, 'python3', '/root/antizapret/nat46-control.py', 'check')
            if failure:
                d('exec', NAME, 'bash', '-c', 'kill -TERM "$(cat /dev/shm/.nat46.pid)"')
                for _ in range(50):
                    if not json.loads(d('inspect', NAME))[0]['State']['Running']:
                        break
                    time.sleep(.1)
                else:
                    raise RuntimeError('PID1 did not exit after TAYGA death')
                assert json.loads(d('inspect', NAME))[0]['State']['ExitCode'] != 0
            else:
                d('stop', '-t', '10', NAME)
                assert json.loads(d('inspect', NAME))[0]['State']['ExitCode'] == 0
            log = d('logs', NAME)
            if enabled == '1':
                # Explicit controller save appears in shutdown log.
                assert 'nat46-control.py save' in log
            print('Production init: NAT46=' + enabled + (' translator death handled' if failure else ' start/TERM OK'))
            d('rm', NAME)
finally:
    subprocess.run([D, 'rm', '-f', NAME], capture_output=True)
    subprocess.run([D, 'network', 'rm', NETWORK], capture_output=True)
