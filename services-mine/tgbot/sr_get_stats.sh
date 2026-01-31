#!/bin/sh
set -e

if [ $# -lt 2 ]; then
  echo "Usage: $0 <swarm_service_name> <profile_prefix>"
  echo "Example: $0 antizapret_wireguard kablag_"
  exit 1
fi

SERVICE="$1"
PREFIX="$2"

TASK_ID="$(docker service ps -q "$SERVICE" | head -n1)"
[ -n "$TASK_ID" ] || { echo "No tasks found for service: $SERVICE" >&2; exit 2; }

CID="$(docker inspect -f '{{.Status.ContainerStatus.ContainerID}}' "$TASK_ID" 2>/dev/null || true)"
[ -n "$CID" ] || { echo "Cannot get container id for service: $SERVICE" >&2; exit 3; }

# Передаём PREFIX внутрь контейнера через env, и запускаем скрипт через stdin (без адских кавычек)
docker exec -i -e PREFIX="$PREFIX" "$CID" sh -s <<'EOS'
set -e

J="/etc/wireguard/wg0.json"
[ -f "$J" ] || { echo "JSON not found: $J" >&2; exit 4; }

# 1) map: pubkey(lower) -> name (prefix filter, case-insensitive)
node - <<'NODE' > /tmp/map.tsv
const fs = require('fs');
const prefix = process.env.PREFIX || '';
const re = new RegExp('^' + prefix, 'i');

const j = JSON.parse(fs.readFileSync('/etc/wireguard/wg0.json', 'utf8'));
const clients = j.clients || {};

for (const id of Object.keys(clients)) {
  const c = clients[id] || {};
  const name = String(c.name || '').trim();
  const pk = String(c.publicKey || '').trim();
  if (re.test(name) && pk) {
    console.log(pk.toLowerCase() + '\t' + name);
  }
}
NODE

# если по префиксу ничего не нашли — просто выходим без ошибки (можно поменять поведение)
[ -s /tmp/map.tsv ] || exit 0

# 2) интерфейс WireGuard (обычно wg0)
IF="$(wg show interfaces | awk 'NR==1 {print $1; exit}')"
[ -n "$IF" ] || { echo "No wg interfaces" >&2; exit 5; }

# 3) wg dump peer rows -> pubkey endpoint last_hs rx tx
# columns: pubkey preshared endpoint allowed latest rx tx keepalive
wg show "$IF" dump \
  | awk -F '\t' 'NR>1 {print tolower($1) "\t" $3 "\t" $5 "\t" $6 "\t" $7}' \
  > /tmp/wg.tsv

# 4) join -> sort by last_hs desc (0 last) -> pretty vertical output
awk -F '\t' '
  function gib(x){ return x/1024/1024/1024 }
  function hs_fmt(hs,   now, d){
    if (hs == 0) return "never"
    now = systime()
    d = now - hs
    if (d <= 300) return d " seconds ago"
    return strftime("%Y-%m-%d %H:%M:%S", hs)
  }
  FNR==NR {name[$1]=$2; next}
  ($1 in name) {
    sk = ($3==0 ? -1 : $3)
    print sk "\t" name[$1] "\t" $2 "\t" $3 "\t" $4 "\t" $5
  }
' /tmp/map.tsv /tmp/wg.tsv \
| sort -nr -k1,1 \
| awk -F '\t' '
  function gib(x){ return x/1024/1024/1024 }
  function hs_fmt(hs,   now, d){
    if (hs == 0) return "never"
    now = systime()
    d = now - hs
    if (d <= 300) return d " seconds ago"
    return strftime("%Y-%m-%d %H:%M:%S", hs)
  }
  {
    printf "profile:  %s\n", $2
    printf "endpoint: %s\n", ($3=="(none)" ? "-" : $3)
    printf "last:     %s\n", hs_fmt($4)
    printf "rx:       %.2f GiB\n", gib($5)
    printf "tx:       %.2f GiB\n", gib($6)
    printf "\n"
  }
'
EOS
