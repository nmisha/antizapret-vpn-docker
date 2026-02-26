#!/usr/bin/env bash
set -euo pipefail

CADDYFILE_PATH="${CADDYFILE_PATH:-/etc/caddy/Caddyfile}"
DAV_ROOT="${DAV_ROOT:-/data}"
DAV_LISTEN="${DAV_LISTEN:-:80}"
DAV_BROWSE="${DAV_BROWSE:-true}"          # true/false
DAV_PREFIX="${DAV_PREFIX:-/}"             # "/" или "/dav"
DAV_READONLY="${DAV_READONLY:-false}"     # true/false

mkdir -p "$(dirname "$CADDYFILE_PATH")"

# Генерация дефолтного Caddyfile если отсутствует
if [ ! -s "$CADDYFILE_PATH" ]; then
  echo "[entrypoint] Caddyfile not found at $CADDYFILE_PATH -> generating default one"

  # Настроим, будет ли browse
  FILE_SERVER_BLOCK=""
  if [ "$DAV_BROWSE" = "true" ]; then
    FILE_SERVER_BLOCK=$'\n  file_server browse\n'
  else
    FILE_SERVER_BLOCK=$'\n  file_server\n'
  fi

  # Если readonly — выключаем методы записи
  # (модуль webdav поддерживает настройки; если у твоей сборки директивы отличаются,
  # просто убери readonly-блок и оставь базовый webdav { root ... } )
  READONLY_BLOCK=""
  if [ "$DAV_READONLY" = "true" ]; then
    # Некоторые реализации модуля поддерживают "readonly"
    READONLY_BLOCK=$'\n    readonly\n'
  fi

  cat > "$CADDYFILE_PATH" <<EOF
{
  # webdav — директива не из core, задаем порядок
  order webdav before file_server
}

$DAV_LISTEN {
  # WebDAV
  @dav path $DAV_PREFIX*
  handle @dav {
    webdav {
      root $DAV_ROOT$READONLY_BLOCK
    }
  }

  # HTTP доступ (GET/dir listing) на тот же root
  handle {
    root * $DAV_ROOT$FILE_SERVER_BLOCK
  }
}
EOF

  echo "[entrypoint] Generated Caddyfile:"
  echo "------------------------------------------------------------"
  cat "$CADDYFILE_PATH"
  echo "------------------------------------------------------------"
else
  echo "[entrypoint] Using existing Caddyfile at $CADDYFILE_PATH"
fi

# Запуск Caddy
exec caddy run --config "$CADDYFILE_PATH" --adapter caddyfile