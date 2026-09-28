#!/usr/bin/env bash
# Phase 0: bootstrap Ubuntu 24.04 VPS for Tesla LLM control plane.
set -euo pipefail

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y ca-certificates curl gnupg ufw jq openssl

# Docker
if ! command -v docker >/dev/null 2>&1; then
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  . /etc/os-release
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update
  apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin docker-buildx-plugin
fi

# Firewall
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp
# NATS for agents (token auth)
ufw allow 4222/tcp
# Headscale WireGuard / DERP
ufw allow 3478/udp
ufw allow 41641/udp
ufw --force enable

# Deploy user
if ! id -u tesla >/dev/null 2>&1; then
  useradd -m -s /bin/bash tesla
  usermod -aG docker tesla
fi

mkdir -p /opt/tesla-llm-ifp
chown tesla:tesla /opt/tesla-llm-ifp

cat <<'EOF'
=== DNS checklist (A records → this VPS public IP) ===
  api.teslant.ru
  fleet.teslant.ru
  chat.teslant.ru
  grafana.teslant.ru
  models.teslant.ru
  registry.teslant.ru
  hs.teslant.ru

Next:
  1. Clone repo into /opt/tesla-llm-ifp
  2. cp deploy/.env.example deploy/.env && edit secrets
  3. cd deploy && docker compose up -d
EOF
