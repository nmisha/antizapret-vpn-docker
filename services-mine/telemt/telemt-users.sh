#!/usr/bin/env bash
# Per-user secrets and limits for telemt via its control API.
# Run in telemt's network namespace (see README), or set a reachable TELEMT_API.
# Token: TELEMT_TOKEN, else auth_header from config-mine/telemt/config.toml (readable by uid 65532/root only).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
CONF=${TELEMT_CONFIG:-$ROOT/config-mine/telemt/config.toml}
API=${TELEMT_API:-http://127.0.0.1:9091}

usage() {
  cat <<'EOF'
Usage: telemt-users.sh <command> [args]

  list                       users, connections, limits, traffic
  info <user>                full user info as JSON
  link <user>                tg://proxy link (fake-TLS)
  add <user> [limits]        create user with a generated secret, print link
  set <user> <limits>        change limits ("none" removes an individual limit)
  del <user>
  enable <user> | disable <user>   disable also drops active sessions
  rotate <user>              new secret (old link stops working), print link
  reset-quota <user>

Limits:
  --ips N          max unique source IPs (devices)
  --conns N        max concurrent TCP connections
  --expires TS     expiration, RFC3339 (2026-12-31T23:59:59Z)
  --quota BYTES    traffic quota
  --up BPS         upload rate limit, bits/s
  --down BPS       download rate limit, bits/s

Env: TELEMT_API (default http://127.0.0.1:9091), TELEMT_TOKEN, TELEMT_CONFIG
EOF
}

die() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null || die "$1 is required"; }
need curl; need jq

token() {
  if [ -n "${TELEMT_TOKEN:-}" ]; then printf '%s' "$TELEMT_TOKEN"; return; fi
  [ -r "$CONF" ] || die "cannot read $CONF (set TELEMT_TOKEN or run with sudo)"
  sed -n 's/^auth_header *= *"\(.*\)"/\1/p' "$CONF" | head -n1
}

# api METHOD PATH [JSON] -> prints the .data of the response
api() {
  local out
  out=$(curl --connect-timeout 5 --max-time 30 -sS -X "$1" -H "Authorization: $(token)" -H 'Content-Type: application/json' \
        ${3:+-d "$3"} "$API/v1$2") || die "API unreachable at $API"
  jq -e '.ok == true' >/dev/null 2>&1 <<<"$out" || { jq -r '.error.message // .' <<<"$out" >&2; exit 1; }
  jq -c '.data' <<<"$out"
}

# limits flags -> JSON body
body() {
  local j='{}' k v kind
  while [ $# -gt 0 ]; do
    [ $# -ge 2 ] || die "$1 needs a value"
    case $1 in
      --ips)     k=max_unique_ips;      kind=num ;;
      --conns)   k=max_tcp_conns;       kind=num ;;
      --expires) k=expiration_rfc3339;  kind=str ;;
      --quota)   k=data_quota_bytes;    kind=num ;;
      --up)      k=rate_limit_up_bps;   kind=num ;;
      --down)    k=rate_limit_down_bps; kind=num ;;
      *) die "unknown option $1" ;;
    esac
    v=$2; shift 2
    if [ "$v" = none ]; then j=$(jq -c --arg k "$k" '.[$k]=null' <<<"$j")
    elif [ $kind = str ]; then j=$(jq -c --arg k "$k" --arg v "$v" '.[$k]=$v' <<<"$j")
    else
      [[ $v =~ ^[0-9]+$ ]] || die "$k must be a number or 'none'"
      j=$(jq -c --arg k "$k" --argjson v "$v" '.[$k]=$v' <<<"$j")
    fi
  done
  printf '%s' "$j"
}

user_arg() { [ -n "${1:-}" ] || die "user name required"; [[ $1 =~ ^[A-Za-z0-9_.-]{1,64}$ ]] || die "invalid user name"; }

print_link() { jq -r '(.user // .).links.tls[0] // "no tls link"' <<<"$1"; }

[ $# -ge 1 ] || { usage; exit 1; }
c=$1; shift
case $c in
  list)
    need column
    api GET /users | jq -r '.[] | [.username, (if .enabled then "on" else "OFF" end),
        "\(.current_connections)/\(.max_tcp_conns // "-")", "\(.active_unique_ips)/\(.max_unique_ips // "-")",
        (.expiration_rfc3339 // "-"), (.total_octets/1048576|floor|tostring + "MiB")] | @tsv' |
      { echo -e "USER\tSTATE\tCONNS\tIPS\tEXPIRES\tTRAFFIC"; cat; } | column -t -s $'\t' ;;
  info)    user_arg "${1:-}"; api GET "/users/$1" | jq . ;;
  link)    user_arg "${1:-}"; print_link "$(api GET "/users/$1")" ;;
  add)
    user_arg "${1:-}"; u=$1; shift
    b=$(body "$@")
    r=$(api POST /users "$(jq -c --arg u "$u" '.username=$u' <<<"$b")")
    echo "created $u"; print_link "$r" ;;
  set)     user_arg "${1:-}"; u=$1; shift; [ $# -gt 0 ] || die "no limits given"
           b=$(body "$@")
           api PATCH "/users/$u" "$b" | jq -r '"updated \(.username)"' ;;
  del)     user_arg "${1:-}"; api DELETE "/users/$1" >/dev/null; echo "deleted $1" ;;
  enable|disable) user_arg "${1:-}"; api POST "/users/$1/$c" >/dev/null; echo "$1: $c" ;;
  rotate)  user_arg "${1:-}"; print_link "$(api POST "/users/$1/rotate-secret")" ;;
  reset-quota) user_arg "${1:-}"; api POST "/users/$1/reset-quota" >/dev/null; echo "quota reset for $1" ;;
  -h|--help|help) usage ;;
  *) usage; exit 1 ;;
esac
