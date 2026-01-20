# AdGuard Home Integration Pack (Prometheus + Loki)

This add-on integrates AdGuard Home with your existing Prometheus/Grafana stack and adds logs via Loki/Promtail.

## What’s inside
- docker-compose.adguard-addon.yml — services: adguard-exporter, loki, promtail
- loki-config.yaml — minimal Loki config (7d retention)
- promtail-config.yaml — reads AdGuard querylog.json and pushes to Loki
- prometheus-scrape-snippet.yml — scrape job for adguard-exporter
- grafana-dashboard-adguard-loki.json — sample Loki dashboard (top domains per client)

## How to use

1) Ensure your main stack network is named `monitoring` (as in the base pack). If not, edit the compose file.

2) Point `ADGUARD_SERVERS` to your AdGuard Home UI URL, and set proper credentials in `docker-compose.adguard-addon.yml`.

3) Mount the correct path to `querylog.json` into promtail. If AdGuard runs in Docker, bind the container volume or mount the host path accordingly.

4) Start services:
   ```bash
   docker compose -f docker-compose.adguard-addon.yml up -d
   ```

5) In Prometheus, add the scrape job from `prometheus-scrape-snippet.yml` and reload Prometheus config.

6) In Grafana:
   - Add Loki datasource (URL: http://loki:3100).
   - Import `grafana-dashboard-adguard-loki.json` for logs analytics.
   - Optionally import an AdGuard Prometheus dashboard compatible with your exporter.

## LogQL Examples
- Top 20 domains over last 24h for client 10.0.0.2:
  ```
  {job="adguard_querylog", IP="10.0.0.2"} | json | stats count() by (QH) | sort desc | limit 20
  ```
- Most chatty clients:
  ```
  {job="adguard_querylog"} | json | stats count() by (IP) | sort desc | limit 20
  ```

## Notes
- Avoid using QH as a label to prevent cardinality explosions.
- Tune Loki retention in `loki-config.yaml`.
- Secure credentials and network exposure.