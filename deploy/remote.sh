#!/bin/bash
set -euo pipefail

if ! command -v curl >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y -qq curl
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y curl
  elif command -v yum >/dev/null 2>&1; then
    yum install -y curl
  fi
fi

if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi

if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
  firewall-cmd --permanent --add-service=http
  firewall-cmd --permanent --add-service=https
  firewall-cmd --reload
fi

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
  ufw allow 80/tcp
  ufw allow 443/tcp
fi

cd /opt/carcasever
chmod +x server
mkdir -p certs acme

if [ -s certs/fullchain.pem ]; then
  cat Caddyfile > Caddyfile.runtime
else
  cat Caddyfile.bootstrap > Caddyfile.runtime
fi

pull_images() {
  docker compose -f docker-compose.prod.yml pull
}

pulled=0
for _ in 1 2 3 4 5; do
  if pull_images; then
    pulled=1
    break
  fi
  sleep 15
done
if [ "$pulled" != 1 ]; then
  if [ ! -f /etc/docker/daemon.json ]; then
    mkdir -p /etc/docker
    printf '%s\n' '{"registry-mirrors":["https://mirror.gcr.io"]}' > /etc/docker/daemon.json
    systemctl restart docker
    sleep 3
  fi
  pull_images
fi

docker compose -f docker-compose.prod.yml up -d --remove-orphans
docker compose -f docker-compose.prod.yml restart app

for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
  if curl -fsS -H "Host: 194.87.236.111" http://127.0.0.1/api/health >/dev/null; then
    break
  fi
  sleep 3
done
if ! curl -fsS -H "Host: 194.87.236.111" http://127.0.0.1/api/health >/dev/null; then
  echo "health check failed" >&2
  docker compose -f docker-compose.prod.yml logs --tail 100
  exit 1
fi

if [ ! -s certs/fullchain.pem ]; then
  if ! command -v certbot >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq certbot
  fi
  certbot certonly --webroot -w /opt/carcasever/acme -d carcasever.online \
    --non-interactive --agree-tos --register-unsafely-without-email
  cp -L /etc/letsencrypt/live/carcasever.online/fullchain.pem certs/fullchain.pem
  cp -L /etc/letsencrypt/live/carcasever.online/privkey.pem certs/privkey.pem
  cat Caddyfile > Caddyfile.runtime
  docker compose -f docker-compose.prod.yml restart caddy
fi

cat > /etc/cron.d/carcasever-cert <<'EOF'
0 3 * * * root certbot renew --quiet --deploy-hook "cp -L /etc/letsencrypt/live/carcasever.online/fullchain.pem /opt/carcasever/certs/fullchain.pem && cp -L /etc/letsencrypt/live/carcasever.online/privkey.pem /opt/carcasever/certs/privkey.pem && docker compose -f /opt/carcasever/docker-compose.prod.yml restart caddy"
EOF

for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -fsS https://carcasever.online/api/health >/dev/null; then
    exit 0
  fi
  sleep 3
done

echo "https check failed" >&2
docker compose -f docker-compose.prod.yml logs caddy --tail 80
exit 1
