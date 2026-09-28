# Pirate Fleet onboarding

## Terms

- **Deckhand** (`worker`) — runs Ollama, gossips status, executes inference jobs.
- **Captain** (`captain`) — local GUI, creates invites, optional `--expose-api`.
- **Crow's Nest** (`crowsnest`) — zero NAT router. Default: `wss://fleet.teslant.ru/nest`.

## Prerequisites (Windows Deckhand)

- Ollama installed and running locally
- Outbound WSS to at least one Nest
- Shared `JOIN_SECRET` for the fleet (from your Captain / ops)

## Join via Captain invite

1. On a Captain, open `http://127.0.0.1:7842` → **Create invite**.
2. On the new PC:

```powershell
$env:JOIN_SECRET = "<same secret as captain>"
.\tesla-agent.exe enroll --join-secret $env:JOIN_SECRET --invite "<token>"
.\tesla-agent.exe run --mode=worker
```

3. Ship appears in Captain **Ships** table after gossip (~15s).
4. **Ensure model** with an Ollama tag (e.g. `llama3.2:1b`).

## Add a second Nest (failover)

1. Someone runs: `tesla-agent run --mode=crowsnest --listen :7843` behind TLS (`wss://nest.example.com/nest`).
2. Captain UI → **Add Nest URL**.
3. Re-enroll or edit `state.json` `nests` array on ships, restart.

If `fleet.teslant.ru` dies, ships with another Nest keep rendezvous.

## Captain with public API

```bash
tesla-agent run --mode=captain --expose-api --api 0.0.0.0:8080
```

Put API keys in `state.json` → `api_keys: ["sk-..."]` (empty list = open, local only recommended).
