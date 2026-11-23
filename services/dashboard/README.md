# Dashboard

The container uses the upstream Docktinel startup and health checks.

- `DASHBOARD_AUTH_ENABLED=1` enables Basic Auth (script default).
- The supplied Compose file defaults to `DASHBOARD_AUTH_ENABLED=0` for use behind Authelia. Set it to `1` and configure `DASHBOARD_USERNAME` / `DASHBOARD_PASSWORD` to enable Basic Auth.
- `DASHBOARD_AUTH_URL=https://auth.example.com` makes the AZ title open the authentication portal. If unset, it links to the upstream repository.
- `DASHBOARD_SERVICE_N=name:external_port:internal_hostname:internal_port` defines consecutively numbered services.
- `DASHBOARD_SERVICE_URL_N=https://service.example.com` overrides the external URL.
- `DASHBOARD_SERVICE_MODE_N=external` opens a new tab instead of an iframe; the default is `iframe`.

Build the modified image before deployment; an existing registry tag does not contain local changes.
