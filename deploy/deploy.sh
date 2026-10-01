#!/usr/bin/env bash
# Pull, build and switch santral-c on the server.
#
# The new version is built next to the running one and its configuration is
# checked first; the database is copied before the switch; if the new
# version does not answer its health check within a minute, the previous
# binary goes back in and the last log lines are printed. The panel is
# published only after the backend is healthy.
#
# Run as a sudo-capable user (e.g. ubuntu), not as the 'santral' service
# user:  BRANCH=main /opt/santral-c/deploy/deploy.sh
set -euo pipefail

APP_DIR=${APP_DIR:-/opt/santral-c}
WEB_ROOT=${WEB_ROOT:-/var/www/santral-c}
BRANCH=${BRANCH:-main}
GO=${GO:-/usr/local/go/bin/go}
PORT=${PORT:-8090}
DB_NAME=${DB_NAME:-santral}
BACKUP_DIR=${BACKUP_DIR:-/var/backups/santral}
HEALTH_WAIT=${HEALTH_WAIT:-60}
UNIT=${UNIT:-/etc/systemd/system/santral.service}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

# This script uses sudo for system steps and the 'santral' user for builds.
# Running it AS santral (which is not in sudoers) fails at the first sudo.
if ! sudo -n true 2>/dev/null; then
  echo "ERROR: deploy.sh needs passwordless sudo. Run it as a sudo-capable user" >&2
  echo "       (e.g. ubuntu): 'BRANCH=main $APP_DIR/deploy/deploy.sh'." >&2
  echo "       Do NOT run it as the 'santral' user or with 'sudo -u santral'." >&2
  exit 1
fi

# Run a command as the service user with its HOME set (so the Go and npm caches
# land in a writable place).
run_as_app() { sudo -u santral env HOME="$APP_DIR" "$@"; }

health() {
  curl -fsS --max-time 3 "http://127.0.0.1:$PORT/healthz" 2>/dev/null | grep -q '"status":"ok"'
}

echo "==> Pulling origin/$BRANCH"
if [ -n "$(run_as_app git -C "$APP_DIR" status --porcelain --untracked-files=no)" ] && [ "${FORCE:-0}" != "1" ]; then
  run_as_app git -C "$APP_DIR" status --short --untracked-files=no >&2
  fail "the checkout on the server has local changes (listed above); they would be lost. Move them away, or run again with FORCE=1."
fi
PREV_SHA=$(run_as_app git -C "$APP_DIR" rev-parse --short HEAD)
run_as_app git -C "$APP_DIR" fetch --all --prune
run_as_app git -C "$APP_DIR" reset --hard "origin/$BRANCH"

GIT_SHA=$(run_as_app git -C "$APP_DIR" rev-parse --short HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
# A random tag shared by the backend and the panel of this deploy. The panel's
# files are public, so they carry this tag instead of the commit; the panel
# compares it with the backend's to see that it is up to date.
BUILD_ID=$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')
echo "==> Version $GIT_SHA ($BUILD_TIME), running $PREV_SHA"

echo "==> Building backend"
LDFLAGS="-X main.version=$GIT_SHA -X main.buildTime=$BUILD_TIME -X main.buildID=$BUILD_ID"
run_as_app bash -c "cd '$APP_DIR/backend' && '$GO' build -trimpath -ldflags '$LDFLAGS' -o '$APP_DIR/santral.new' ./cmd/santral"

echo "==> Checking config.yml with the new version"
if ! run_as_app "$APP_DIR/santral.new" -check-config -config "$APP_DIR/config.yml"; then
  rm -f "$APP_DIR/santral.new"
  fail "config.yml needs the changes listed above. Nothing was switched; the running version keeps running."
fi

echo "==> Building frontend"
run_as_app bash -c "cd '$APP_DIR/frontend' && npm ci && VITE_BUILD_ID='$BUILD_ID' npm run build"

echo "==> Copying the database before the switch"
sudo install -d -m 700 -o postgres -g postgres "$BACKUP_DIR"
DUMP="$BACKUP_DIR/pre-deploy-$(date +%Y%m%d-%H%M%S)-$PREV_SHA.dump"
sudo -u postgres sh -c "umask 077 && pg_dump -Fc '$DB_NAME' > '$DUMP'"
# Keep the last ten of these local copies; the regular backups go to Drive.
sudo -u postgres sh -c "ls -1t '$BACKUP_DIR'/pre-deploy-*.dump 2>/dev/null | tail -n +11 | xargs -r rm -f"
echo "    $DUMP"

UNIT_CHANGED=0
if ! sudo cmp -s "$APP_DIR/deploy/santral.service" "$UNIT"; then
  UNIT_CHANGED=1
  echo "==> Installing the changed systemd unit"
  sudo cp "$UNIT" "$UNIT.prev" 2>/dev/null || true
  sudo cp "$APP_DIR/deploy/santral.service" "$UNIT"
  sudo systemctl daemon-reload
fi

echo "==> Switching to $GIT_SHA"
if [ -f "$APP_DIR/santral" ]; then
  sudo cp -p "$APP_DIR/santral" "$APP_DIR/santral.prev"
fi
sudo mv -f "$APP_DIR/santral.new" "$APP_DIR/santral"
sudo systemctl restart santral

echo "==> Waiting for the health check (up to ${HEALTH_WAIT}s)"
ok=0
for _ in $(seq 1 $((HEALTH_WAIT / 2))); do
  if health; then
    ok=1
    break
  fi
  sleep 2
done

if [ "$ok" != "1" ]; then
  echo "!!! The new version did not come up. Last log lines:" >&2
  sudo journalctl -u santral -n 60 --no-pager >&2 || true
  if [ -f "$APP_DIR/santral.prev" ]; then
    echo "==> Going back to $PREV_SHA" >&2
    sudo mv -f "$APP_DIR/santral.prev" "$APP_DIR/santral"
    if [ "$UNIT_CHANGED" = "1" ] && [ -f "$UNIT.prev" ]; then
      sudo mv -f "$UNIT.prev" "$UNIT"
      sudo systemctl daemon-reload
    fi
    sudo systemctl restart santral
    sleep 3
    if health; then
      echo "    $PREV_SHA is running again. The panel files were not changed." >&2
    else
      echo "    The previous version does not answer either. If the new version changed" >&2
      echo "    the database, restore the copy taken before the switch:" >&2
      echo "    $DUMP" >&2
    fi
  fi
  fail "deploy of $GIT_SHA failed"
fi

echo "==> Publishing frontend to $WEB_ROOT"
sudo mkdir -p "$WEB_ROOT"
# The previous version's page files stay for a week: panels open since
# before this deploy load their pages from them until they reload. Files
# of the current version get a fresh time on every deploy, so only those
# no version has used for seven days are removed.
sudo rsync -a "$APP_DIR/frontend/dist/" "$WEB_ROOT/"
sudo find "$WEB_ROOT/assets" -type f -mtime +7 -delete 2>/dev/null || true

if ! run_as_app git -C "$APP_DIR" diff --quiet "$PREV_SHA" "$GIT_SHA" -- deploy/nginx; then
  echo "!!! deploy/nginx changed in this update. Install the site file again and test it:" >&2
  echo "    sudo cp $APP_DIR/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com" >&2
  echo "    sudo nginx -t && sudo systemctl reload nginx" >&2
  echo "    The file serves HTTPS with the Cloudflare Origin Certificate in /etc/ssl/cloudflare/." >&2
  echo "    If those files are missing, nginx -t fails, nothing is reloaded and the old file keeps" >&2
  echo "    running; install the certificate first (DEPLOY.md, section 7)." >&2
fi
if ! run_as_app git -C "$APP_DIR" diff --quiet "$PREV_SHA" "$GIT_SHA" -- deploy/observability; then
  echo "!!! deploy/observability changed; restart the monitoring stack:" >&2
  echo "    sudo docker compose -f $APP_DIR/deploy/observability/docker-compose.yml up -d" >&2
fi

echo "==> Reloading nginx"
if sudo nginx -t 2>/dev/null; then
  sudo systemctl reload nginx
else
  echo "!!! nginx -t failed; nginx was not reloaded and keeps its current settings:" >&2
  sudo nginx -t >&2 || true
fi

echo "==> Done: $GIT_SHA is running"
