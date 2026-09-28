# Runbook: Pirate Fleet ops

## Deploy public Nest (`fleet.teslant.ru`)

1. DNS A record `fleet` → VPS IP
2. `cd deploy/crowsnest && cp .env.example .env && docker compose up -d --build`
3. `curl -fsS https://fleet.teslant.ru/healthz`

## Rotate JOIN_SECRET

All ships must re-enroll with the new secret; old gossip HMACs will be ignored. Issue fresh invites from a Captain.

## Nest outage

1. Confirm ships have ≥2 nests in `state.json`
2. Bring up replacement Nest
3. Add URL in Captain UI / state and restart agents

## Legacy stack

`deploy/compose.yml` (Postgres, NATS, fleet-api, …) is legacy and not required for Pirate Fleet.
