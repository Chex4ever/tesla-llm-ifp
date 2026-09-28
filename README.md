# Pirate Fleet

Decentralized self-hosted LLM mesh. No control-plane brain on a VPS.

**One Go binary.** Every PC is a **ship**. **Host Nest + UI** and **Accept inference** are independent flags — not two apps or exclusive roles.

> [!WARNING]
> **Work in Progress (WIP)**  
> Active development. APIs and UX may change. Issues and PRs welcome.

---

## Capabilities

| Flag | What it does |
|------|----------------|
| **Ship** (always) | Gossip, Nest catalog by RTT |
| **Host Nest + UI** | Local Nest + panel at `http://127.0.0.1:7842` |
| **Accept inference** | Take LLM jobs via Ollama/vLLM |

| Scenario | Host Nest | Infer |
|----------|-----------|-------|
| Laptop console (no GPU) | on | off |
| GPU PC + panel | on | on |
| GPU worker only | off | on |

Standalone public Nest: `pirate run --mode=crowsnest`.

---

## Quick start (TUI)

```bash
go build -o bin/pirate ./agent/cmd/pirate
./bin/pirate          # no args → TUI
```

1. **Create fleet** — Host Nest on (toggle **Accept inference off** on a GPU-less laptop).
2. **Deploy Nest** — VPS host/IP + root password (self-signed TLS).
3. **Copy join link** — share `pirate://join?…`.
4. Other PC: **Join fleet** → paste → **Run**.

Invite from the UI: name + tags + max VRAM → **Copy join link**. Filter ships by tag / model / VRAM.

Details: [docs/onboarding.md](docs/onboarding.md) · [docs/crowsnest.md](docs/crowsnest.md).

---

## CLI (advanced)

```bash
pirate join 'pirate://join?secret=…&nest=wss://HOST/nest'
pirate enroll --join-secret SECRET [--invite TOKEN] [--tags office,gpu]
pirate run                          # flags from state
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

Optional: `X-Pirate-Tags: office` or JSON `pirate_tags`.

---

## Public Nest (manual)

Preferred: TUI → **Deploy Nest**. Manual: [deploy/crowsnest/README.md](deploy/crowsnest/README.md).

Ops: [docs/runbooks/pirate-fleet.md](docs/runbooks/pirate-fleet.md).

---

## Build

```bash
go build -o bin/pirate ./agent/cmd/pirate
# Windows: go build -o bin/pirate.exe ./agent/cmd/pirate
```

Legacy Compose under `deploy/compose.yml` / `apps/fleet-api` is **not** the Pirate Fleet path.
