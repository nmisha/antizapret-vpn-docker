#!/bin/sh

set -eu

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
        if openssl x509 -in "$certificate" -noout -checkend 0 >/dev/null 2>&1 \
            && [ -s "$key" ]; then
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
    openssl x509 -in "$1" -noout -fingerprint -sha256 2>/dev/null || true
}

sync_certificate() {
    label="$1"
    identity_file="$2"
    fallback_cert="$3"
    fallback_key="$4"
    active_cert="$5"
    active_key="$6"
    identity=$(cat "$identity_file" 2>/dev/null || true)

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

    desired_fingerprint=$(certificate_fingerprint "$desired_cert")
    active_fingerprint=$(certificate_fingerprint "$active_cert")
    if [ -n "$desired_fingerprint" ] \
        && [ "$desired_fingerprint" = "$active_fingerprint" ] \
        && [ -s "$active_key" ]; then
        return 1
    fi

    activate_certificate "$desired_cert" "$desired_key" "$active_cert" "$active_key"
    echo "[INFO] $label certificate activated from $desired_source: $desired_cert"
    return 0
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
        if sync_certificate "SNI certificate $identity" \
            "$output_directory/identity" \
            "$output_directory/fallback.crt" "$output_directory/fallback.key" \
            "$output_directory/certificate.crt" "$output_directory/certificate.key"; then
            certificates_changed=1
        fi
        counter=$((counter + 1))
    done
}

sync_all_certificates() {
    certificates_changed=0
    if sync_certificate "ocserv" \
        "$OCSERV_IDENTITY_FILE" \
        "$OCSERV_FALLBACK_CERT" "$OCSERV_FALLBACK_KEY" \
        "$OCSERV_ACTIVE_CERT" "$OCSERV_ACTIVE_KEY"; then
        certificates_changed=1
    fi
    if sync_certificate "web" \
        "$WEB_IDENTITY_FILE" \
        "$WEB_FALLBACK_CERT" "$WEB_FALLBACK_KEY" \
        "$WEB_ACTIVE_CERT" "$WEB_ACTIVE_KEY"; then
        certificates_changed=1
    fi
    sync_configured_certificates
    return 0
}

watch_managed_certificates() {
    reload_pending=0
    while sleep 10; do
        sync_all_certificates
        if [ "$certificates_changed" -eq 1 ]; then
            reload_pending=1
        fi
        if [ "$reload_pending" -eq 1 ] \
            && caddy reload --force --config "$CADDYFILE" --adapter caddyfile; then
            reload_pending=0
            echo "[INFO] Caddy reloaded after certificate update"
        fi
    done
}

/init.sh

sync_all_certificates

if [ "${PROXY_CERT_MODE:-auto}" = "auto" ]; then
    watch_managed_certificates &
fi

exec caddy run --config "$CADDYFILE" --adapter caddyfile
