#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-https://fleet.teslant.ru}"
API="${2:-https://api.teslant.ru}"
echo "Checking $API/healthz"
curl -fsS "$API/healthz" | grep -q ok
echo "Checking fleet login page"
curl -fsSI "$ROOT" | head -n1
echo "OK"
