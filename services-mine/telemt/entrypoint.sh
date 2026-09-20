#!/bin/sh
set -eu

CONF=/etc/telemt/config.toml
UID_GID=65532:65532
rand_hex() { head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

mkdir -p /etc/telemt

# First start: create config with a random API token, the first user and the public domain
if [ ! -f "$CONF" ]; then
  user=${TELEMT_ADMIN_USER:-admin}
  case $user in *[!A-Za-z0-9_.-]*) echo "TELEMT_ADMIN_USER: allowed chars are A-Za-z0-9_.-" >&2; exit 1 ;; esac
  domain=${TELEMT_DOMAIN:-}
  [ -n "$domain" ] || { echo "TELEMT_DOMAIN is required (the SNI domain the https container routes to this service)" >&2; exit 1; }
  case $domain in *[!A-Za-z0-9_.-]*) echo "TELEMT_DOMAIN: allowed chars are A-Za-z0-9_.-" >&2; exit 1 ;; esac
  sed -e "s/__API_TOKEN__/$(rand_hex 24)/" \
      -e "s/__ADMIN_USER__/$user/" \
      -e "s/__ADMIN_SECRET__/$(rand_hex 16)/" \
      -e "s/__TLS_DOMAIN__/$domain/g" \
      /opt/config.example.toml > "$CONF"
  echo "created $CONF (user: $user, domain: $domain)"
fi

# Optional fixed API token from the environment (keeps it out of the config-reading path of telemt-users.sh)
if [ -n "${TELEMT_API_TOKEN:-}" ]; then
  case $TELEMT_API_TOKEN in *[!A-Za-z0-9._-]*) echo "TELEMT_API_TOKEN: allowed chars are A-Za-z0-9._-" >&2; exit 1 ;; esac
  sed -i "s/^auth_header *=.*/auth_header = \"$TELEMT_API_TOKEN\"/" "$CONF"
fi

# telemt runs as nonroot and rewrites config.toml via temp file + rename inside this directory
chown -R "$UID_GID" /etc/telemt
chmod 600 "$CONF"

exec su-exec "$UID_GID" /app/telemt "$@"
