# Local verification — 2026-11-06

## Environment

- Intel macOS, Lima 2.3.0 with VZ, Ubuntu 24.04 guest, 2 CPU / 2 GiB RAM.
- Guest kernel: 6.8.0-147-generic, matching the inspected exit hosts.
- Jool 4.1.15 built/installed via DKMS; reported module/client version 4.1.15.0.
- Guest Docker Engine: 29.1.3; local Swarm has one node. The host Docker Desktop daemon separately builds
  project images and runs Python/AdGuard tests.
- Real AdGuard Home tested: v0.107.80.

## Passed

1. Jool module compilation/loading and live EAMT operations.
2. Two separate IPv4 clients, identical source port 40000, simultaneous TCP
   and UDP/443 through one translated IPv6 source; NAT resolves port collisions.
3. Adding an EAMT destination while an established TCP connection remains open.
4. ICMP echo and IPv6 MTU 1280: 1200-byte ping payload succeeds, DF payload
   1320 yields IPv4 fragmentation-needed with MTU 1260.
5. Original IPv4 DNAT remains functional with explicit source EAMT and no pool6.
6. Encrypted IPv4-only Swarm overlay + additional macvlan + host-side NAT66.
7. Swarm packet test without NAT66, with an explicit routed-source ULA fixture.
8. Repository resolver: controlled DoH -> synthetic A -> actual IPv6 TCP.
9. Production nat46-control.py export, kernel instance deletion/recreation,
   restoration before DNS; old synthetic A still reaches its original target.
10. Two actual QUIC/443 clients echo 73728 bytes each with reduced egress MTU.
11. Host preparation script: repeated apply and smoke Swarm task receive the
    reserved IPv6 ::2 address. Tested using a simulated guest uplink/default route.
12. Real AdGuard: exit-internal AAAA permitted; ordinary AAAA suppressed; A,
    exit selection and resolver exclusion rules preserved. Configuration script
    enables/disables idempotently.
13. Full antizapret and AdGuard images build. Antizapret build passes 81 Python
    tests and its Go checks. Existing IPv4 resolver tests remain passing.
14. Compose merge -> project compose2swarm -> docker stack config validation;
    networks, NET_ADMIN, explicit image tags, stop-first and 1-minute stop grace
    survive conversion. Feature remains disabled in the base configuration.

## Boundaries

No deployment, package installation or configuration mutations on remote nodes.
Images are local, not pushed. Tests do not establish external provider IPv6,
real VPN ingress, full production CoreDNS/AdGuard filter datasets, two-node
Swarm behavior, reboot/systemd ordering or performance with large EAMTs.
The QUIC test is stream echo, not browser HTTP/3 interoperability.
Mapping persistence does not preserve conntrack or active transport sessions.
Only linux/amd64 builds were tested.

Detailed transient build and packet logs are under ignored tmp/nat46-vm/.
Reproduction: README.md in this directory. Deployment: docs/guide_Jool_RU.md.
