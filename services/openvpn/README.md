# OpenVPN

The server and UI use the upstream Docktinel lifecycle. The server Dockerfile
builds OpenVPN 2.7.9 and takes routes/shared helpers from the upstream antizapret
runtime. The server image is `nmisha/antizapret-vpn-openvpn:6.12.3`; the UI uses
`xtrime/antizapret-vpn-openvpn-ui:6.12.2`, matching upstream.

The server supervises routes, follows logs, rotates logs daily with seven-day
retention, and checks CRL renewal daily. CRL renewal requests a Docktinel restart.
The UI prepares configuration before starting, then checks its local login URL.
Public IPv4 discovery has a timeout; set `OPENVPN_EXTERNAL_IP` explicitly when
needed.

The fork retains server-side `az_clients` destination restrictions and blocks
client ICMP echo requests. Allowed destinations include AZ_SUBNET, the Docker
network, the VPN subnet and the route snapshot copied to
`/etc/openvpn/openvpn-blocked-ranges.txt`. This is a subnet-wide policy, not a
per-user policy. Existing connections remain allowed. NIC detection uses
`ip -4 route`, which works with BusyBox. Routes are started only by Docktinel.

Build and publish the modified server image before deployment. Tests do not
replace a build of the new OpenVPN version or a real VPN/UI login.

## Verification

```sh
python -m unittest discover -s services/openvpn/tests -v
```

Tests run current scripts in disposable containers with isolated networking,
no published ports, persistent data or Docker socket. They check firewall rules,
route-change detection, real EasyRSA CRL renewal, and UI database preparation.
The restart request is intercepted; no real container is restarted. UI database
checks use SQLite in a dependency image, with a replacement for sponge; they do
not start the UI application. Override `OPENVPN_TEST_IMAGE` and
`OPENVPN_UI_TEST_IMAGE` to select compatible local dependency images.
