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
WEB_CERT_CRT="$WEB_CERT_DIR/certificate.crt"
WEB_CERT_KEY="$WEB_CERT_DIR/certificate.key"
WEB_FALLBACK_CRT="$WEB_CERT_DIR/fallback.crt"
WEB_FALLBACK_KEY="$WEB_CERT_DIR/fallback.key"
WEB_IDENTITY_FILE="$WEB_CERT_DIR/identity"
WEB_TYPE_FILE="$WEB_CERT_DIR/identity.type"
CERT_IDENTITY_FILE="$OCSERV_CERT_DIR/identity"
CERT_TYPE_FILE="$OCSERV_CERT_DIR/identity.type"
CONFIG_FILE="/etc/caddy/Caddyfile"
SITES_ENABLED_DIR="/config/sites-enabled"
REACHABLE_SERVICES=""
SNI_ROUTES=""
SNI_CERTIFICATES=""
SNI_DEFAULT_UPSTREAM=""
SNI_DEFAULT_PORT=""
SNI_DEFAULT_PROXY_PROTOCOL=""
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

validate_hostname() {
    case "$1" in
        ""|*[!A-Za-z0-9._-]*) return 1 ;;
    esac
}

validate_certificate_directory() {
    case "$1" in
        /|""|*/|*//*|*[!A-Za-z0-9_./-]*|*/../*|*/..|*/./*|*/.) return 1 ;;
        /*) return 0 ;;
        *) return 1 ;;
    esac
}

normalize_proxy_protocol() {
    case "$1" in
        none) printf '%s\n' "none" ;;
        proxy-v1|v1) printf '%s\n' "v1" ;;
        proxy-v2|v2) printf '%s\n' "v2" ;;
        *) return 1 ;;
    esac
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

get_sni_routes() {
    if ! validate_port "$HTTPS_PORT"; then
        echo "[ERROR] Invalid PROXY_HTTPS_PORT: $HTTPS_PORT" >&2
        exit 1
    fi

    seen_sni_names=""
    counter=1
    while :; do
        route_var="SNI_ROUTE_$counter"
        eval "route_value=\${$route_var:-}"

        if [ -z "$route_value" ]; then
            break
        fi

        IFS=: read -r sni_name upstream_host upstream_port proxy_protocol remainder <<EOF
$route_value
EOF
        sni_name=$(normalize_domain "$sni_name")
        if ! validate_hostname "$sni_name" || ! validate_hostname "$upstream_host" \
            || ! validate_port "$upstream_port" || [ -n "$remainder" ]; then
            echo "[ERROR] $route_var has an invalid format. Expected: sni:upstream_hostname:upstream_port:none|proxy-v1|proxy-v2" >&2
            exit 1
        fi
        if ! proxy_protocol=$(normalize_proxy_protocol "$proxy_protocol"); then
            echo "[ERROR] $route_var has an invalid PROXY protocol mode: $proxy_protocol. Expected: none, proxy-v1 or proxy-v2" >&2
            exit 1
        fi
        case " $seen_sni_names " in
            *" $sni_name "*)
                echo "[ERROR] Duplicate SNI name in $route_var: $sni_name" >&2
                exit 1
                ;;
        esac
        seen_sni_names="$seen_sni_names $sni_name"
        SNI_ROUTES=$(printf "%s\n%s:%s:%s:%s" "$SNI_ROUTES" \
            "$sni_name" "$upstream_host" "$upstream_port" "$proxy_protocol")
        SNI_ROUTING=1

        counter=$((counter + 1))
    done

    default_value="${SNI_DEFAULT_ROUTE:-}"
    if [ -n "$default_value" ]; then
        IFS=: read -r upstream_host upstream_port proxy_protocol remainder <<EOF
$default_value
EOF
        if ! validate_hostname "$upstream_host" || ! validate_port "$upstream_port" \
            || [ -n "$remainder" ]; then
            echo "[ERROR] SNI_DEFAULT_ROUTE has an invalid format. Expected: upstream_hostname:upstream_port:none|proxy-v1|proxy-v2" >&2
            exit 1
        fi
        if ! proxy_protocol=$(normalize_proxy_protocol "$proxy_protocol"); then
            echo "[ERROR] SNI_DEFAULT_ROUTE has an invalid PROXY protocol mode: $proxy_protocol. Expected: none, proxy-v1 or proxy-v2" >&2
            exit 1
        fi
        SNI_DEFAULT_UPSTREAM="$upstream_host"
        SNI_DEFAULT_PORT="$upstream_port"
        SNI_DEFAULT_PROXY_PROTOCOL="$proxy_protocol"
        SNI_ROUTING=1
    fi
}

get_sni_certificates() {
    seen_identities=""
    seen_directories=""
    counter=1
    while :; do
        certificate_var="SNI_CERT_$counter"
        eval "certificate_value=\${$certificate_var:-}"

        if [ -z "$certificate_value" ]; then
            break
        fi

        IFS=: read -r identity output_directory remainder <<EOF
$certificate_value
EOF
        if validate_ipv4 "$identity"; then
            identity_type="ip"
        else
            identity=$(normalize_domain "$identity")
            identity_type="dns"
        fi
        if ! validate_hostname "$identity" || ! validate_certificate_directory "$output_directory" \
            || [ -n "$remainder" ]; then
            echo "[ERROR] $certificate_var has an invalid format. Expected: identity:/absolute/output/directory" >&2
            exit 1
        fi
        case "$output_directory" in
            "$WEB_CERT_DIR"|"$OCSERV_CERT_DIR")
                echo "[ERROR] $certificate_var must not use reserved directory: $output_directory" >&2
                exit 1
                ;;
        esac
        case " $seen_identities " in
            *" $identity "*)
                echo "[ERROR] Duplicate SNI certificate identity in $certificate_var: $identity" >&2
                exit 1
                ;;
        esac
        case " $seen_directories " in
            *" $output_directory "*)
                echo "[ERROR] Duplicate SNI certificate directory in $certificate_var: $output_directory" >&2
                exit 1
                ;;
        esac
        seen_identities="$seen_identities $identity"
        seen_directories="$seen_directories $output_directory"
        SNI_CERTIFICATES=$(printf "%s\n%s:%s:%s" "$SNI_CERTIFICATES" \
            "$identity" "$identity_type" "$output_directory")
        counter=$((counter + 1))
    done
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
        if validate_ipv4 "$PROXY_HOST"; then
            PROXY_HOST_TYPE="ip"
        else
            PROXY_HOST=$(normalize_domain "$PROXY_HOST")
            PROXY_HOST_TYPE="dns"
        fi
        if ! validate_hostname "$PROXY_HOST"; then
            echo "[ERROR] Invalid PROXY_DOMAIN: $PROXY_HOST" >&2
            exit 1
        fi
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
    if ! validate_hostname "$CERT_IDENTITY"; then
        echo "[ERROR] Invalid OCSERV_DOMAIN: $CERT_IDENTITY" >&2
        exit 1
    fi

    if [ "$SNI_ROUTING" -eq 1 ] && [ "$HTTPS_PORT" -eq 443 ]; then
        echo "[ERROR] PROXY_HTTPS_PORT must differ from 443 when SNI routing is configured" >&2
        exit 1
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
    printf '%s\n' "$PROXY_HOST" > "$WEB_IDENTITY_FILE.tmp"
    printf '%s\n' "$PROXY_HOST_TYPE" > "$WEB_TYPE_FILE.tmp"
    mv -f "$CERT_IDENTITY_FILE.tmp" "$CERT_IDENTITY_FILE"
    mv -f "$CERT_TYPE_FILE.tmp" "$CERT_TYPE_FILE"
    mv -f "$WEB_IDENTITY_FILE.tmp" "$WEB_IDENTITY_FILE"
    mv -f "$WEB_TYPE_FILE.tmp" "$WEB_TYPE_FILE"

    echo "$SNI_CERTIFICATES" | while IFS= read -r certificate_value; do
        if [ -z "$certificate_value" ]; then
            continue
        fi
        IFS=: read -r identity identity_type output_directory <<EOF
$certificate_value
EOF
        generate_fallback_certificate \
            "$identity" "$identity_type" \
            "$output_directory/fallback.crt" "$output_directory/fallback.key"
        printf '%s\n' "$identity" > "$output_directory/identity.tmp"
        printf '%s\n' "$identity_type" > "$output_directory/identity.type.tmp"
        mv -f "$output_directory/identity.tmp" "$output_directory/identity"
        mv -f "$output_directory/identity.type.tmp" "$output_directory/identity.type"
    done
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
EOF
    if [ "$SNI_ROUTING" -eq 1 ]; then
        cat <<EOF >>"$CONFIG_FILE"
  layer4 {
    :443 {
EOF
        route_counter=1
        echo "$SNI_ROUTES" | while IFS= read -r route_value; do
            if [ -z "$route_value" ]; then
                continue
            fi
            IFS=: read -r sni_name upstream_host upstream_port proxy_protocol <<EOF
$route_value
EOF
            cat <<EOF >>"$CONFIG_FILE"
      @sni_route_$route_counter tls sni $sni_name
      route @sni_route_$route_counter {
        proxy {
EOF
            if [ "$proxy_protocol" != "none" ]; then
                printf '          proxy_protocol %s\n' "$proxy_protocol" >> "$CONFIG_FILE"
            fi
            cat <<EOF >>"$CONFIG_FILE"
          upstream $upstream_host:$upstream_port
        }
      }

EOF
            route_counter=$((route_counter + 1))
        done
        if [ -n "$SNI_DEFAULT_UPSTREAM" ]; then
            cat <<EOF >>"$CONFIG_FILE"
      route {
        proxy {
EOF
            if [ "$SNI_DEFAULT_PROXY_PROTOCOL" != "none" ]; then
                printf '          proxy_protocol %s\n' "$SNI_DEFAULT_PROXY_PROTOCOL" >> "$CONFIG_FILE"
            fi
            cat <<EOF >>"$CONFIG_FILE"
          upstream $SNI_DEFAULT_UPSTREAM:$SNI_DEFAULT_PORT
        }
      }
EOF
        fi
        cat <<EOF >>"$CONFIG_FILE"
    }
  }
EOF
    fi
    cat <<EOF >>"$CONFIG_FILE"
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
    write_tls_policy "$PROXY_HOST" "$WEB_CERT_CRT" "$WEB_CERT_KEY"
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
        write_tls_policy "$PROXY_HOST" "$WEB_CERT_CRT" "$WEB_CERT_KEY"
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
    if echo "$SNI_ROUTES" | grep -q "^$PROXY_HOST:127\.0\.0\.1:$HTTPS_PORT:"; then
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
    if [ "$CERT_IDENTITY" = "$PROXY_HOST" ] && [ "$HAS_CERT_SITE" -eq 1 ]; then
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

add_sni_certificate_sites() {
    echo "$SNI_CERTIFICATES" | while IFS= read -r certificate_value; do
        if [ -z "$certificate_value" ]; then
            continue
        fi
        IFS=: read -r identity identity_type output_directory <<EOF
$certificate_value
EOF
        if [ "$identity" = "$PROXY_HOST" ] || [ "$identity" = "$CERT_IDENTITY" ]; then
            continue
        fi

        cat <<EOF >>"$CONFIG_FILE"

#SNI certificate automation: $identity#
$identity:$HTTPS_PORT {
EOF
        write_tls_policy \
            "$identity" \
            "$output_directory/certificate.crt" \
            "$output_directory/certificate.key"
        cat <<EOF >>"$CONFIG_FILE"
  respond 204
}
EOF
    done
}

main() {
    mkdir -p "$SITES_ENABLED_DIR"
    : >"$CONFIG_FILE"
    get_sni_routes
    resolve_certificate_identity
    get_sni_certificates
    generate_fallback_certificates
    get_services
    generate_global_config
    add_http_redirect


    generate_authelia_proxy   # add authelia proxy
    add_services_to_config
    add_ocserv_certificate_site
    add_sni_certificate_sites

    cat <<EOF >>"$CONFIG_FILE"

import $SITES_ENABLED_DIR/*
EOF

    echo
    echo "[INFO] Caddyfile has been successfully created at: $CONFIG_FILE"
    echo "[INFO] ocserv certificate identity: $CERT_TYPE:$CERT_IDENTITY"
    if [ "$SNI_ROUTING" -eq 1 ]; then
        echo "[INFO] Port 443 SNI routing configured"
    fi
    if [ -z "$SNI_DEFAULT_UPSTREAM" ]; then
        echo "[INFO] Port 443 unmatched SNI connections are not routed"
    fi
}

main
