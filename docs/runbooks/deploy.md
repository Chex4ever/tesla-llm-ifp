# Runbook: Deploy control plane

## First deploy

```bash
sudo ./deploy/scripts/bootstrap-vps.sh
# clone repo to /opt/tesla-llm-ifp
cd /opt/tesla-llm-ifp
cp deploy/.env.example deploy/.env
# edit secrets + PUBLIC_* URLs + PUBLIC_NATS_URL=nats://<VPS_IP>:4222
cd deploy
docker compose up -d --build
```

## Publish agent binaries

On a build machine:

```bash
make build-agent-windows build-agent-linux
# copy bin/* to VPS, then:
./deploy/scripts/upload-agent.sh
```

## Smoke checks

```bash
curl -fsS https://fleet.teslant.ru/api/v1/stats   # 401 without auth is ok for /stats? needs auth
curl -fsS https://api.teslant.ru/healthz
curl -fsS https://chat.teslant.ru/
curl -fsS https://grafana.teslant.ru/login
```

Login Fleet → Add PC → enroll Windows agent → assign model → chat via Open WebUI.
