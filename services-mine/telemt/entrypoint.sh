#!/bin/sh
set -eu
umask 077

CONF=/etc/telemt/config.toml
UID_GID=65532:65532
rand_hex() { head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

mkdir -p /etc/telemt

# Validate before creating or changing a config. Empty means preserve its value.
for limit in "${TELEMT_MAX_TCP_CONNS:-}" "${TELEMT_MAX_UNIQUE_IPS:-}"; do
  [ -n "$limit" ] || continue
  case $limit in
    *[!0-9]*|0*) echo "TELEMT_MAX_TCP_CONNS/TELEMT_MAX_UNIQUE_IPS must be positive integers without leading zeros" >&2; exit 1 ;;
  esac
done

# Change only the default in [access], preserving per-user overrides and secrets.
set_access_limit() {
  key=$1 value=$2
  [ -n "$value" ] || return 0
  awk -v key="$key" -v value="$value" '
    /^[[:space:]]*\[/ {
      if (inside && !written) { print key " = " value; written=1 }
      inside=($0 ~ /^[[:space:]]*\[access\][[:space:]]*(#.*)?$/)
      if (inside) found=1
    }
    inside && $0 ~ "^[[:space:]]*" key "[[:space:]]*=" {
      if (!written) print key " = " value
      written=1
      next
    }
    { print }
    END {
      if (!found) print "\n[access]"
      if (!written) print key " = " value
    }
  ' "$CONF" > "$CONF.tmp"
  mv "$CONF.tmp" "$CONF"
}

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
      /opt/config.example.toml > "$CONF.tmp"
  mv "$CONF.tmp" "$CONF"
  echo "created $CONF (user: $user, domain: $domain)"
fi

# Optional fixed API token from the environment (keeps it out of the config-reading path of telemt-users.sh)
if [ -n "${TELEMT_API_TOKEN:-}" ]; then
  case $TELEMT_API_TOKEN in *[!A-Za-z0-9._-]*) echo "TELEMT_API_TOKEN: allowed chars are A-Za-z0-9._-" >&2; exit 1 ;; esac
  sed -i "s/^auth_header *=.*/auth_header = \"$TELEMT_API_TOKEN\"/" "$CONF"
fi

set_access_limit user_max_tcp_conns_global_each "${TELEMT_MAX_TCP_CONNS:-}"
set_access_limit user_max_unique_ips_global_each "${TELEMT_MAX_UNIQUE_IPS:-}"

# telemt runs as nonroot and rewrites config.toml via temp file + rename inside this directory
chown -R "$UID_GID" /etc/telemt
chmod 600 "$CONF"

# Swarm can mount tmpfs with the image directory's 0755 root:root ownership.
# Prepare the writable runtime directory before dropping privileges.
mkdir -p /run/telemt
chown "$UID_GID" /run/telemt

exec su-exec "$UID_GID" /app/telemt "$@"
