#!/bin/sh
set -e

SERVICE="${1:-antizapret_adguard}"
DNS_NAME="${2:-adguard.antizapret}"
PORT="${AGH_PORT:-3000}"
SCHEME="${AGH_SCHEME:-http}"

if [ -z "${AGH_USER:-}" ] || [ -z "${AGH_PASS:-}" ]; then
  echo "Usage:" >&2
  echo "  AGH_USER=admin AGH_PASS='***' $0 [service_name] [dns_name]" >&2
  echo "Defaults: service=antizapret_adguard dns_name=adguard.antizapret" >&2
  echo "Optional env: AGH_PORT=3000 AGH_SCHEME=http" >&2
  exit 1
fi

NET="$(
  docker service inspect "$SERVICE" \
    --format '{{range .Spec.TaskTemplate.Networks}}{{.Target}}{{"\n"}}{{end}}' \
  | head -n1
)"
[ -n "$NET" ] || { echo "Cannot determine network for service: $SERVICE" >&2; exit 2; }

BASE="$SCHEME://$DNS_NAME:$PORT"
TMP="curl_refresh_$(date +%s)"

echo "Using swarm network: $NET"
echo "Target API: $BASE"
echo "Temp service: $TMP"

# Создаём одноразовый сервис, который выполнит запрос и завершится
docker service create \
  --name "$TMP" \
  --restart-condition none \
  --network "$NET" \
  --env AGH_USER="$AGH_USER" \
  --env AGH_PASS="$AGH_PASS" \
  --env BASE="$BASE" \
  curlimages/curl:8.7.1 \
  sh -lc '
    set -e
    curl -fsS -u "$AGH_USER:$AGH_PASS" "$BASE/control/status" >/dev/null
    curl -fsS -u "$AGH_USER:$AGH_PASS" -H "Content-Type: application/json" -d "{}" \
      "$BASE/control/filtering/refresh" >/dev/null
    echo "OK"
  ' >/dev/null

# ждём, пока таска отработает (короткий polling)
for i in 1 2 3 4 5 6 7 8 9 10; do
  # Состояние таски
  OUT="$(docker service ps "$TMP" --no-trunc 2>/dev/null | awk "NR==2{print \$6\" \"\$7\" \"\$8\" \"\$9\" \"\$10}")"
  echo "Task state: $OUT" >&2
  echo "$OUT" | grep -qi "complete" && break
  sleep 1
done

# вывести логи (там будет OK или ошибка curl)
echo "--- logs ---" >&2
docker service logs --raw "$TMP" 2>/dev/null || true

# удалить сервис
docker service rm "$TMP" >/dev/null 2>&1 || true

echo "AdGuard Home: refresh triggered (check logs above)"
