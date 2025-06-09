#!/bin/sh

set -eu

CERT_DIR="/data/caddy/certificates/self-signed"
CERT_CRT="$CERT_DIR/selfsigned.crt"
CERT_KEY="$CERT_DIR/selfsigned.key"
CONFIG_FILE="/etc/caddy/Caddyfile"
REACHABLE_SERVICES=""
IS_SELF_SIGNED=0


is_host_resolved() {
    sleep 1s
    host=$1
    if getent hosts "$host" >/dev/null; then
        return 0
    else
        return 1
    fi
}

generate_certificate() {
    echo "[INFO] Generating or checking SSL certificates..."

    mkdir -p "$CERT_DIR"
    if [ ! -f "$CERT_KEY" ] || [ ! -f "$CERT_CRT" ]; then
        openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
          -keyout "$CERT_KEY" \
          -out "$CERT_CRT" \
          -subj "/O=ANTIZAPRET/OU=ANTIZAPRET/CN=ANTIZAPRET"
        echo "[INFO] Certificates have been generated."
    else
        echo "[INFO] Certificates already exist. Skipping generation."
    fi
    echo
}

get_services() {
    COUNTER=1
    while :; do
        service_var="PROXY_SERVICE_$COUNTER"
        service_value=$(eval echo "\${$service_var:-}")

        if [ -z "$service_value" ]; then
            break
        fi

        name=$(echo "$service_value" | cut -d':' -f1)
        external_port=$(echo "$service_value" | cut -d':' -f2)
        internal_host=$(echo "$service_value" | cut -d':' -f3)
        internal_port=$(echo "$service_value" | cut -d':' -f4)

        if [ -z "$name" ] || [ -z "$external_port" ] || [ -z "$internal_host" ] || [ -z "$internal_port" ]; then
            echo "[ERROR] $service_var has an invalid format. Expected: name:external_port:internal_hostname:internal_port"
            exit 1
        fi

        if is_host_resolved "$internal_host"; then
            REACHABLE_SERVICES=$(printf "%s\n%s" "$REACHABLE_SERVICES" "$service_value")
            echo "[INFO] Host $internal_host is reachable. Adding service: $service_value"
        else
            echo "[WARNING] Host $internal_host is not reachable. Skipping: $service_value"
        fi

        COUNTER=$((COUNTER + 1))
    done
    echo "[INFO] Services read successfully."
}

generate_global_config() {
    if [ "$IS_SELF_SIGNED" -eq 1 ]; then
        cat <<EOF >>"$CONFIG_FILE"
{
  auto_https disable_redirects
}
EOF
    else
        cat <<EOF >>"$CONFIG_FILE"
{
  email $PROXY_EMAIL
  auto_https disable_redirects
}
EOF
    fi
    echo "[INFO] Global configuration block created."
}

add_services_to_config() {
    echo "$REACHABLE_SERVICES" | while IFS= read -r service_value; do

    if [ -z "$service_value" ]; then
        continue
    fi

    name=$(echo "$service_value" | cut -d':' -f1)
    external_port=$(echo "$service_value" | cut -d':' -f2)
    internal_host=$(echo "$service_value" | cut -d':' -f3)
    internal_port=$(echo "$service_value" | cut -d':' -f4)

    if [ "$IS_SELF_SIGNED" -eq 1 ]; then
        cat <<EOF >>"$CONFIG_FILE"

#$name#
:$external_port {
  tls $CERT_CRT $CERT_KEY
#  reverse_proxy {
#    to http://$internal_host:$internal_port
#  }

  @auth_exempt path /auth* /favicon.ico /api/verify /locales/*

  @protected {
    not path /auth* /favicon.ico /api/verify /locales/*
  }

  route @protected {
    forward_auth authelia:9091 {
#    forward_auth auth.vps-nl-1.20x40.ru:9091 {
#    forward_auth az.vps-nl-1.20x40.ru:9091 {
      uri /api/verify?rd=https://{host}{uri}
      copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
      trusted_proxies private_ranges
    }
    reverse_proxy http://$internal_host:$internal_port
  }

  reverse_proxy @auth_exempt authelia:9091
#  reverse_proxy /auth* auth.vps-nl-1.20x40.ru:9091

}
EOF
    else
        cat <<EOF >>"$CONFIG_FILE"

#$name#
https://$PROXY_DOMAIN:$external_port {
#  reverse_proxy {
#    to http://$internal_host:$internal_port
#  }
##  basicauth {
##    $PROXY_USERNAME $PROXY_PASSWORD
##  }

  @auth_exempt path /auth* /favicon.ico /api/verify /locales/*

  @protected {
    not path /auth* /favicon.ico /api/verify /locales/*
  }

  route @protected {
    forward_auth authelia:9091 {
#    forward_auth auth.vps-nl-1.20x40.ru:9091 {
#    forward_auth az.vps-nl-1.20x40.ru:9091 {
      uri /api/verify?rd=https://{host}{uri}
      copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
      trusted_proxies private_ranges
    }
    reverse_proxy http://$internal_host:$internal_port
  }

  reverse_proxy @auth_exempt authelia:9091
#  reverse_proxy /auth* auth.vps-nl-1.20x40.ru:9091

  log {
    output file /var/log/caddy/access.log {
      roll_size 10MB # Create new file when size exceeds 10MB
      roll_keep 5 # Keep at most 5 rolled files
#      roll_keep_days 14 # Delete files older than 14 days
    }
  }
}
EOF
    fi
      echo "[INFO] Service added: $external_port -> $internal_host:$internal_port"
    done
}


generate_authelia_proxy() {
    if [ "$IS_SELF_SIGNED" -eq 1 ]; then
        cat <<EOF >>"$CONFIG_FILE"

## Authelia web
## :9191 {
:9091 {
#   tls $CERT_CRT $CERT_KEY
#   reverse_proxy authelia:9091
# }
# EOF
#     else
#         cat <<EOF >>"$CONFIG_FILE"

# Authelia web
#https://auth.$PROXY_DOMAIN:9191 {
#https://$PROXY_DOMAIN:$external_port {
#https://$PROXY_DOMAIN:9091 {
  tls $CERT_CRT $CERT_KEY

  @authelia_path path /auth*  # только путь /auth* будет проксироваться

#  reverse_proxy authelia:9091
  reverse_proxy @authelia_path authelia:9091
  {
    header_up Host {host}
    header_up X-Real-IP {remote}
    header_up X-Forwarded-For {remote}
    header_up X-Forwarded-Proto {scheme}
  }

  # Всё остальное — редирект на /auth
  handle {
    @not_auth not path /auth*
    redir /auth 302
  }

  log {
    output file /var/log/caddy/authelia-access.log {
      roll_size 10MB
      roll_keep 5
    }
  }
}
EOF
    fi
    echo "[INFO] Authelia proxy block added."
}



main() {
    : >"$CONFIG_FILE"
    get_services

    if [ -z "${PROXY_DOMAIN:-}" ] || [ -z "${PROXY_EMAIL:-}" ]; then
        IS_SELF_SIGNED=1
        generate_certificate
        generate_global_config
    else
        IS_SELF_SIGNED=0
        generate_global_config
    fi

    add_services_to_config
    generate_authelia_proxy   # add authelia proxy

    echo
    echo "[INFO] Caddyfile has been successfully created at: $CONFIG_FILE"
}

main


#https://auth.$PROXY_DOMAIN:9191 {
#    reverse_proxy authelia:9091
#    ...
#}
