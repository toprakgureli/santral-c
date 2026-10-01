#!/usr/bin/env bash
# Pull, build and switch santral-c on the server.
#
# The new version is built next to the running one and its configuration is
# checked first; the database is copied before the switch (when the disk has
# room for it); if the new version does not answer its health check within a
# minute (longer while it is still migrating the database), the previous
# binary and checkout go back in and the last log lines are printed. The
# panel is published only after the backend is healthy.
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
# How long a start that is still migrating the database is waited for.
MIGRATE_WAIT=${MIGRATE_WAIT:-1800}
UNIT=${UNIT:-/etc/systemd/system/santral.service}
# Room left free on the backup disk after the copy, in MB.
DUMP_MARGIN_MB=${DUMP_MARGIN_MB:-1024}
# Where the commit that is really running is written after each good deploy.
STATE_DIR=${STATE_DIR:-/var/lib/santral-deploy}

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

# migrating tells whether the new version is still bringing the database up
# to date: its migration connection shows in pg_stat_activity under its own
# name while it runs.
migrating() {
  local n
  n=$(sudo -u postgres psql -d "$DB_NAME" -tAc "SELECT count(*) FROM pg_stat_activity WHERE application_name = 'santral-migrate'" 2>/dev/null | tr -dc '0-9')
  [ "${n:-0}" -gt 0 ]
}

# free_kb is the space left on the file system holding a folder, in KB.
free_kb() { df -Pk "$1" | awk 'NR==2 {print $4}'; }

echo "==> Pulling origin/$BRANCH"
if [ -n "$(run_as_app git -C "$APP_DIR" status --porcelain --untracked-files=no)" ] && [ "${FORCE:-0}" != "1" ]; then
  run_as_app git -C "$APP_DIR" status --short --untracked-files=no >&2
  fail "the checkout on the server has local changes (listed above); they would be lost. Move them away, or run again with FORCE=1."
fi
# The running commit is the one the last good deploy wrote down. The
# checkout's HEAD is not: a deploy that was rolled back may have left it on
# the commit that failed.
PREV_SHA=""
if [ -f "$STATE_DIR/deployed-sha" ]; then
  PREV_SHA=$(tr -dc '0-9a-f' < "$STATE_DIR/deployed-sha")
  if [ -n "$PREV_SHA" ] && ! run_as_app git -C "$APP_DIR" cat-file -e "$PREV_SHA^{commit}" 2>/dev/null; then
    PREV_SHA=""
  fi
fi
if [ -z "$PREV_SHA" ]; then
  PREV_SHA=$(run_as_app git -C "$APP_DIR" rev-parse --short HEAD)
fi
run_as_app git -C "$APP_DIR" fetch --all --prune
run_as_app git -C "$APP_DIR" reset --hard "origin/$BRANCH"

GIT_SHA=$(run_as_app git -C "$APP_DIR" rev-parse --short HEAD)
GIT_FULL=$(run_as_app git -C "$APP_DIR" rev-parse HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
# A random tag shared by the backend and the panel of this deploy. The panel's
# files are public, so they carry this tag instead of the commit; the panel
# compares it with the backend's to see that it is up to date.
BUILD_ID=$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')
echo "==> Version $GIT_SHA ($BUILD_TIME), running $PREV_SHA"

# root installs the systemd unit, so it is taken from the commit itself and
# read before the build: npm packages run as the santral user and could
# change the working tree, never this copy.
UNIT_NEW=$(mktemp)
trap 'rm -f "$UNIT_NEW"' EXIT
run_as_app git -C "$APP_DIR" show "$GIT_FULL:deploy/santral.service" > "$UNIT_NEW"

echo "==> Building backend"
LDFLAGS="-X main.version=$GIT_SHA -X main.buildTime=$BUILD_TIME -X main.buildID=$BUILD_ID"
run_as_app bash -c "cd '$APP_DIR/backend' && '$GO' build -trimpath -ldflags '$LDFLAGS' -o '$APP_DIR/santral.new' ./cmd/santral"

echo "==> Checking config.yml with the new version"
if ! run_as_app "$APP_DIR/santral.new" -check-config -config "$APP_DIR/config.yml"; then
  rm -f "$APP_DIR/santral.new"
  fail "config.yml needs the changes listed above. Nothing was switched; the running version keeps running."
fi

echo "==> Building frontend"
# Install scripts of npm packages do not run: the build needs none of them
# (esbuild finds its program in its platform package without its own).
run_as_app bash -c "cd '$APP_DIR/frontend' && npm ci --ignore-scripts && VITE_BUILD_ID='$BUILD_ID' npm run build"

echo "==> Copying the database before the switch"
sudo install -d -m 700 -o postgres -g postgres "$BACKUP_DIR"
# Keep the last ten of these local copies (nine old ones and the new one).
# The old ones go first, so they make room for the new copy; the regular
# backups go to Drive.
sudo -u postgres sh -c "ls -1t '$BACKUP_DIR'/pre-deploy-*.dump 2>/dev/null | tail -n +10 | xargs -r rm -f"
# The copy is about as big as the last one; without one, the database's
# size is the estimate (the copy is compressed, so usually smaller).
LAST_DUMP=$(sudo -u postgres sh -c "ls -1t '$BACKUP_DIR'/pre-deploy-*.dump 2>/dev/null | head -n 1")
if [ -n "$LAST_DUMP" ]; then
  EST_KB=$(sudo -u postgres du -k "$LAST_DUMP" | awk '{print $1}')
else
  EST_KB=$(sudo -u postgres psql -tAc "SELECT pg_database_size('$DB_NAME') / 1024" | tr -dc '0-9')
fi
EST_KB=${EST_KB:-0}
NEED_KB=$((EST_KB + EST_KB / 2 + DUMP_MARGIN_MB * 1024))
FREE_KB=$(free_kb "$BACKUP_DIR")
if [ "${FREE_KB:-0}" -lt "$NEED_KB" ]; then
  rm -f "$APP_DIR/santral.new"
  fail "not enough disk space for the database copy in $BACKUP_DIR: $((${FREE_KB:-0} / 1024)) MB free, about $((NEED_KB / 1024)) MB needed (the copy plus a $DUMP_MARGIN_MB MB margin). Free some space (old copies: sudo -u postgres ls -lh $BACKUP_DIR) and run again. Nothing was switched; the running version keeps running."
fi
DUMP="$BACKUP_DIR/pre-deploy-$(date +%Y%m%d-%H%M%S)-$PREV_SHA.dump"
if ! sudo -u postgres sh -c "umask 077 && pg_dump -Fc '$DB_NAME' > '$DUMP'"; then
  # A half-written copy is worthless and only fills the disk.
  sudo -u postgres rm -f "$DUMP"
  rm -f "$APP_DIR/santral.new"
  fail "the database could not be copied (see above). Nothing was switched; the running version keeps running."
fi
echo "    $DUMP"

UNIT_CHANGED=0
if ! sudo cmp -s "$UNIT_NEW" "$UNIT"; then
  UNIT_CHANGED=1
  echo "==> Installing the changed systemd unit"
  sudo cp "$UNIT" "$UNIT.prev" 2>/dev/null || true
  sudo install -m 644 "$UNIT_NEW" "$UNIT"
  sudo systemctl daemon-reload
fi

echo "==> Switching to $GIT_SHA"
if [ -f "$APP_DIR/santral" ]; then
  sudo cp -p "$APP_DIR/santral" "$APP_DIR/santral.prev"
fi
sudo mv -f "$APP_DIR/santral.new" "$APP_DIR/santral"
sudo systemctl restart santral

echo "==> Waiting for the health check (up to ${HEALTH_WAIT}s, longer while the database is migrating)"
ok=0
waited=0
while :; do
  if health; then
    ok=1
    break
  fi
  if [ "$waited" -ge "$HEALTH_WAIT" ]; then
    # A long migration (an index on a big table) is waited for: stopping
    # it half way would only make the next start do it again.
    if [ "$waited" -lt "$MIGRATE_WAIT" ] && migrating; then
      if [ $(((waited - HEALTH_WAIT) % 30)) -lt 2 ]; then
        echo "    still migrating the database (${waited}s so far, up to ${MIGRATE_WAIT}s)"
      fi
    else
      break
    fi
  fi
  sleep 2
  waited=$((waited + 2))
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
    # The checkout goes back too, so it matches what runs again.
    run_as_app git -C "$APP_DIR" reset -q --hard "$PREV_SHA" || true
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

sudo install -d -m 755 "$STATE_DIR"
echo "$GIT_SHA" | sudo tee "$STATE_DIR/deployed-sha" > /dev/null

echo "==> Done: $GIT_SHA is running"
