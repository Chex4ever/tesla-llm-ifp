#!/usr/bin/env bash
# Minimal Docker install for Crow's Nest (self-signed TLS path).
set -euo pipefail
if [[ "$(id -u)" -ne 0 ]]; then
  echo "need root" >&2
  exit 1
fi
export DEBIAN_FRONTEND=noninteractive

docker_ok() {
  command -v docker >/dev/null 2>&1 \
    && docker compose version >/dev/null 2>&1 \
    && docker info >/dev/null 2>&1
}

if ! docker_ok; then
  apt-get update
  apt-get install -y ca-certificates curl gnupg openssl
  install -m 0755 -d /etc/apt/keyrings
  if [[ ! -f /etc/apt/keyrings/docker.gpg ]]; then
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg
  fi
  . /etc/os-release
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update
  apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin docker-buildx-plugin
fi

# openssl needed for self-signed nest certs
if ! command -v openssl >/dev/null 2>&1; then
  apt-get update
  apt-get install -y openssl
fi

# CLI can exist while dockerd is stopped — always start/enable.
if command -v systemctl >/dev/null 2>&1; then
  systemctl enable docker >/dev/null 2>&1 || true
  systemctl start docker
elif command -v service >/dev/null 2>&1; then
  service docker start
fi

# Wait until the daemon answers (fresh install / slow VPS).
for i in $(seq 1 30); do
  if docker info >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! docker info >/dev/null 2>&1; then
  echo "docker daemon did not become ready" >&2
  systemctl status docker --no-pager >&2 || true
  journalctl -u docker -n 40 --no-pager >&2 || true
  exit 1
fi

if command -v ufw >/dev/null 2>&1; then
  ufw allow OpenSSH || true
  ufw allow 80/tcp || true
  ufw allow 443/tcp || true
  ufw allow 443/udp || true
fi

echo "docker ready"
