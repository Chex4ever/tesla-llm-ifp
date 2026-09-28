# Tesla LLM IFP — Pirate Fleet

Decentralized self-hosted LLM mesh. **No control-plane brain on a VPS.**

| Role | Pirate name | Mode |
|------|-------------|------|
| Inference node | **Deckhand** | `worker` |
| GUI / invites / optional OpenAI API | **Captain** | `captain` |
| NAT rendezvous + relay | **Crow's Nest** | `crowsnest` |

`fleet.teslant.ru` is a **public Crow's Nest** (zero router): signaling + relay only, no fleet registry, no API keys, no admin. Anyone can run another Nest; ships keep a list and survive Nest outages.

## Quick start (no VPS)

```bash
# 1) Share a fleet secret among your ships
export JOIN_SECRET="$(openssl rand -hex 32)"

# 2) First Captain
tesla-agent enroll --join-secret "$JOIN_SECRET" --name captain-1 --mode captain
tesla-agent run --mode=captain --expose-api
# UI: http://127.0.0.1:7842  (or --api/--ui addr)

# 3) In Captain UI → Create invite → on another PC:
tesla-agent enroll --join-secret "$JOIN_SECRET" --invite "<token>"
tesla-agent run --mode=worker
```

Default Nest URL baked in: `wss://fleet.teslant.ru/nest`. Add your own Nest in the Captain UI.

## Crow's Nest (public or self-hosted)

```bash
# Anywhere with a public IP / DNS
tesla-agent run --mode=crowsnest --listen :7843
```

Or Docker on `fleet.teslant.ru`:

```bash
cd deploy/crowsnest
cp .env.example .env
docker compose up -d --build
```

See [docs/onboarding.md](docs/onboarding.md) and [docs/crowsnest.md](docs/crowsnest.md).

## OpenAI API

On a Captain with `--expose-api`:

```bash
curl http://CAPTAIN:8080/v1/chat/completions \
  -H "Authorization: Bearer <key-from-state.json api_keys>" \
  -d '{"model":"llama3.2:1b","messages":[{"role":"user","content":"ahoy"}]}'
```

Jobs go Captain → mesh (via Nest if needed) → Deckhand Ollama.

## Build

```bash
go build -o bin/tesla-agent ./agent/cmd/tesla-agent
```

Legacy monolithic Compose under `deploy/compose.yml` / `apps/fleet-api` is **not** the Pirate Fleet path.
