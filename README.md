# Pirate Fleet

Decentralized self-hosted LLM mesh. No control-plane brain on a VPS.

**One Go binary.** Every PC is a **ship** (inference + Nest discovery). **Host Nest + UI** is an optional toggle — not a second app.

> [!WARNING]
> **Work in Progress (WIP)**  
> Active development. APIs and UX may change. Issues and PRs welcome.

---

## Roles

| Capability | What it does |
|------------|----------------|
| **Ship** (always) | Ollama/vLLM inference, gossip, Nest catalog by RTT |
| **Host Nest + UI** | Local Nest (`:7843`) + panel at `http://127.0.0.1:7842` |
| **Crow's Nest** | Standalone / public Nest (`run --mode=crowsnest`) |

LAN ships prefer their Host Nest. Public Nest is an uplink between sites. Nest catalog is gossip-driven (multi-hop).

---

## Quick start (TUI)

```bash
go build -o bin/pirate ./agent/cmd/pirate
./bin/pirate          # no args → TUI
```

1. **Create fleet** — `JOIN_SECRET` generated; Host Nest on.
2. **Deploy Nest** — VPS host/IP + root password (self-signed TLS).
3. **Copy join link** — share `pirate://join?…` with another PC.
4. Other PC: **Join fleet** → paste link → **Run**.

Same app everywhere. Toggle **Host Nest + UI** if this machine should also host a Nest/panel.

Invite from the UI (after **Run** with Host Nest): name + tags + optional max VRAM → **Copy join link**. Ships filter by tag / model / VRAM.

Details: [docs/onboarding.md](docs/onboarding.md) · [docs/crowsnest.md](docs/crowsnest.md).

---

## CLI (advanced)

```bash
pirate join 'pirate://join?secret=…&nest=wss://HOST/nest'
pirate enroll --join-secret SECRET [--invite TOKEN] [--tags office,gpu]
pirate run                          # mode from state
pirate run --mode=captain --expose-api
pirate run --mode=crowsnest --listen :7843
```

---

## OpenAI API

With Host Nest + `--expose-api`:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer <key-from-state.json api_keys>" \
  -d '{"model":"llama3.2:1b","messages":[{"role":"user","content":"ahoy"}]}'
```

Optional routing: header `X-Pirate-Tags: office` or JSON field `pirate_tags`.

---

## Public Nest (manual)

Preferred: TUI → **Deploy Nest**. Manual Docker: [deploy/crowsnest/README.md](deploy/crowsnest/README.md).

Ops: [docs/runbooks/pirate-fleet.md](docs/runbooks/pirate-fleet.md).

---

## Build

```bash
go build -o bin/pirate ./agent/cmd/pirate
```
```powershell
go build -o bin/pirate.exe ./agent/cmd/pirate
```
Legacy Compose under `deploy/compose.yml` / `apps/fleet-api` is **not** the Pirate Fleet path.
