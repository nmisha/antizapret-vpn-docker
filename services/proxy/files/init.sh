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


AUTHELIA_SERVICE_NAME="auth"

generate_authelia_proxy() {
    if [ "$IS_SELF_SIGNED" -eq 1 ]; then
        cat <<EOF >>"$CONFIG_FILE"

#Authelia#
:9091 {
  tls $CERT_CRT $CERT_KEY

 reverse_proxy {
   to http://authelia:9091
 }

  # Все запросы к /auth/* идут в контейнер authelia:9091 (без /auth)
  # handle_path /auth/* {
  #   reverse_proxy http://authelia:9091
  # }

  # # Всё остальное: редирект на /auth (если хочешь)
  # handle {
  #   redir /auth 302
  # }

  log {
    output file /var/log/caddy/authelia-access.log {
      roll_size 10MB
      roll_keep 5
    }
  }
}

EOF
    else
        cat <<EOF >>"$CONFIG_FILE"



# #Authelia /auth#
# https://$PROXY_DOMAIN:443 {

# #  reverse_proxy {
# #    to http://authelia:9091
# #  }


#      redir /$AUTHELIA_SERVICE_NAME /$AUTHELIA_SERVICE_NAME/ # Just to redirect users that are missing the closing slash to the correct page
#      handle_path /$AUTHELIA_SERVICE_NAME/* { # Actually configures the used subfolder (also internally strips the path prefix)
#          reverse_proxy http://authelia:9091 { # Enables the reverse proxy for the configured program:port
#              header_up X-Forwarded-Prefix "/$AUTHELIA_SERVICE_NAME" # Sets the correct header for the login cookies
#          }
#      }



#   log {
#     output file /var/log/caddy/authelia-access.log {
#       roll_size 10MB
#       roll_keep 5
#     }
#   }
# }

# #reverse_proxy /app2/*

#Authelia#
https://$PROXY_DOMAIN:9091 {

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
    fi
      echo "[INFO] Authelia proxy block added."

#echo "$CONFIG_FILE"
#echo cat "$CONFIG_FILE"

}

add_services_to_config_subnames_test() {
    if [ "$IS_SELF_SIGNED" -eq 1 ]; then
        echo ":443 {" >>"$CONFIG_FILE"
        echo "  tls $CERT_CRT $CERT_KEY" >>"$CONFIG_FILE"
    else
        echo "https://$PROXY_DOMAIN {" >>"$CONFIG_FILE"
    fi
    echo "" >>"$CONFIG_FILE"

    # Authelia (доступ по /auth и /auth/*)
    cat <<EOF >>"$CONFIG_FILE"
  # Authelia web
# #  handle_path /auth/* {
#   handle_path /auth* {
#     reverse_proxy http://authelia:9091
# #    header_up Host {host}
# #    header_up X-Forwarded-Prefix /auth
#   }

#redir /old.html /new.html

  # 1. Проксируем всё, связанное с Authelia
  @authelia {
    path /auth* /api/* /static/* /assets/*
  }
  reverse_proxy @authelia http://authelia:9091 {
    header_up Host {host}
    # необязательно, но полезно
    header_up X-Forwarded-Prefix /auth
  }


  handle_path /srv1* {
    forward_auth authelia:9091 {
#      uri /auth/api/authz/forward-auth
      uri /api/authz/forward-auth
     header_up Host {host}
     header_up X-Forwarded-Prefix /auth
      copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
    }
    reverse_proxy http://dashboard.antizapret:80
  }

  # Всё остальное
  # handle /* {
  #   reverse_proxy http://dashboard.antizapret:80
  # }


EOF

}





add_services_to_config_subnames() {
    if [ "$IS_SELF_SIGNED" -eq 1 ]; then
        echo ":443 {" >>"$CONFIG_FILE"
        echo "  tls $CERT_CRT $CERT_KEY" >>"$CONFIG_FILE"
    else
        echo "https://$PROXY_DOMAIN {" >>"$CONFIG_FILE"
    fi
    echo "" >>"$CONFIG_FILE"

    # Authelia (доступ по /auth и /auth/*)
    cat <<EOF >>"$CONFIG_FILE"
#   # Authelia web
#   handle_path /auth/* {
# #  handle_path /auth* {
#     reverse_proxy http://authelia:9091
#   }

  # 1. Проксируем всё, связанное с Authelia
  @authelia {
    path /auth* /api/* /static/* /assets/*
  }
  reverse_proxy @authelia http://authelia:9091 {
    header_up Host {host}
    # необязательно, но полезно
    header_up X-Forwarded-Prefix /auth
  }

EOF

    # Счётчик subpath
    idx=1
    default_subpath="srv1"

    echo "$REACHABLE_SERVICES" | while IFS= read -r service_value; do
        [ -z "$service_value" ] && continue

        name=$(echo "$service_value" | cut -d':' -f1)
        internal_host=$(echo "$service_value" | cut -d':' -f3)
        internal_port=$(echo "$service_value" | cut -d':' -f4)
        subpath="srv$idx"

        cat <<EOF >>"$CONFIG_FILE"
  # $name → /$subpath/
#   handle_path /$subpath/* {
#     forward_auth authelia:9091 {
#       uri /api/authz/forward-auth
# #      auth/uri /api/authz/forward-auth
#       copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
#     }
#     reverse_proxy http://$internal_host:$internal_port
#   }

#========================================

#   handle_path /$subpath* {
#     forward_auth authelia:9091 {
# #      uri /auth/api/authz/forward-auth
#       uri /api/authz/forward-auth
#      header_up Host {host}
#      header_up X-Forwarded-Prefix /auth
#       copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
#     }
#     reverse_proxy http://$internal_host:$internal_port
#   }

#========================================

  # 1. Ловим все запросы к /app и автоматически отрезаем этот префикс
  handle_path /$subpath/* {
    # Если нужно, прокидываем оригинальный Host и X-Forwarded-Prefix,
    # чтобы upstream-приложение знало, где оно “сидит”
    reverse_proxy http://$internal_host:$internal_port {
      header_up Host {host}
      header_up X-Forwarded-Prefix /$subpath
    }
  }

  # 2. (Опционально) редирект с /app на /app/ — чтобы нормально работали относительные пути
  @bareApp {
    path /$subpath
  }
  redir @bareApp /$subpath/ 301




EOF
        idx=$((idx+1))
    done

    # Дефолтный редирект на /srv1/
    cat <<EOF >>"$CONFIG_FILE"
  handle {
    redir /$default_subpath/ 302
  }

  log {
    output file /var/log/caddy/access.log {
      roll_size 10MB
      roll_keep 5
    }
  }
}
EOF

    echo "[INFO] Caddyfile with numbered subpaths and Authelia created."
}


add_services_to_config_subnames_2() { 
    if [ "$IS_SELF_SIGNED" -eq 1 ]; then
        echo ":443 {" >>"$CONFIG_FILE"
        echo "  tls $CERT_CRT $CERT_KEY" >>"$CONFIG_FILE"
    else
        echo "https://$PROXY_DOMAIN {" >>"$CONFIG_FILE"
    fi
    echo "" >>"$CONFIG_FILE"

    # Authelia (доступ по /auth и /auth/*)
    cat <<EOF >>"$CONFIG_FILE"
  @authelia {
    path /auth* /api/* /static/* /assets/*
  }
  reverse_proxy @authelia http://authelia:9091 {
    header_up Host {host}
    header_up X-Forwarded-Prefix /auth
  }

EOF

    idx=1
    default_subpath="srv1"

    echo "$REACHABLE_SERVICES" | while IFS= read -r service_value; do
        [ -z "$service_value" ] && continue

        name=$(echo "$service_value" | cut -d':' -f1)
        internal_host=$(echo "$service_value" | cut -d':' -f3)
        internal_port=$(echo "$service_value" | cut -d':' -f4)
        subpath="srv$idx"
        bare_marker="@bareApp_$subpath"

        cat <<EOF >>"$CONFIG_FILE"
  # $name → /$subpath/
  handle_path /$subpath/* {
      forward_auth authelia:9091 {
      uri /auth/api/authz/forward-auth
#      uri /api/authz/forward-auth
      copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
      }
    reverse_proxy http://$internal_host:$internal_port {
      header_up Host {host}
      header_up X-Forwarded-Prefix /$subpath
    }
  }



  $bare_marker {
    path /$subpath
  }
  redir $bare_marker /$subpath/ 301

EOF
        idx=$((idx+1))
    done

    cat <<EOF >>"$CONFIG_FILE"
  handle {
    redir /$default_subpath/ 302
  }

  log {
    output file /var/log/caddy/access.log {
      roll_size 10MB
      roll_keep 5
    }
  }
}
EOF

    echo "[INFO] Caddyfile with numbered subpaths and Authelia created."
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

	forward_auth authelia:9091 {
		uri /api/authz/forward-auth
		copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
#    trusted_proxies private_ranges
	}

  reverse_proxy {
    to http://$internal_host:$internal_port
  }






#   @auth_exempt path /auth* /favicon.ico /api/verify /locales/*

#   @protected {
#     not path /auth* /favicon.ico /api/verify /locales/*
#   }

#   route @protected {
#     forward_auth authelia:9091 {
# #    forward_auth auth.vps-nl-1.20x40.ru:9091 {
# #    forward_auth az.vps-nl-1.20x40.ru:9091 {
#       uri /api/verify?rd=https://{host}{uri}
#       copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
#       trusted_proxies private_ranges
#     }
#     reverse_proxy http://$internal_host:$internal_port
#   }

#   reverse_proxy @auth_exempt authelia:9091
# #  reverse_proxy /auth* auth.vps-nl-1.20x40.ru:9091

}
EOF
    else
        cat <<EOF >>"$CONFIG_FILE"

#$name#
https://$PROXY_DOMAIN:$external_port {
#  reverse_proxy {
#    to http://$internal_host:$internal_port
#  }
# #  basicauth {
# #    $PROXY_USERNAME $PROXY_PASSWORD
# #  }



	forward_auth authelia:9091 {
		uri /api/authz/forward-auth
		copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
#    trusted_proxies private_ranges
	}

  reverse_proxy {
    to http://$internal_host:$internal_port
  }



#   @auth_exempt path /auth* /favicon.ico /api/verify /locales/*

#   @protected {
#     not path /auth* /favicon.ico /api/verify /locales/*
#   }

#   route @protected {
#     forward_auth authelia:9091 {
# #    forward_auth auth.vps-nl-1.20x40.ru:9091 {
# #    forward_auth az.vps-nl-1.20x40.ru:9091 {
#       uri /api/verify?rd=https://{host}{uri}
#       copy_headers Remote-User Remote-Groups Remote-Name Remote-Email
#       trusted_proxies private_ranges
#     }
#     reverse_proxy http://$internal_host:$internal_port
#   }

#   reverse_proxy @auth_exempt authelia:9091
# #  reverse_proxy /auth* auth.vps-nl-1.20x40.ru:9091

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


#echo "$CONFIG_FILE"
#echo cat "$CONFIG_FILE"

}


main() {
    : >"$CONFIG_FILE"
    get_services

    if [ -z "${PROXY_DOMAIN:-}" ] || [ -z "${PROXY_EMAIL:-}" ]; then
        IS_SELF_SIGNED=1
        generate_certificate
        generate_global_config
#        generate_authelia_proxy

    else
        IS_SELF_SIGNED=0
        generate_global_config
#        generate_authelia_proxy
    fi

#    generate_authelia_proxy   # add authelia proxy
#    add_services_to_config
    add_services_to_config_subnames_2
#    add_services_to_config_subnames_test

    echo
    echo "[INFO] Caddyfile has been successfully created at: $CONFIG_FILE"
}

main
