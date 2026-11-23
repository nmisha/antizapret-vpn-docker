# ocserv

Uses the upstream ocserv 1.6.0 build and Docktinel lifecycle. The Compose image
is `nmisha/antizapret-vpn-ocserv:6.12.2`; build and publish it before deployment.
The routes binary and shared Docktinel helpers come from the upstream
`xtrime/antizapret-vpn:6.12.2` runtime image.

HTTPS exports `certificate.crt`, `certificate.key`, `identity` and `identity.type`
to the shared ocserv certificate directory. Certificate/key pairs are checked
with the shared certificate cache. Changes to certificates or generated IP lists
fail the configuration check; Docktinel handles recovery instead of healthcheck
scripts killing PID 1. Routes are supervised with `watch_routes --vpn`.

The fork retains the `az_clients` chain: destinations are limited to the IPv4
routes from the global and group configurations. This is a server-wide union,
not a separate policy for each group. Established connections remain allowed.
Client ICMP echo requests are blocked; other ICMP traffic remains available.
The custom `Nextcloud Panel` camouflage realm is retained.

Existing editable templates in the mounted `/etc/ocserv` directory are preserved;
changing bundled templates does not overwrite existing deployment templates.

## Verification

```sh
python -m unittest discover -s services/ocserv/tests -v
```

Checks use current scripts mounted in an existing image with OpenSSL, ocserv and
iptables, in an isolated Docker network namespace with no published ports or
persistent volumes. Set `OCSERV_TEST_IMAGE` to choose another image. These checks
do not replace building and testing the new ocserv 1.6.0 image or a real VPN login.
