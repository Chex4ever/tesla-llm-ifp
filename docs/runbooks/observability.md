# Runbook: Observability

## Endpoints

- Grafana: `https://grafana.teslant.ru` (admin from `.env`)
- Prometheus: internal `http://prometheus:9090`
- Metrics: `fleet-api:/metrics`, `gateway:/metrics`

## Dashboards

Provisioned: **Pirate Fleet Overview** (`pirate-fleet-overview`)

Key signals:

- `pirate_fleet_nodes_online`
- `pirate_gateway_requests_total`
- `pirate_gateway_request_duration_seconds`

## Alerts (manual v1)

Suggested Prometheus rules (add later under `deploy/prometheus/alerts.yml`):

- Node offline: `pirate_fleet_nodes_online == 0` for 5m while nodes expected
- High gateway 5xx rate
- Disk low on VPS via node_exporter

## Backup

```bash
cd deploy
./scripts/backup.sh
```

Keep Postgres dumps and MinIO model objects off-box.
