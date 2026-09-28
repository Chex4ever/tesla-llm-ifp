# Runbook: Hardening

1. Change all passwords in `deploy/.env` before first boot.
2. Restrict SSH to your IP; prefer key auth only.
3. Grafana / MinIO console: consider Tailscale-only access later; until then use strong passwords.
4. NATS port 4222 is public with token auth — use a long random `NATS_TOKEN`.
5. Rotate `INTERNAL_SHARED_SECRET` and API keys periodically.
6. Keep Ollama bound to localhost on workers; agent proxies via NATS.
7. Revoke invites and drain nodes from Fleet UI when decommissioning PCs.
