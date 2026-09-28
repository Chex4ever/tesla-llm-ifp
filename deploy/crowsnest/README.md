# Deploy Crow's Nest (`fleet.teslant.ru`)

Zero router for Pirate Fleet.

```bash
cp .env.example .env
# DNS: fleet.teslant.ru → this host
docker compose up -d --build
curl -fsS https://fleet.teslant.ru/healthz
```

Self-hosted Nest: change the site name in `Caddyfile` and tell Captains the `wss://your-domain/nest` URL.
