#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck disable=SC1091
source .env
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${BACKUP_DIR:-./backups}/$STAMP"
mkdir -p "$OUT"
docker compose exec -T postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" > "$OUT/postgres.sql"
docker compose exec -T minio mc mirror --overwrite local/"$MINIO_BUCKET_MODELS" /tmp/models-mirror 2>/dev/null || true
echo "Backup written to $OUT"
