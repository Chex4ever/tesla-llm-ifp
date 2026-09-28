#!/usr/bin/env bash
# Upload built agent binaries to MinIO agents bucket.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
source .env
mc alias set local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" 2>/dev/null || \
  docker compose exec -T minio mc alias set local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"

upload() {
  local src="$1" dest="$2"
  docker compose run --rm -v "$ROOT/bin:/binaries:ro" --entrypoint /bin/sh minio-init -c \
    "mc alias set local http://minio:9000 \$MINIO_ROOT_USER \$MINIO_ROOT_PASSWORD && mc cp /binaries/$src local/\$MINIO_BUCKET_AGENTS/$dest"
}

upload tesla-agent.exe windows/tesla-agent.exe
upload tesla-agent linux/tesla-agent
echo "Agents uploaded."
