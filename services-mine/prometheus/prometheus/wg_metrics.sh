#!/usr/bin/env bash
# Emit WireGuard peer transfer metrics for Prometheus textfile collector.
# Usage (as root): WG_IFACES="wg0 wg1" /opt/wg-metrics/wg_metrics.sh /path/to/textfile/wireguard.prom
# Default interface list is all wg* links if WG_IFACES is not set.

set -euo pipefail

OUTFILE="${1:-/opt/wg-metrics/textfile/wireguard.prom}"
TMP="$(mktemp)"
DATE="$(date +%s)"

# Determine interfaces
if [[ -z "${WG_IFACES:-}" ]]; then
  # List wg interfaces from 'wg show interfaces' (if available), else fall back to 'ip link'
  if command -v wg >/dev/null 2>&1; then
    IFACES="$(wg show interfaces 2>/dev/null || true)"
  fi
  if [[ -z "${IFACES:-}" ]]; then
    IFACES="$(ip -o link show | awk -F': ' '/^[0-9]+: wg[0-9a-zA-Z]*/{print $2}')"
  fi
else
  IFACES="${WG_IFACES}"
fi

echo "# HELP wireguard_peer_transfer_received_bytes Total bytes received for a WireGuard peer." > "$TMP"
echo "# TYPE wireguard_peer_transfer_received_bytes gauge" >> "$TMP"
echo "# HELP wireguard_peer_transfer_sent_bytes Total bytes sent for a WireGuard peer." >> "$TMP"
echo "# TYPE wireguard_peer_transfer_sent_bytes gauge" >> "$TMP"
echo "# HELP wireguard_peer_latest_handshake_seconds Unix time of the latest successful handshake for a WireGuard peer." >> "$TMP"
echo "# TYPE wireguard_peer_latest_handshake_seconds gauge" >> "$TMP"

for IFACE in $IFACES; do
  # 'wg show <iface> dump' format fields:
  # private_key public_key listen_port fwmark peers...
  # For peers lines: public_key preshared_key endpoint allowed_ips latest_handshake rx_bytes tx_bytes persistent_keepalive
  if ! wg show "$IFACE" dump >/dev/null 2>&1; then
    continue
  fi
  # Skip first line (interface), then parse peers
  wg show "$IFACE" dump | tail -n +2 | while IFS=$'\t' read -r pub presh endpoint allowed latest rx tx keepalive; do
    # Sanitize labels
    lp_iface="$IFACE"
    lp_peer="${pub}"
    lp_endpoint="${endpoint:-unknown}"
    lp_allowed="${allowed:-}"
    # Prometheus label escaping: backslash quotes
    esc() { echo -n "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }
    printf 'wireguard_peer_transfer_received_bytes{iface="%s",peer="%s",endpoint="%s",allowed_ips="%s"} %s\n' \
      "$(esc "$lp_iface")" "$(esc "$lp_peer")" "$(esc "$lp_endpoint")" "$(esc "$lp_allowed")" "${rx:-0}" >> "$TMP"
    printf 'wireguard_peer_transfer_sent_bytes{iface="%s",peer="%s",endpoint="%s",allowed_ips="%s"} %s\n' \
      "$(esc "$lp_iface")" "$(esc "$lp_peer")" "$(esc "$lp_endpoint")" "$(esc "$lp_allowed")" "${tx:-0}" >> "$TMP"
    printf 'wireguard_peer_latest_handshake_seconds{iface="%s",peer="%s"} %s\n' \
      "$(esc "$lp_iface")" "$(esc "$lp_peer")" "${latest:-0}" >> "$TMP"
  done
done

mv "$TMP" "$OUTFILE"