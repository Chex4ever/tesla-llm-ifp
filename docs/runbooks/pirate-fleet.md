# Runbook: Pirate Fleet ops

## Captain-as-Nest

Host Nest mode embeds Nest (`:7843` by default). Local ships should use that Nest. The ship also dials uplink Nests from catalog for multi-hop. Inference runs on every ship including Host Nest nodes.

## Public Nest

Preferred: TUI → Deploy Nest (`n`) with host/IP + root password (self-signed TLS).

Manual:

```bash
# Linux binary next to deploy/crowsnest/Dockerfile
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o deploy/crowsnest/pirate ./agent/cmd/pirate
cd deploy/crowsnest
cp .env.example .env   # set NEST_HOST=
docker compose up -d --build
curl -kfsS https://$NEST_HOST/healthz
```

Share `pirate://join?secret=…&nest=wss%3A%2F%2FHOST%2Fnest` with other Captains.

## Dynamic Nest catalog

- Max catalog size: 32
- Max active ship→Nest WS: 3 (nearest by probe RTT; invite seeds preferred)
- Max Nest hops: 3
- Add Nest in Captain UI advertises via gossip — no restart

## Nest outage

Ships rebalance active set. Keep ≥2 Nest URLs in catalog when possible (Captain local + public).

## wgmesh

Keep as a separate repo/tool for WireGuard L3. Not required for Crow's Nest operation.
