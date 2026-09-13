# Domain-based HTTPS services

`PROXY_SERVICE_N=name:external_port:internal_hostname:internal_port` keeps serving
applications on separate ports of `PROXY_DOMAIN`.

For applications on their own domains, configure consecutive variables starting
at 1 (an empty entry ends the list):

```yaml
environment:
  - PROXY_AUTHELIA_DOMAIN=auth.nope.jo3.org
  - PROXY_VHOST_1=2FAuth:twof.auth.nope.jo3.org:2fauth:8000
  # Append after your existing SNI_ROUTE_1 and SNI_ROUTE_2:
  - SNI_ROUTE_3=auth.nope.jo3.org:127.0.0.1:444:proxy-v2
  - SNI_ROUTE_4=twof.auth.nope.jo3.org:127.0.0.1:444:proxy-v2
```

Use the configured `PROXY_HTTPS_PORT` instead of `444` if it differs. Layer 4
passes TLS to the local Caddy HTTP server, which selects the site by hostname.
Public URLs use port 443. When SNI routing is enabled, the generator requires an
explicit local route for every virtual host. Without SNI routing, arrange external
443-to-`PROXY_HTTPS_PORT` forwarding, or use `PROXY_HTTPS_PORT=443`.

`PROXY_AUTHELIA_DOMAIN` replaces the legacy `PROXY_DOMAIN:9091` portal. Leave it
empty to keep that address. The portal has no `forward_auth`; all `PROXY_VHOST_N`
applications have the same Authelia check as port-based applications. Domain names
must be unique and different from `PROXY_DOMAIN` and `OCSERV_DOMAIN`.

Caddy manages domain certificates directly in `/data/caddy` using ACME HTTP-01;
DNS must point to the HTTPS server and TCP ports 80 and 443 must be reachable.
Unlike the legacy web/VPN certificate mechanism, new domain sites in `auto` mode
wait for successful certificate issuance and do not use a fallback certificate.
`PROXY_CERT_MODE=selfsigned` uses per-domain certificates in `/data/vhosts`.
If a domain is also listed in `SNI_CERT_N`, its exported certificate is reused
with the existing fallback/synchronization mechanism; no duplicate `respond 204`
site is emitted. Explicit HTTP redirects preserve the URI and use public port 443.

## Authelia and 2FAuth migration

The local `config-docker-swarm` and `config-mine` directories are ignored by Git.
Copy their updated configuration separately to the deployment nodes; `git pull`
does not transfer these files.

1. Add the HTTPS variables above to the effective Compose override. Remove
   `PROXY_SERVICE_14=2FAuth:10443:2fauth:8000` and published ports `9091:9091`
   and `10443:10443`. Existing `PROXY_SERVICE_1` through `PROXY_SERVICE_13` stay.
2. In `config-mine/authelia/config/configuration.yml`, keep the session cookie
   domain `nope.jo3.org`, set `authelia_url: https://auth.nope.jo3.org`, and keep
   `default_redirection_url: https://m.nope.jo3.org`. Add
   `twof.auth.nope.jo3.org` to the existing `one_factor` rule for `admins` and
   `vpn_users`; keep the default deny policy. This preserves current access
   policy and does not enable automatic account login inside 2FAuth.
3. Set `services.2fauth.environment.APP_URL` to
   `https://twof.auth.nope.jo3.org`. Preserve the existing `APP_KEY` and data.
4. Configure the Dashboard link:

   ```yaml
   - DASHBOARD_SERVICE_11=2FAuth:443:2fauth:8000
   - DASHBOARD_SERVICE_URL_11=https://twof.auth.nope.jo3.org
   ```

   `DASHBOARD_SERVICE_URL_N` overrides the external HTTPS link. Without it,
   Dashboard uses its current host and the service port. Internal Docker links
   retain their existing behavior.

5. Build and distribute new HTTPS and Dashboard images before applying the
   configuration. For Swarm, publish both images under new tags and update the
   effective Compose image references: `docker stack deploy` does not build
   images. Copy the updated Authelia configuration to the node hosting Authelia,
   then deploy through the project's normal Swarm workflow and restart Authelia
   if its bind-mounted configuration change did not recreate its task.
6. Check certificates, login at `https://auth.nope.jo3.org`, the redirect back to
   2FAuth, access to existing `m.nope.jo3.org` services, the Dashboard link and
   ocserv connectivity. Old portal/2FAuth ports are removed, not redirected.

## Local generator checks

```sh
python -m unittest discover -s services/https/tests -v
```

The tests mount current scripts into the existing HTTPS image and run Caddy
validation with no container network, published ports or persistent data.
Set `HTTPS_TEST_IMAGE` to use a different image containing the same Caddy modules.
