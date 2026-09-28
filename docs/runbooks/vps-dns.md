# Runbook: VPS & DNS

## DNS

Create A records pointing to the VPS public IPv4:

- `api`, `fleet`, `chat`, `grafana`, `models`, `registry`, `hs` under `teslant.ru`

Wait for propagation (`dig +short fleet.teslant.ru`).

## Firewall

`bootstrap-vps.sh` opens: 22, 80/tcp, 443/tcp+udp, 3478/udp, 41641/udp.

## Restore Postgres

```bash
cd /opt/tesla-llm-ifp/deploy
docker compose exec -T postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > backup.sql
# restore:
cat backup.sql | docker compose exec -T postgres psql -U "$POSTGRES_USER" "$POSTGRES_DB"
```

## Rotate admin password

Set new hash via fleet-api CLI or update `ADMIN_PASSWORD` and recreate `fleet-api` on first boot only (bootstrap runs if no admins exist).
