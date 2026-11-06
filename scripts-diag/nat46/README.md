# Isolated Jool/NAT46 checks

These scripts modify **a disposable local VM**, never a production host.
The test runner requires root and the marker installed by provision-vm.sh.
Use an otherwise empty test Docker daemon; the Swarm runner reserves `n46-*`
names and creates host routes/firewall rules. Do not run concurrent test jobs.

Tested environment: Lima/VZ on Intel macOS, Ubuntu 24.04, Linux
6.8.0-147-generic, Jool 4.1.15. A normal Docker Desktop container cannot supply
a module missing from Docker Desktop's kernel.

## Reproduce

With Lima installed (this session uses `tmp/nat46-vm/lima/bin/limactl`):

```sh
limactl start --name nat46 --tty=false scripts-diag/nat46/lima.yaml
limactl copy scripts-diag/nat46/provision-vm.sh nat46:/tmp/provision-vm.sh
limactl shell nat46 bash /tmp/provision-vm.sh
limactl shell nat46 mkdir -p /tmp/nat46-tests
limactl copy scripts-diag/nat46/Dockerfile scripts-diag/nat46/packet-test.py \
  scripts-diag/nat46/dns_integration.py scripts-diag/nat46/quic-test.py \
  services/antizapret/root/antizapret/nat46.py \
  services/antizapret/root/antizapret/nat46-control.py \
  services/antizapret/root/antizapret/dnsmap.py nat46:/tmp/nat46-tests/
limactl shell nat46 sudo docker build -t nat46-jool-test:local /tmp/nat46-tests
```

Initialize Swarm **once** in this VM with its actual interface IPv4 address:

```sh
limactl shell nat46 ip -4 addr
limactl shell nat46 sudo docker swarm init --advertise-addr VM_IPV4
```

Run sequentially:

```sh
# Bridge baseline, live Jool backend and IPv4 DNAT coexistence.
limactl shell nat46 sudo python3 /tmp/nat46-tests/packet-test.py
# Real Swarm task, encrypted IPv4 overlay, separate macvlan, host NAT66,
# repository DNS resolver + checkpoint/startup controller, actual QUIC/443.
limactl shell nat46 sudo env N46_SWARM=1 N46_DNS=1 N46_QUIC=1 \
  python3 /tmp/nat46-tests/packet-test.py
# Optional routed-source mode, without NAT66. Uses an isolated ULA fixture.
limactl shell nat46 sudo env N46_SWARM=1 N46_NAT66=0 \
  python3 /tmp/nat46-tests/packet-test.py
```

The runner deletes its containers/networks/routes/rules in finally. Swarm network
removal is asynchronous; verify previous cleanup before another run. Test-only
host IPv6 forwarding remains enabled in the disposable VM. For failure diagnosis
`N46_KEEP=1` retains resources instead; inspect and remove them before repeating.
Do not retain resources for an ordinary test run.

The runner uses nsenter to call the VM's Jool CLI in the exit namespace. The
DNS integration also runs the repository's Jool backend and control script
inside the container, with the CLI and its libraries copied into it.
A controlled DoH server supplies repeatable A/AAAA answers; this does not test
the complete production CoreDNS/AdGuard configuration or external DNS availability.

The host preparation script is tested separately in the VM using a simulated
IPv6 default route, repeated apply and a smoke Swarm service. Its exact deployment
steps and reserved addresses are in docs/guide_Jool_RU.md. Never simulate routes
on a real exit host.

Stop the VM when finished:

```sh
limactl stop nat46
```

## Findings

- Jool translates in-kernel; NAT66 in the same namespace did not match the
  translated outgoing packets in this setup. Host-side NAT66 passed.
- Swarm macvlan on an internal dummy parent, with a host macvlan shim, delivers
  the routed client source prefix back to Jool. Tested ipvlan did not.
- Swarm rejects IPv6-only network scope. Preserve the original IPv4 default
  gateway explicitly when attaching the additional dual-stack network.
- Configure source EAMT entries plus destination /32→/128 entries, without a
  catch-all pool6; otherwise ordinary IPv4 traffic can be translated as well.
- Real Jool CSV includes `IPv6 Prefix,IPv4 Prefix` as its first row.
- Some Engine/API versions omit IPAM.Config for config-only networks. The host
  script records its creation settings in an ownership/configuration label.


## Real AdGuard filtering/DoH check

On the Mac Docker daemon, after building the project images:

```sh
docker build --build-arg ROUTES_IMAGE=antizapret-nat46:local \
  -t antizapret-adguard-nat46:local services/adguard
python3 scripts-diag/nat46/adguard-test.py
```

The script uses nat46-dns-tests:local (the antizapret Dockerfile's test-dnsmap
stage), a controlled UDP upstream and the real AdGuard binary/configuration
script. It checks internal AAAA, ordinary-client AAAA suppression, existing A,
exit routing/exclusion and configuration enable/disable/idempotence. No ports
are published. Set DOCKER if the CLI is not at Docker Desktop's standard path.

The DNS integration additionally supplies a CNAME-only reply, checks that the
next AAAA lookup succeeds, kills the resolver with SIGKILL after its A answer,
and recreates Jool using only the checkpoint written before that answer.
The test must recover the same synthetic address and establish a new TCP flow.
