# Deploy Crow's Nest

Zero router for Pirate Fleet. Self-signed TLS via openssl (no Let's Encrypt).

## From TUI (recommended)

```text
pirate
  → Become Captain
  → Deploy Nest  (host/IP + root password)
```

## Manual

```bash
# Place a Linux pirate binary next to Dockerfile, then:
cp .env.example .env
# edit NEST_HOST=your.vps.ip.or.hostname
bash ../... # or:
openssl ... # easier: copy gen-certs from agent/internal/vpsdeploy/templates
# From repo:
bash agent/internal/vpsdeploy/templates/gen-certs.sh "$NEST_HOST" deploy/crowsnest/certs
cd deploy/crowsnest
docker compose up -d --build

curl -kfsS https://$NEST_HOST/healthz
```

Browser will warn about the self-signed cert — that is expected. Clients use InsecureSkipVerify for `wss://$NEST_HOST/nest`.
