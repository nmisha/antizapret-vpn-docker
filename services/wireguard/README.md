# WireGuard and AmneziaWG

The Dockerfile and base Compose use upstream wg-easy 15.5.0, AmneziaWG tools
and Docktinel. The image is `nmisha/antizapret-vpn-wireguard:6.12.2`; build and
publish it before deployment. The routes binary and shared lifecycle helpers
come from the upstream antizapret runtime image.

- Routes and optional Bird BGP are supervised by Docktinel.
- `WG_INTERFACE` is validated and used in PostUp/PostDown. Database updates target
  the existing interface row, allowing wg-easy to rename it after initialization.
- A failed or invalid public IPv4 lookup preserves the saved endpoint host.
- `WG_MTU` takes precedence over `MTU`; the fallback is 1280.
- The fork retains the subnet-wide `az_clients` policy generated from
  `WG_ALLOWED_IPS`, with established connections allowed and other destinations
  rejected. It is not a per-client policy. IPv4 ICMP echo requests are dropped
  in mangle FORWARD. The built-in wg-easy per-client firewall is disabled at each
  startup because this policy replaces it.
- Both WireGuard and AmneziaWG use the same lifecycle, with separate data volumes.

## Verification

```sh
python -m unittest discover -s services/wireguard/tests -v
```

Tests use real SQLite migrations and iptables in disposable containers with
isolated networking, no published ports and no persistent volumes. They check
renamed-interface hooks, hook cleanup, MTU, Amnezia settings, route-change
checksums and endpoint preservation. The Node entrypoint is intercepted; the
wg-easy web server and VPN are not started. Set `WIREGUARD_TEST_IMAGE` to choose
a compatible local image. The default dependency image contains wg-easy 15.4
migrations, so these checks do not replace building the new 15.5 image and
verifying its database migration, UI and real WireGuard/AmneziaWG connections.
