#!/bin/sh

set -eu

ACME_CA="${PROXY_ACME_CA:-https://acme-v02.api.letsencrypt.org/directory}"
CERT_MODE="${PROXY_CERT_MODE:-auto}"
HTTPS_PORT="${PROXY_HTTPS_PORT:-444}"
OCSERV_CERT_DIR="/data/ocserv"
CERT_CRT="$OCSERV_CERT_DIR/certificate.crt"
CERT_KEY="$OCSERV_CERT_DIR/certificate.key"
FALLBACK_CRT="$OCSERV_CERT_DIR/fallback.crt"
FALLBACK_KEY="$OCSERV_CERT_DIR/fallback.key"
WEB_CERT_DIR="/data/web"
WEB_FALLBACK_CRT="$WEB_CERT_DIR/fallback.crt"
WEB_FALLBACK_KEY="$WEB_CERT_DIR/fallback.key"
CERT_IDENTITY_FILE="$OCSERV_CERT_DIR/identity"
CERT_TYPE_FILE="$OCSERV_CERT_DIR/identity.type"
CONFIG_FILE="/etc/caddy/Caddyfile"
SITES_ENABLED_DIR="/config/sites-enabled"
REACHABLE_SERVICES=""
CERT_IDENTITY=""
CERT_TYPE=""
PROXY_HOST=""
PROXY_HOST_TYPE=""
SNI_ROUTING=0
HAS_CERT_SITE=0

validate_ipv4() {
    printf '%s\n' "$1" | awk -F. '
        NF != 4 { exit 1 }
        {
            for (i = 1; i <= 4; i++) {
                if ($i !~ /^[0-9]+$/ || $i < 0 || $i > 255) {
                    exit 1
                }
            }
        }
    '
}

validate_port() {
    case "$1" in
        ""|*[!0-9]*) return 1 ;;
    esac
    [ "$1" -ge 1 ] && [ "$1" -le 65535 ]
}

normalize_domain() {
    idn2 --quiet "$1" | tr '[:upper:]' '[:lower:]'
}

detect_public_ipv4() {
    public_ip="${PROXY_IP:-}"
    if [ -z "$public_ip" ]; then
        public_ip=$(curl --ipv4 --fail --silent --show-error \
            --max-time "${IP_CHECK_TIMEOUT:-10}" \
            "${IP_CHECK_URL:-https://api.ipify.org}")
    fi
    public_ip=$(printf '%s' "$public_ip" | tr -d '[:space:]')

    if ! validate_ipv4 "$public_ip"; then
        echo "[ERROR] Invalid public IPv4 address: $public_ip" >&2
        exit 1
    fi

    printf '%s\n' "$public_ip"
}

resolve_certificate_identity() {
    case "$CERT_MODE" in
        auto|selfsigned) ;;
        *)
            echo "[ERROR] Invalid PROXY_CERT_MODE: $CERT_MODE. Expected: auto or selfsigned" >&2
            exit 1
            ;;
    esac

    if ! validate_port "$HTTPS_PORT"; then
        echo "[ERROR] Invalid PROXY_HTTPS_PORT: $HTTPS_PORT" >&2
        exit 1
    fi

    PROXY_HOST="${PROXY_DOMAIN:-}"
    if [ -n "$PROXY_HOST" ]; then
        PROXY_HOST=$(normalize_domain "$PROXY_HOST")
        PROXY_HOST_TYPE="dns"
    else
        PROXY_HOST=$(detect_public_ipv4)
        PROXY_HOST_TYPE="ip"
    fi

    CERT_IDENTITY="${OCSERV_DOMAIN:-$PROXY_HOST}"
    if validate_ipv4 "$CERT_IDENTITY"; then
        CERT_TYPE="ip"
    else
        CERT_IDENTITY=$(normalize_domain "$CERT_IDENTITY")
        CERT_TYPE="dns"
    fi

    if [ "$CERT_IDENTITY" != "$PROXY_HOST" ]; then
        if [ "$CERT_TYPE" != "dns" ] || [ "$PROXY_HOST_TYPE" != "dns" ]; then
            echo "[ERROR] SNI routing requires DNS names in both PROXY_DOMAIN and OCSERV_DOMAIN" >&2
            exit 1
        fi
        if [ "$HTTPS_PORT" -eq 443 ]; then
            echo "[ERROR] PROXY_HTTPS_PORT must differ from 443 when SNI routing is enabled" >&2
            exit 1
        fi
        SNI_ROUTING=1
    fi
}

generate_fallback_certificate() {
    identity="$1"
    identity_type="$2"
    fallback_crt="$3"
    fallback_key="$4"
    mkdir -p "$(dirname "$fallback_crt")"
    if [ "$identity_type" = "ip" ]; then
        identity_check="-checkip"
    else
        identity_check="-checkhost"
    fi
    if ! openssl x509 -in "$fallback_crt" -noout -checkend 86400 \
            "$identity_check" "$identity" >/dev/null 2>&1 \
        || [ ! -s "$fallback_key" ]; then
        [ "$identity_type" = "ip" ] && san="IP:$identity" || san="DNS:$identity"
        echo "[INFO] Generating fallback certificate for $identity_type:$identity"
        openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 365 \
            -subj "/CN=$identity" -addext "subjectAltName=$san" \
            -keyout "$fallback_key.tmp" -out "$fallback_crt.tmp" >/dev/null 2>&1
        chmod 600 "$fallback_key.tmp"
        mv -f "$fallback_key.tmp" "$fallback_key"
        mv -f "$fallback_crt.tmp" "$fallback_crt"
    fi
}

generate_fallback_certificates() {
    mkdir -p "$OCSERV_CERT_DIR"
    generate_fallback_certificate \
        "$CERT_IDENTITY" "$CERT_TYPE" "$FALLBACK_CRT" "$FALLBACK_KEY"
    generate_fallback_certificate \
        "$PROXY_HOST" "$PROXY_HOST_TYPE" "$WEB_FALLBACK_CRT" "$WEB_FALLBACK_KEY"
    printf '%s\n' "$CERT_IDENTITY" > "$CERT_IDENTITY_FILE.tmp"
    printf '%s\n' "$CERT_TYPE" > "$CERT_TYPE_FILE.tmp"
    mv -f "$CERT_IDENTITY_FILE.tmp" "$CERT_IDENTITY_FILE"
    mv -f "$CERT_TYPE_FILE.tmp" "$CERT_TYPE_FILE"
}

get_services() {
    counter=1
    while :; do
        service_var="PROXY_SERVICE_$counter"
        service_value=$(eval echo "\${$service_var:-}")

        if [ -z "$service_value" ]; then
            break
        fi

        IFS=: read -r name external_port internal_host internal_port remainder <<EOF
$service_value
EOF

        if [ -z "$name" ] || [ -z "$internal_host" ] || [ -n "$remainder" ] \
            || ! validate_port "$external_port" || ! validate_port "$internal_port"; then
            echo "[ERROR] $service_var has an invalid format. Expected: name:external_port:internal_hostname:internal_port"
            exit 1
        fi

        if [ "$external_port" -eq "$HTTPS_PORT" ]; then
            HAS_CERT_SITE=1
        fi
        REACHABLE_SERVICES=$(printf "%s\n%s" "$REACHABLE_SERVICES" "$service_value")
        counter=$((counter + 1))
    done
    echo "[INFO] Services read successfully."
}

write_tls_policy() {
    host="$1"
    certificate="$2"
    key="$3"
    if [ "$CERT_MODE" = "selfsigned" ]; then
        cat <<EOF >>"$CONFIG_FILE"
  tls $certificate $key
EOF
        return
    fi

    if validate_ipv4 "$host"; then
        cat <<EOF >>"$CONFIG_FILE"
  tls $certificate $key {
    issuer acme $ACME_CA {
      profile shortlived
      disable_tlsalpn_challenge
    }
  }
EOF
    else
        cat <<EOF >>"$CONFIG_FILE"
  tls $certificate $key {
    issuer acme $ACME_CA {
      disable_tlsalpn_challenge
    }
  }
EOF
    fi
}

generate_global_config() {
    if [ "$CERT_MODE" = "auto" ]; then
        auto_https_mode="ignore_loaded_certs disable_redirects"
    else
        auto_https_mode="disable_certs disable_redirects"
    fi
    cat <<EOF >>"$CONFIG_FILE"
{
  auto_https $auto_https_mode
  default_sni $PROXY_HOST
  http_port 80
  https_port $HTTPS_PORT
  layer4 {
    :443 {
EOF
    if [ "$SNI_ROUTING" -eq 1 ]; then
        cat <<EOF >>"$CONFIG_FILE"
      @web tls sni $PROXY_HOST
      route @web {
        proxy {
          proxy_protocol v2
          upstream 127.0.0.1:$HTTPS_PORT
        }
      }

      @ocserv tls sni $CERT_IDENTITY
      route @ocserv {
        proxy {
          proxy_protocol v2
          upstream ocserv.antizapret:443
        }
      }

EOF
    fi
    cat <<EOF >>"$CONFIG_FILE"
      route {
        proxy {
          proxy_protocol v2
          upstream ocserv.antizapret:443
        }
      }
    }
  }
  servers {
    listener_wrappers {
      http_redirect
      tls
    }
  }
EOF
    if [ "$SNI_ROUTING" -eq 1 ]; then
        cat <<EOF >>"$CONFIG_FILE"
  servers :$HTTPS_PORT {
    listener_wrappers {
      proxy_protocol {
        allow 127.0.0.1/32 ::1/128
        fallback_policy skip
      }
      http_redirect
      tls
    }
    protocols h1 h2
  }
EOF
    fi
    cat <<EOF >>"$CONFIG_FILE"
  servers :80 {
  }
EOF
    if [ -n "${PROXY_EMAIL:-}" ]; then
        printf '  email %s\n' "$PROXY_EMAIL" >> "$CONFIG_FILE"
    fi
    cat <<EOF >>"$CONFIG_FILE"
}
EOF
    echo "[INFO] Global configuration block created."
}

AUTHELIA_SERVICE_NAME="auth"

generate_authelia_proxy() {
    authelia_address="$PROXY_HOST:9091"
    if [ "$PROXY_HOST_TYPE" = "ip" ]; then
        authelia_address="$authelia_address, :9091"
    fi
    if [ "$HTTPS_PORT" -eq 9091 ]; then
        HAS_CERT_SITE=1
    fi

    cat <<EOF >>"$CONFIG_FILE"

#Authelia#
$authelia_address {
EOF
    write_tls_policy "$PROXY_HOST" "$WEB_FALLBACK_CRT" "$WEB_FALLBACK_KEY"
    cat <<EOF >>"$CONFIG_FILE"

  reverse_proxy {
    to http://authelia:9091
  }


  log {
    output file /var/log/caddy/authelia-access.log {
      roll_size 10MB
      roll_keep 5
    }
  }
}

EOF
    echo "[INFO] Authelia proxy block added."

#echo "$CONFIG_FILE"
#echo cat "$CONFIG_FILE"

}

add_services_to_config() {
    echo "$REACHABLE_SERVICES" | while IFS= read -r service_value; do
        if [ -z "$service_value" ]; then
            continue
        fi

        IFS=: read -r name external_port internal_host internal_port <<EOF
$service_value
EOF
        site_address="$PROXY_HOST:$external_port"
        if [ "$PROXY_HOST_TYPE" = "ip" ]; then
            site_address="$site_address, :$external_port"
        fi

        cat <<EOF >>"$CONFIG_FILE"

#$name#
$site_address {
EOF
        write_tls_policy "$PROXY_HOST" "$WEB_FALLBACK_CRT" "$WEB_FALLBACK_KEY"
        cat <<EOF >>"$CONFIG_FILE"
  header {
    -X-Frame-Options
  }

	forward_auth authelia:9091 {
		uri /api/authz/forward-auth
		copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
#    trusted_proxies private_ranges
	}

  reverse_proxy {
    header_up Authorization {http.request.header.Authorization}
    header_up Proxy-Authorization {http.request.header.Proxy-Authorization}
    dynamic a {
      name $internal_host
      port $internal_port
      refresh 1s
    }
  }




  log {
    output file /var/log/caddy/access.log {
      roll_size 10MB # Create new file when size exceeds 10MB
      roll_keep 5 # Keep at most 5 rolled files
#      roll_keep_days 14 # Delete files older than 14 days
    }
  }

}
EOF
        echo "[INFO] Service added: $PROXY_HOST:$external_port -> $internal_host:$internal_port"
    done


#echo "$CONFIG_FILE"
#echo cat "$CONFIG_FILE"
}

add_http_redirect() {
    if [ "$SNI_ROUTING" -eq 1 ]; then
        redirect_target="https://{host}{uri}"
    else
        redirect_target="https://{host}:$HTTPS_PORT{uri}"
    fi
    cat <<EOF >>"$CONFIG_FILE"

#HTTP to Dashboard#
http://$PROXY_HOST, :80 {
  redir $redirect_target 308
}
EOF
}

add_ocserv_certificate_site() {
    if [ "$SNI_ROUTING" -eq 0 ] && [ "$HAS_CERT_SITE" -eq 1 ]; then
        return
    fi

    cat <<EOF >>"$CONFIG_FILE"

#ocserv certificate automation#
$CERT_IDENTITY:$HTTPS_PORT {
EOF
    write_tls_policy "$CERT_IDENTITY" "$CERT_CRT" "$CERT_KEY"
    cat <<EOF >>"$CONFIG_FILE"
  respond 204
}
EOF
}

main() {
    mkdir -p "$SITES_ENABLED_DIR"
    : >"$CONFIG_FILE"
    resolve_certificate_identity
    generate_fallback_certificates
    get_services
    generate_global_config
    add_http_redirect


    generate_authelia_proxy   # add authelia proxy
    add_services_to_config
    add_ocserv_certificate_site

    cat <<EOF >>"$CONFIG_FILE"

import $SITES_ENABLED_DIR/*
EOF

    echo
    echo "[INFO] Caddyfile has been successfully created at: $CONFIG_FILE"
    echo "[INFO] ocserv certificate identity: $CERT_TYPE:$CERT_IDENTITY"
    if [ "$SNI_ROUTING" -eq 1 ]; then
        echo "[INFO] Port 443 SNI routing: $PROXY_HOST -> dashboard, $CERT_IDENTITY -> ocserv"
    fi
}

main
