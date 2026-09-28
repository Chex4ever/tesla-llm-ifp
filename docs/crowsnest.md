# Crow's Nest — zero router

A **Crow's Nest** is a Pirate Fleet NAT rendezvous and byte-relay. It is **not** the fleet brain.

## What it does

- Accepts WebSocket connections on `/nest`
- Registers ephemeral peer presence in RAM
- Relays envelopes between `node_id`s when direct path is unavailable
- Exposes `/healthz`

## What it does NOT do

- No Postgres / SQLite / disk state for the fleet
- No OpenAI API, no API keys, no model registry
- No admin UI, no invites storage
- No authority over who is Captain

## Run your own

```bash
tesla-agent run --mode=crowsnest --listen :7843
# Put TLS in front (Caddy/nginx) → wss://your-nest.example/nest
```

Docker (same layout as public nest):

```bash
cd deploy/crowsnest
# Point Caddyfile server_name to your domain
docker compose up -d --build
```

## Public default

`fleet.teslant.ru` ships as the default Nest URL in the agent. Treat it as a community lookout post — replaceable anytime via Captain Nest list.

## Resources

Single Go binary: typically **32–64 MB RAM**. No database volume required.
