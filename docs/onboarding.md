# Pirate Fleet onboarding

One app: **`pirate`**. Every PC is a **ship** (inference + Nest discovery). **Host Nest + UI** is an optional toggle, not a separate product.

## Roles (capabilities)

| Capability | What it does |
|------------|----------------|
| Ship (always) | Ollama/vLLM inference, gossip, Nest catalog by RTT |
| Host Nest + UI | Local Nest on `:7843` + panel at `http://127.0.0.1:7842` |
| Crow's Nest | Standalone public Nest (`pirate run --mode=crowsnest`) |

## Golden path (TUI)

```bash
pirate
```

1. **Create fleet** — generates `JOIN_SECRET`, enables Host Nest.
2. **Deploy Nest** (optional) — VPS host/IP + root password (self-signed TLS).
3. **Copy join link** or create invite in the UI (`Run` → open `:7842`).
4. On another PC: **Join fleet** → paste `pirate://join?…` → **Run**.

Same binary on every machine. Toggle **Host Nest + UI** if this PC should also host a Nest/panel.

## Invite another ship (tags / VRAM)

With Host Nest running, open UI → **Invite ship**:

- Name, tags (e.g. `office`, `fat-pipe`), optional Max VRAM MB
- **Copy join link** (`pirate://join?secret=…&nest=…&invite=…`)
- Other PC: `pirate` → Join fleet → paste → Run

Ships appear under **Ships** with tags, models, and VRAM. Filter by tag / model / VRAM ≥ N.

Routing respects tags (`X-Pirate-Tags` or `pirate_tags` in the chat body) and capacity (`max_vram_mb`).

## CLI (advanced)

```powershell
.\pirate.exe enroll --join-secret $env:JOIN_SECRET --invite "<token>"
.\pirate.exe run
```

Or with Host Nest:

```powershell
.\pirate.exe run --mode=captain --expose-api
```

## Second site

Share a join link **without** invite (`pirate://join?secret=…&nest=…`) — joins with Host Nest on. Or paste an invite link for a peer-only ship.

## Add Nest / failover

UI → Nest catalog → **Add & advertise**, or TUI **Deploy Nest**. Fleet learns via gossip. Active connections stay ≤ 3 nearest.

## wgmesh

Optional separate L3 WireGuard mesh (`wgmesh` repo). Not required for Crow's Nest.
