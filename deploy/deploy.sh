#!/usr/bin/env bash
# Pull, build and restart santral-c on the server.
#
# One-time setup (see DEPLOY.md) must already be done: repo cloned to
# /opt/santral-c, config.yml in place, systemd unit installed, nginx configured.
#
# Usage: /opt/santral-c/deploy/deploy.sh
set -euo pipefail

APP_DIR=/opt/santral-c
WEB_ROOT=/var/www/santral-c
BRANCH=${BRANCH:-main}

cd "$APP_DIR"

echo "==> Pulling $BRANCH"
git fetch --all --prune
git reset --hard "origin/$BRANCH"

echo "==> Building backend"
cd "$APP_DIR/backend"
go build -o "$APP_DIR/santral" ./cmd/santral

echo "==> Building frontend"
cd "$APP_DIR/frontend"
npm ci
npm run build

echo "==> Publishing frontend to $WEB_ROOT"
sudo mkdir -p "$WEB_ROOT"
sudo rsync -a --delete "$APP_DIR/frontend/dist/" "$WEB_ROOT/"

echo "==> Restarting backend"
sudo systemctl restart santral

echo "==> Reloading nginx"
sudo nginx -t && sudo systemctl reload nginx

echo "==> Health check"
sleep 2
curl -fsS http://127.0.0.1:8090/healthz && echo " OK"
echo "==> Done"
