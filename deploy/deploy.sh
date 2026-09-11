#!/usr/bin/env bash
# Pull, build and restart santral-c on the server.
#
# Run as a sudo-capable user (e.g. ubuntu). The repo and builds are owned by the
# 'santral' service user; system steps use sudo.
#
# Usage: /opt/santral-c/deploy/deploy.sh
set -euo pipefail

APP_DIR=/opt/santral-c
WEB_ROOT=/var/www/santral-c
BRANCH=${BRANCH:-main}
GO=${GO:-/usr/local/go/bin/go}

# This script must run as a sudo-capable user (e.g. ubuntu); it switches to the
# 'santral' service user for builds and uses sudo for system steps. Running it
# AS santral (which is not in sudoers) fails at the first sudo and silently skips
# the rebuild/restart, so guard against it with a clear message.
if ! sudo -n true 2>/dev/null; then
  echo "ERROR: deploy.sh needs passwordless sudo. Run it as a sudo-capable user" >&2
  echo "       (e.g. ubuntu): 'BRANCH=main $APP_DIR/deploy/deploy.sh'." >&2
  echo "       Do NOT run it as the 'santral' user or with 'sudo -u santral'." >&2
  exit 1
fi

# Run a command as the service user with its HOME set (so the Go and npm caches
# land in a writable place).
run_as_app() { sudo -u santral env HOME="$APP_DIR" "$@"; }

echo "==> Pulling origin/$BRANCH"
run_as_app git -C "$APP_DIR" fetch --all --prune
run_as_app git -C "$APP_DIR" reset --hard "origin/$BRANCH"

echo "==> Building backend"
run_as_app bash -c "cd '$APP_DIR/backend' && '$GO' build -o '$APP_DIR/santral' ./cmd/santral"

echo "==> Building frontend"
run_as_app bash -c "cd '$APP_DIR/frontend' && npm ci && npm run build"

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
