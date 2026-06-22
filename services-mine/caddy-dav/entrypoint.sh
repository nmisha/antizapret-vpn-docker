#!/usr/bin/env bash
set -euo pipefail

CADDYFILE_PATH="${CADDYFILE_PATH:-/etc/caddy/Caddyfile}"
DAV_ROOT="${DAV_ROOT:-/data}"
DAV_LISTEN="${DAV_LISTEN:-:80}"
DAV_BROWSE="${DAV_BROWSE:-true}"       # true/false
DAV_PATH="${DAV_PATH:-/}"              # "/" или "/dav"
DAV_PREFIX="${DAV_PREFIX:-}"           # обычно пусто; для /dav выставим автоматически

# если DAV_PATH=/dav, то матчим /dav/* и выставляем prefix /dav
if [ "$DAV_PATH" = "/" ]; then
  DAV_MATCH="/*"
  DAV_PREFIX_LINE=""
else
  # нормализуем: "/dav" без trailing slash
  DAV_PATH="${DAV_PATH%/}"
  DAV_MATCH="${DAV_PATH}/*"
  DAV_PREFIX_LINE=$'\n    prefix '"$DAV_PATH"$'\n'
fi

mkdir -p "$(dirname "$CADDYFILE_PATH")"

# ВСЕГДА перезаписываем Caddyfile
echo "[entrypoint] Generating Caddyfile at: $CADDYFILE_PATH"
cat > "$CADDYFILE_PATH" <<EOF
{
  order webdav before file_server
}

$DAV_LISTEN {
  # WebDAV
  webdav $DAV_MATCH {
    root $DAV_ROOT$DAV_PREFIX_LINE
  }

  # Optional browsing / GET access
  root * $DAV_ROOT
EOF

if [ "$DAV_BROWSE" = "true" ]; then
  echo "  file_server browse" >> "$CADDYFILE_PATH"
else
  echo "  file_server" >> "$CADDYFILE_PATH"
fi

cat >> "$CADDYFILE_PATH" <<'EOF'
}
EOF

echo "[entrypoint] Caddyfile content:"
echo "------------------------------------------------------------"
cat "$CADDYFILE_PATH"
echo "------------------------------------------------------------"

exec caddy run --config "$CADDYFILE_PATH" --adapter caddyfile