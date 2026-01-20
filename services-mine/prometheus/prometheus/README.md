# WireGuard + Prometheus + Grafana (Textfile Collector)

This bundle runs Prometheus, Node Exporter, and Grafana via Docker Compose and
exposes WireGuard per-peer metrics using a host-side script that writes Prometheus
textfile metrics.

## What you'll get

- Prometheus at http://localhost:9090
- Grafana at http://localhost:3000 (admin / admin by default)
- Node Exporter at http://localhost:9100 (scraped by Prometheus)
- Grafana pre-provisioned with a Prometheus datasource and a WireGuard dashboard

## Quick start

1. **Place this folder on the WireGuard host**, e.g. `/opt/wg-prom-grafana`.

2. **Install the textfile writer script on the host** (needs `wg` and root):
   ```bash
   sudo mkdir -p /opt/wg-metrics/textfile
   sudo cp wg_metrics.sh /opt/wg-metrics/wg_metrics.sh
   sudo chmod +x /opt/wg-metrics/wg_metrics.sh
   sudo cp systemd/wg-metrics.service /etc/systemd/system/wg-metrics.service
   sudo cp systemd/wg-metrics.timer /etc/systemd/system/wg-metrics.timer
   sudo systemctl daemon-reload
   sudo systemctl enable --now wg-metrics.timer
   ```

   By default it discovers `wg*` interfaces. To limit interfaces, set:
   ```bash
   sudo systemctl edit wg-metrics.service
   # and add Environment=WG_IFACES="wg0"
   ```

   It writes metrics to `/opt/wg-metrics/textfile/wireguard.prom` every ~15s.

3. **Mount that textfile directory into Node Exporter**.
   This repo's `docker-compose.yml` expects a *relative* `./textfile` dir.
   Symlink it to the host path created above:
   ```bash
   ln -sf /opt/wg-metrics/textfile ./textfile
   ```

4. **Start the stack**:
   ```bash
   docker compose up -d
   ```

5. Open Grafana (http://localhost:3000), log in (admin/admin), and find the
   dashboard "WireGuard Overview".

## Notes

- If Docker runs on a different host than WireGuard, ensure the textfile dir is reachable (NFS/SSHFS, etc.).
- Security: change Grafana admin password and restrict ports as needed.
- Prometheus retention and storage can be tuned via `--storage.tsdb.retention.time` and volume size.

## Useful PromQL

- RX/TX rate per peer (bytes/s):
  ```
  rate(wireguard_peer_transfer_received_bytes[5m])
  rate(wireguard_peer_transfer_sent_bytes[5m])
  ```

- Total bytes per peer last 24h:
  ```
  sum(increase(wireguard_peer_transfer_received_bytes[24h]) + increase(wireguard_peer_transfer_sent_bytes[24h])) by (peer, iface)
  ```

- Peers with stale handshakes (> 2 minutes):
  ```
  max(time() - wireguard_peer_latest_handshake_seconds) by (peer, iface) > 120
  ```

## Alert idea (define in Prometheus alerting rules)

- Example (stale handshake > 10m):
  ```
  ALERT WireGuardPeerNoHandshake
    IF max(time() - wireguard_peer_latest_handshake_seconds) by (peer, iface) > 600
    FOR 5m
    LABELS { severity = "warning" }
    ANNOTATIONS {
      summary = "WireGuard peer missing handshake",
      description = "Peer {{ $labels.peer }} on {{ $labels.iface }} hasn't handshaked for 10m"
    }
  ```