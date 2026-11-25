#!/bin/sh

. /usr/local/lib/docktinel/certificate-cache.sh

CADDYFILE="/etc/caddy/Caddyfile"
CERT_STORAGE="/data/caddy/certificates"
OCSERV_CERT_DIR="/data/ocserv"
OCSERV_IDENTITY_FILE="$OCSERV_CERT_DIR/identity"
OCSERV_ACTIVE_CERT="$OCSERV_CERT_DIR/certificate.crt"
OCSERV_ACTIVE_KEY="$OCSERV_CERT_DIR/certificate.key"
OCSERV_FALLBACK_CERT="$OCSERV_CERT_DIR/fallback.crt"
OCSERV_FALLBACK_KEY="$OCSERV_CERT_DIR/fallback.key"
WEB_CERT_DIR="/data/web"
WEB_IDENTITY_FILE="$WEB_CERT_DIR/identity"
WEB_ACTIVE_CERT="$WEB_CERT_DIR/certificate.crt"
WEB_ACTIVE_KEY="$WEB_CERT_DIR/certificate.key"
WEB_FALLBACK_CERT="$WEB_CERT_DIR/fallback.crt"
WEB_FALLBACK_KEY="$WEB_CERT_DIR/fallback.key"

find_managed_certificate() {
    identity="$1"
    for certificate in "$CERT_STORAGE"/*/"$identity"/"$identity.crt"; do
        key="${certificate%.crt}.key"
        if certificate_cached "$certificate" "$key"; then
            MANAGED_CERT="$certificate"
            MANAGED_KEY="$key"
            return 0
        fi
    done
    return 1
}

activate_certificate() {
    source_cert="$1"
    source_key="$2"
    active_cert="$3"
    active_key="$4"
    cp -f "$source_key" "$active_key.tmp"
    cp -f "$source_cert" "$active_cert.tmp"
    chmod 600 "$active_key.tmp"
    mv -f "$active_key.tmp" "$active_key"
    mv -f "$active_cert.tmp" "$active_cert"
}

certificate_fingerprint() {
    if certificate_cached "$1" "$2"; then
        printf '%s' "$CERT_FINGERPRINT"
    fi
}

sync_certificate() {
    label="$1"
    identity_file="$2"
    fallback_cert="$3"
    fallback_key="$4"
    active_cert="$5"
    active_key="$6"
    identity=$(cat "$identity_file")
    identity_type=$(cat "$(dirname "$identity_file")/identity.type")
    generate_fallback_certificate "$identity" "$identity_type" "$fallback_cert" "$fallback_key"

    desired_cert="$fallback_cert"
    desired_key="$fallback_key"
    desired_source="fallback"
    if [ "${PROXY_CERT_MODE:-auto}" = "auto" ] \
        && [ -n "$identity" ] \
        && find_managed_certificate "$identity"; then
        desired_cert="$MANAGED_CERT"
        desired_key="$MANAGED_KEY"
        desired_source="managed"
    fi

    desired_fingerprint=$(certificate_fingerprint "$desired_cert" "$desired_key")
    active_fingerprint=$(certificate_fingerprint "$active_cert" "$active_key")
    if [ -n "$desired_fingerprint" ] \
        && [ "$desired_fingerprint" = "$active_fingerprint" ] \
        && cmp -s "$desired_key" "$active_key"; then
        return 0
    fi

    activate_certificate "$desired_cert" "$desired_key" "$active_cert" "$active_key"
    echo "[INFO] $label certificate activated from $desired_source: $desired_cert"
    certificates_changed=1
}

sync_configured_certificates() {
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
        sync_certificate "SNI certificate $identity" \
            "$output_directory/identity" \
            "$output_directory/fallback.crt" "$output_directory/fallback.key" \
            "$output_directory/certificate.crt" "$output_directory/certificate.key"
        counter=$((counter + 1))
    done
}

sync_all_certificates() {
    certificates_changed=0
    sync_certificate "ocserv" \
        "$OCSERV_IDENTITY_FILE" \
        "$OCSERV_FALLBACK_CERT" "$OCSERV_FALLBACK_KEY" \
        "$OCSERV_ACTIVE_CERT" "$OCSERV_ACTIVE_KEY"
    sync_certificate "web" \
        "$WEB_IDENTITY_FILE" \
        "$WEB_FALLBACK_CERT" "$WEB_FALLBACK_KEY" \
        "$WEB_ACTIVE_CERT" "$WEB_ACTIVE_KEY"
    sync_configured_certificates
    return 0
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
        || ! certificate_cached "$fallback_crt" "$fallback_key" 86400; then
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
