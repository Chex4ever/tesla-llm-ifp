# Crow's Nest — zero router

A **Crow's Nest** is a Pirate Fleet NAT rendezvous and byte-relay. It is **not** the fleet brain.

## Who runs a Nest?

| Who | Role |
|-----|------|
| **Captain** | Always embeds a Nest for local Deckhands (LAN). Advertises it via gossip. |
| **`crowsnest` mode** | Standalone / public Nest for bootstrap and inter-site hops. |

Deckhands in the same LAN should connect through **their Captain Nest** first. Public Nest is an uplink between Captain islands.

## Deploy (recommended)

From TUI: **Deploy Nest** (`n`) — host/IP + root password. Uses self-signed TLS (Caddy `tls internal`). No Let's Encrypt.

Manual Docker: [deploy/crowsnest/README.md](../deploy/crowsnest/README.md).

Clients dial `wss://HOST/nest` with TLS skip-verify (self-signed).

## Multi-hop

If Deckhand A is on Captain A’s Nest and Deckhand B on Captain B’s Nest, traffic can go:

`A → CapA Nest → public Nest → CapB Nest → B`

Nests forward with TTL and `via[]` to prevent loops (max 3 hops).

## Dynamic catalog

Captains broadcast `nest_advert` (HMAC with `JOIN_SECRET`). Ships merge into a catalog (max 32), keep up to 3 **active** Nest WebSockets by RTT.

## Standalone Nest (local)

```bash
pirate run --mode=crowsnest --listen :7843
```

## Resources

Nest process: typically **32–64 MB RAM**, no database.
