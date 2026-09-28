# Pirate Fleet onboarding

One app: **`pirate`**. Every PC is a **ship**. Capabilities are independent toggles — not mutually exclusive roles.

## Capabilities

| Flag | What it does |
|------|----------------|
| **Host Nest + UI** | Local Nest (`:7843`) + panel `http://127.0.0.1:7842` (invites, ships, OpenAI gateway) |
| **Accept inference** | This PC takes LLM jobs (Ollama/vLLM) |
| Always on | Gossip + Nest catalog discovery |

| Scenario | Host Nest | Infer |
|----------|-----------|-------|
| Laptop console (no GPU) | on | **off** |
| GPU PC + panel | on | on |
| GPU worker only | off | on |

## Golden path (TUI)

```bash
pirate
```

1. **Create fleet** — secret generated; Host Nest on, Infer on (turn Infer **off** on a laptop without GPU).
2. **Deploy Nest** (optional) — VPS host/IP + root password.
3. **Copy join link** or Invite in the UI.
4. Other PC: **Join fleet** → paste → **Run**.

## Invite another ship (tags / VRAM)

UI → **Invite ship** → Copy join link. Other PC: Join → Run.

Ships filter by tag / model / VRAM. Routing: `X-Pirate-Tags` / `pirate_tags`. Peers with Accept inference off are never selected.

## CLI (advanced)

```powershell
.\pirate.exe enroll --join-secret $env:JOIN_SECRET --invite "<token>"
.\pirate.exe run
```

## Second site

Join link without invite → Host Nest on. Invite link → peer ship (Host Nest off). Toggle either flag anytime in TUI.

## Add Nest / failover

UI → Nest catalog → **Add & advertise**, or TUI **Deploy Nest**. Active connections ≤ 3 nearest.

## wgmesh

Optional separate L3 WireGuard mesh. Not required for Crow's Nest.
