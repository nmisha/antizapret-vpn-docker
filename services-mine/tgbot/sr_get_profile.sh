#!/bin/sh
set -e

if [ $# -lt 2 ]; then
  echo "Usage: $0 <swarm_service_name> <profile_name>"
  echo "Example: $0 antizapret_wireguard Kablag_Work1"
  exit 1
fi

SERVICE="$1"
PROFILE_NAME="$2"

TASK_ID="$(docker service ps -q "$SERVICE" | head -n1)"
[ -n "$TASK_ID" ] || { echo "No tasks found for service: $SERVICE" >&2; exit 2; }

CID="$(docker inspect -f '{{.Status.ContainerStatus.ContainerID}}' "$TASK_ID" 2>/dev/null || true)"
[ -n "$CID" ] || { echo "Cannot get container id for service: $SERVICE" >&2; exit 3; }

docker exec -i -e PROFILE_NAME="$PROFILE_NAME" "$CID" sh -s <<'EOS'
set -e

J="/etc/wireguard/wg0.json"
[ -f "$J" ] || { echo "JSON not found: $J" >&2; exit 4; }

# интерфейс WG
IF="$(wg show interfaces | awk 'NR==1 {print $1; exit}')"
[ -n "$IF" ] || { echo "No wg interfaces" >&2; exit 5; }

# server listen port
SRV_PORT="$(wg show "$IF" listen-port 2>/dev/null | head -n1 || true)"
[ -n "$SRV_PORT" ] || SRV_PORT="51820"

# server public key (предпочтительно из wg show)
SRV_PUB="$(wg show "$IF" 2>/dev/null | awk -F': ' '/public key/ {print $2; exit}' || true)"
# fallback: из showconf privatekey -> wg pubkey
if [ -z "$SRV_PUB" ]; then
  SRV_PRIV="$(wg showconf "$IF" 2>/dev/null | awk -F'= ' '/^PrivateKey/ {print $2; exit}' || true)"
  if [ -n "$SRV_PRIV" ] && command -v wg >/dev/null 2>&1; then
    SRV_PUB="$(printf '%s' "$SRV_PRIV" | wg pubkey 2>/dev/null || true)"
  fi
fi
[ -n "$SRV_PUB" ] || { echo "Cannot determine server public key" >&2; exit 6; }

# Попытка определить endpoint host/port из env или json
# env (часто в wg-easy): WG_HOST/WG_ENDPOINT/WG_PORT
ENV_HOST="${WG_ENDPOINT:-${WG_HOST:-}}"
ENV_PORT="${WG_PORT:-}"

export IF SRV_PUB SRV_PORT ENV_HOST ENV_PORT
node - <<'NODE'
const fs = require('fs');

const profileName = (process.env.PROFILE_NAME || '').trim();
if (!profileName) {
  console.error('PROFILE_NAME env is empty');
  process.exit(7);
}
const target = profileName.toLowerCase();

const j = JSON.parse(fs.readFileSync('/etc/wireguard/wg0.json', 'utf8'));

// Извлечь настройки хоста/порта из JSON (если есть)
function pick(obj, keys) {
  if (!obj || typeof obj !== 'object') return undefined;
  for (const k of keys) if (obj[k] != null) return obj[k];
  const lower = Object.fromEntries(Object.entries(obj).map(([k,v]) => [k.toLowerCase(), v]));
  for (const k of keys) {
    const v = lower[k.toLowerCase()];
    if (v != null) return v;
  }
  return undefined;
}

const jsonHost =
  pick(j, ['wgHost','host','endpoint','WG_HOST','WG_ENDPOINT']) ??
  pick(j.settings, ['wgHost','host','endpoint']) ??
  pick(j.server, ['wgHost','host','endpoint']);

const jsonPort =
  pick(j, ['wgPort','port','WG_PORT']) ??
  pick(j.settings, ['wgPort','port']) ??
  pick(j.server, ['wgPort','port']);

const dns =
  pick(j, ['wgDns','dns','WG_DNS']) ??
  pick(j.settings, ['wgDns','dns']) ??
  pick(j.server, ['wgDns','dns']);

const allowedIps =
  pick(j, ['wgAllowedIps','allowedIps','WG_ALLOWED_IPS']) ??
  pick(j.settings, ['wgAllowedIps','allowedIps']) ??
  pick(j.server, ['wgAllowedIps','allowedIps']) ??
  '0.0.0.0/0, ::/0';

// Найти клиента по имени (case-insensitive exact match)
const clients = j.clients || {};
let client = null;
for (const id of Object.keys(clients)) {
  const c = clients[id] || {};
  const name = String(c.name || '').trim();
  if (name && name.toLowerCase() === target) {
    client = c;
    break;
  }
}
if (!client) {
  console.error(`Profile not found in wg0.json: "${profileName}" (case-insensitive exact match)`);
  process.exit(8);
}

const cName = String(client.name || '').trim();
const cPriv = String(client.privateKey || '').trim();
const cAddr = String(client.address || '').trim();
const cPsk  = String(client.preSharedKey || '').trim();

if (!cPriv) { console.error('Client privateKey missing in JSON'); process.exit(9); }
if (!cAddr) { console.error('Client address missing in JSON'); process.exit(10); }

// address в JSON у тебя без /32
const addr = cAddr.includes('/') ? cAddr : (cAddr + '/32');

const srvPub = process.env.SRV_PUB;
const listenPort = process.env.SRV_PORT || '51820';

let host = (process.env.ENV_HOST || '').trim();
if (!host) host = (jsonHost != null ? String(jsonHost).trim() : '');
let port = (process.env.ENV_PORT || '').trim();
if (!port) port = (jsonPort != null ? String(jsonPort).trim() : '');
if (!port) port = listenPort;

let endpoint = '';
if (host) endpoint = `${host}:${port}`;

// Вывод конфига
console.log(`# Profile: ${cName}`);
console.log(`[Interface]`);
console.log(`PrivateKey = ${cPriv}`);
console.log(`Address = ${addr}`);
if (dns) console.log(`DNS = ${dns}`);

console.log('');
console.log(`[Peer]`);
console.log(`PublicKey = ${srvPub}`);
if (cPsk) console.log(`PresharedKey = ${cPsk}`);

if (endpoint) {
  console.log(`Endpoint = ${endpoint}`);
} else {
  console.log(`# Endpoint is unknown (no WG_HOST/WG_ENDPOINT and no host in wg0.json).`);
  console.log(`# Set it manually, e.g.: Endpoint = <your-public-host>:${port}`);
}

console.log(`AllowedIPs = ${allowedIps}`);
console.log(`PersistentKeepalive = 25`);
NODE
EOS
