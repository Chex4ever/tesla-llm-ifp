#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
docker compose exec headscale headscale users create workers || true
docker compose exec headscale headscale users create admin || true
# Create API key for fleet-api automation (store in .env as HEADSCALE_API_KEY)
docker compose exec headscale headscale apikeys create --expiration 365d
echo "Put the key into deploy/.env as HEADSCALE_API_KEY and recreate fleet-api."
