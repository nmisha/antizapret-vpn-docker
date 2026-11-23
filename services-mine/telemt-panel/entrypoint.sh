#!/bin/sh
set -eu
umask 077

config=/etc/telemt-panel/config.toml
if [ ! -e "$config" ]; then
  domain=${TELEMT_PANEL_DOMAIN:?TELEMT_PANEL_DOMAIN is required}
  case "$domain" in *[!A-Za-z0-9.-]*|'') echo 'Invalid TELEMT_PANEL_DOMAIN' >&2; exit 1 ;; esac
  # telemt creates a quoted auth_header in [server.api]. Wait for first startup.
  token=''
  attempt=0
  while [ "$attempt" -lt 30 ]; do
    if [ -r /etc/telemt-source/config.toml ]; then
      token=$(awk '
        /^[[:space:]]*\[/ { api=($0 ~ /^[[:space:]]*\[server.api\][[:space:]]*(#.*)?$/) }
        api && /^[[:space:]]*auth_header[[:space:]]*=/ {
          if (match($0, /"[A-Za-z0-9._-]+"/)) {
            print substr($0, RSTART+1, RLENGTH-2); exit
          }
        }
      ' /etc/telemt-source/config.toml)
      [ -z "$token" ] || break
    fi
    attempt=$((attempt + 1))
    sleep 2
  done
  [ -n "$token" ] || { echo 'Cannot read telemt API token; expected a quoted A-Za-z0-9._- token in [server.api]' >&2; exit 1; }
  mkdir -p /etc/telemt-panel /var/lib/telemt-panel
  tmp=$(mktemp /etc/telemt-panel/config.toml.XXXXXX)
  trap 'rm -f "$tmp"' EXIT
  cat > "$tmp" <<EOF
listen = "0.0.0.0:8080"
public_url = "https://$domain"
trusted_proxies = ["10.44.42.0/24"]
data_dir = "/var/lib/telemt-panel"
[tls]
mode = "http"
[telemt]
url = "http://telemt:9091"
auth_header = "$token"
[auth]
disabled = true
[store]
driver = "sqlite"
path = "/var/lib/telemt-panel/panel.db"
[subpage]
enabled = false
[host]
service_manager = "none"
[privileges]
mode = "manual"
EOF
  /usr/local/bin/telemt-panel config check --config "$tmp"
  mv "$tmp" "$config"
  trap - EXIT
  echo 'Created panel config for Authelia authentication'
fi

exec /usr/local/bin/telemt-panel --config "$config"
