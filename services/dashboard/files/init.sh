#!/bin/sh
set -eu

if [ -z "${SERVER_ROOT:-}" ]; then
    echo "[ERROR] SERVER_ROOT is not defined. Set SERVER_ROOT environment variable."
    exit 1
fi

CONFIG_JSON="$SERVER_ROOT/config.json"
HTPASSWD_FILE="/etc/lighttpd/.htpasswd"
AUTH_CONF_FILE="/etc/lighttpd/conf.d/010-auth.conf"

create_auth() {
    if [ -z "${DASHBOARD_USERNAME:-}" ] || [ -z "${DASHBOARD_PASSWORD:-}" ]; then
        echo "[ERROR] DASHBOARD_USERNAME or DASHBOARD_PASSWORD environment variables are not set."
        echo "Example usage:"
        echo "  export DASHBOARD_USERNAME=user"
        echo "  export DASHBOARD_PASSWORD=secret"
        exit 1
    fi

    htpasswd -cb "$HTPASSWD_FILE" "$DASHBOARD_USERNAME" "$DASHBOARD_PASSWORD"
    echo "[INFO] The file $HTPASSWD_FILE has been successfully created."

    cat <<EOL > "$AUTH_CONF_FILE"
auth.backend = "htpasswd"
auth.backend.htpasswd.userfile = "$HTPASSWD_FILE"
auth.require = (
    "/" => (
        "method" => "basic",
        "realm" => "Restricted Area",
        "require" => "valid-user"
    )
)
EOL
    echo "[INFO] The file $AUTH_CONF_FILE has been successfully created."
}

create_empty_auth() {
    printf '%s\n' '# Authentication disabled' > "$AUTH_CONF_FILE"
}

create_modules() {
    MOD_CONF_FILE="/etc/lighttpd/conf.d/000-modules.conf"
    cat <<EOL > "$MOD_CONF_FILE"
server.modules += ( "mod_setenv" )
EOL
    echo "[INFO] Modules config created: $MOD_CONF_FILE"
}

create_cors() {
    CORS_CONF_FILE="/etc/lighttpd/conf.d/020-cors.conf"
    cat <<EOL > "$CORS_CONF_FILE"
setenv.add-response-header = (
    "Access-Control-Allow-Origin" => "*",
    "Access-Control-Allow-Credentials" => "true"
)
EOL
    echo "[INFO] CORS config created: $CORS_CONF_FILE"
}

create_services_json() {
    services=""
    COUNTER=1

    while :; do
        service_var="DASHBOARD_SERVICE_$COUNTER"
        service_value=$(eval echo "\${$service_var:-}")

        if [ -z "$service_value" ]; then
            break
        fi

        name=$(echo "$service_value" | cut -d':' -f1)
        external_port=$(echo "$service_value" | cut -d':' -f2)
        internal_hostname=$(echo "$service_value" | cut -d':' -f3)
        internal_port=$(echo "$service_value" | cut -d':' -f4)
        url_var="DASHBOARD_SERVICE_URL_$COUNTER"
        eval "external_url=\${$url_var:-}"
        case "$external_url" in
            ""|https://*) ;;
            *) echo "[ERROR] $url_var must be an HTTPS URL" >&2; exit 1 ;;
        esac

        mode_var="DASHBOARD_SERVICE_MODE_$COUNTER"
        eval "open_mode=\${$mode_var:-iframe}"
        case "$open_mode" in
            iframe|external) ;;
            *) echo "[ERROR] $mode_var must be iframe or external" >&2; exit 1 ;;
        esac

        service_json=$(jq -n \
                --arg name "$name" \
                --arg externalPort "$external_port" \
                --arg internalHostname "$internal_hostname" \
                --arg internalPort "$internal_port" \
                --arg externalUrl "$external_url" \
                --arg openMode "$open_mode" \
                '$ARGS.named')

        if [ -n "$services" ]; then
            services="$services,$service_json"
        else
            services="$service_json"
        fi

        echo "[INFO] Added service '$name' to JSON."

        COUNTER=$((COUNTER + 1))
    done

    config=$(jq -n \
        --arg internalHostname "$(hostname)" \
        --argjson services "[$services]" \
        --arg authUrl "${DASHBOARD_AUTH_URL:-}" \
        '{services: $services, internalHostname: $internalHostname, authUrl: $authUrl}')

    echo "$config" > "$CONFIG_JSON"
    jq -e '.services | type == "array"' "$CONFIG_JSON" >/dev/null
    echo "[INFO] JSON file has been successfully created at: $CONFIG_JSON"
}

case "${DASHBOARD_AUTH_ENABLED:-1}" in
    1) create_auth ;;
    0) create_empty_auth ;;
    *) echo '[ERROR] DASHBOARD_AUTH_ENABLED must be 0 or 1' >&2; exit 1 ;;
esac
case "${DASHBOARD_AUTH_URL:-}" in
    ""|https://?*) ;;
    *) echo '[ERROR] DASHBOARD_AUTH_URL must be an HTTPS URL' >&2; exit 1 ;;
esac
create_modules
create_cors
create_services_json
