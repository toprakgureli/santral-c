#!/usr/bin/env bash
# Runs deploy/deploy.sh against a stand-in server: sudo, systemctl, curl,
# pg_dump, psql, df, npm, go and nginx are small scripts, the repository is
# a local clone. Scenarios: a good release, one that does not come up
# (rollback), a config that fails the check, local changes on the server, a
# start that is still migrating, a disk too full for the database copy, a
# copy that fails half way, and an npm package that edits the unit file.
set -u
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.autocrlf GIT_CONFIG_VALUE_0=false
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/w/bin" && cd "$T/w"

# ---- stubs ---------------------------------------------------------------
cat > bin/sudo <<'EOF'
#!/usr/bin/env bash
while [ $# -gt 0 ]; do
  case "$1" in
    -n) shift ;;
    -u) shift 2 ;;
    *) break ;;
  esac
done
[ "$1" = "true" ] && exit 0
exec "$@"
EOF
cat > bin/systemctl <<'EOF'
#!/usr/bin/env bash
echo "systemctl $*" >> "$W/log"
if [ "$1" = "restart" ]; then
  # The service is healthy when the installed binary says so; a MIGRATE
  # binary is still migrating for a few checks first.
  if grep -q MIGRATE "$APP_DIR/santral" 2>/dev/null; then echo down > "$W/health"; echo 3 > "$W/migrate_left"
  elif grep -q GOOD "$APP_DIR/santral" 2>/dev/null; then echo up > "$W/health"; else echo down > "$W/health"; fi
fi
EOF
cat > bin/journalctl <<'EOF'
#!/usr/bin/env bash
echo "(journal lines)"
EOF
cat > bin/curl <<'EOF'
#!/usr/bin/env bash
if [ "$(cat "$W/health" 2>/dev/null)" = "up" ]; then echo '{"status":"ok"}'; exit 0; fi
exit 22
EOF
cat > bin/nginx <<'EOF'
#!/usr/bin/env bash
echo "nginx $*" >> "$W/log"
EOF
cat > bin/pg_dump <<'EOF'
#!/usr/bin/env bash
# Counts the copies on disk when it starts (its own file is already there).
ls "$BACKUP_DIR"/pre-deploy-*.dump 2>/dev/null | wc -l | tr -d ' ' > "$W/dumps_at_start"
echo "PGDMP"
if [ -f "$W/dumpfail" ]; then echo "pg_dump: error: connection lost" >&2; exit 1; fi
EOF
cat > bin/psql <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *pg_stat_activity*)
    left=$(cat "$W/migrate_left" 2>/dev/null || echo 0)
    if [ "$left" -gt 0 ]; then
      echo $((left - 1)) > "$W/migrate_left"
      [ "$left" = 1 ] && echo up > "$W/health"
      echo 1
    else
      echo 0
    fi ;;
  *pg_database_size*) echo 2048 ;;
esac
EOF
cat > bin/df <<'EOF'
#!/usr/bin/env bash
echo "Filesystem 1024-blocks Used Available Capacity Mounted on"
echo "/dev/root 100000000 1 $(cat "$W/free_kb" 2>/dev/null || echo 90000000) 1% /"
EOF
cat > bin/install <<'EOF'
#!/usr/bin/env bash
dir=0; args=()
while [ $# -gt 0 ]; do
  case "$1" in
    -d) dir=1; shift ;;
    -m|-o|-g) shift 2 ;;
    *) args+=("$1"); shift ;;
  esac
done
if [ "$dir" = 1 ]; then mkdir -p "${args[@]}"; else cp "${args[0]}" "${args[1]}"; fi
EOF
cat > bin/rsync <<'EOF'
#!/usr/bin/env bash
src="${@: -2:1}"; dst="${@: -1}"; mkdir -p "$dst"
for a in "$@"; do [ "$a" = "--delete" ] && rm -rf "$dst"/*; done
cp -r "$src". "$dst"
EOF
cat > bin/npm <<'EOF'
#!/usr/bin/env bash
echo "npm $*" >> "$W/log"
# A package that runs code during the build and edits the unit file.
[ -f "$W/evil" ] && echo "ExecStartPre=/bin/sh -c 'evil'" >> ../deploy/santral.service
mkdir -p dist && echo "<html>$VITE_BUILD_ID $(git rev-parse --short HEAD)</html>" > dist/index.html
EOF
cat > bin/fakego <<'EOF'
#!/usr/bin/env bash
# "go build ... -o OUT": writes a stand-in binary whose behaviour comes from
# the RELEASE file in the checkout.
out=""; while [ $# -gt 0 ]; do [ "$1" = "-o" ] && out="$2"; shift; done
kind=$(cat ../RELEASE)
cat > "$out" <<BIN
#!/usr/bin/env bash
# $kind
if [ "\$1" = "-check-config" ]; then
  if [ "$kind" = "BADCONFIG" ]; then echo "HATA auth.secret: boş bırakılmış"; exit 1; fi
  echo "config.yml uygun"; exit 0
fi
BIN
chmod +x "$out"
EOF
chmod +x bin/*
export PATH="$T/w/bin:$PATH" W="$T/w"

# ---- a repository with deploy/ from the real one ------------------------
git init -q --bare origin.git
git clone -q origin.git seed 2>/dev/null
mkdir -p seed/deploy/nginx seed/backend seed/frontend && touch seed/backend/.keep seed/frontend/.keep
cp "$REPO/deploy/deploy.sh" "$REPO/deploy/santral.service" seed/deploy/
cp "$REPO/deploy/nginx/"*.conf seed/deploy/nginx/
echo GOOD > seed/RELEASE
(cd seed && git add -A && git -c user.name=t -c user.email=t@t commit -qm v1 && git push -q origin HEAD:main 2>/dev/null)
git clone -q -b main origin.git app 2>/dev/null
export APP_DIR="$T/w/app" WEB_ROOT="$T/w/www" BACKUP_DIR="$T/w/backups" UNIT="$T/w/santral.service" GO="$T/w/bin/fakego" HEALTH_WAIT=4 STATE_DIR="$T/w/state"
printf '#!/usr/bin/env bash\n# GOOD (old)\n' > app/santral; chmod +x app/santral
cp seed/deploy/santral.service "$UNIT"
mkdir -p www/assets && echo old > www/index.html && echo "old page" > www/assets/Calls-old.js
# A file the previous version stopped using long ago.
echo "ancient" > www/assets/Teams-ancient.js && touch -d "10 days ago" www/assets/Teams-ancient.js
# Copies from earlier deploys, more than the ten that are kept.
mkdir -p backups
for i in $(seq 1 12); do echo old > "backups/pre-deploy-2026010$((i % 10))-0000$i-old$i.dump"; touch -d "$((20 + i)) days ago" "backups/pre-deploy-2026010$((i % 10))-0000$i-old$i.dump"; done

release() { (cd seed && echo "$1" > RELEASE && echo "$1 $RANDOM" >> deploy/santral.service.note && git add -A && git -c user.name=t -c user.email=t@t commit -qm "$1" && git push -q origin HEAD:main 2>/dev/null); }
seedsha() { (cd seed && git rev-parse --short HEAD); }
run() { bash app/deploy/deploy.sh > "$W/out" 2>&1; echo $?; }
pass=0; failn=0
check() { if eval "$2"; then echo "PASS $1"; pass=$((pass+1)); else echo "FAIL $1"; sed 's/^/    /' "$W/out"; failn=$((failn+1)); fi; }

# 1. a good release goes live, the old binary is kept, the panel is published
release GOOD
good1=$(seedsha)
code=$(run)
check "good release exits 0" '[ "$code" = 0 ]'
check "new binary installed" 'grep -q "# GOOD$" app/santral'
check "previous binary kept" 'grep -q "GOOD (old)" app/santral.prev'
check "panel published" 'grep -q "<html>" www/index.html'
check "previous page files kept for open panels" '[ -f www/assets/Calls-old.js ]'
check "page files unused for a week removed" '[ ! -f www/assets/Teams-ancient.js ]'
check "database copied first" 'ls backups/pre-deploy-*.dump >/dev/null 2>&1'
check "old copies pruned before the dump" '[ "$(cat dumps_at_start)" -le 10 ]'
check "ten copies kept" '[ "$(ls backups/pre-deploy-*.dump | wc -l)" -eq 10 ]'
check "npm install scripts do not run" 'grep -q "npm ci --ignore-scripts" log'
check "deployed commit written down" '[ "$(cat state/deployed-sha)" = "$good1" ]'

# 2. a release that does not come up: the previous binary goes back in
release BAD
bad=$(seedsha)
code=$(run)
check "failed release exits 1" '[ "$code" = 1 ]'
check "previous binary restored" 'grep -q "# GOOD$" app/santral'
check "panel left as it was" '! grep -q "$bad" www/index.html'
check "logs printed" 'grep -q "journal lines" out'
check "service healthy again" '[ "$(cat health)" = up ]'
check "checkout back on the running commit" '[ "$(cd app && git rev-parse --short HEAD)" = "$good1" ]'
check "deployed commit unchanged by the rollback" '[ "$(cat state/deployed-sha)" = "$good1" ]'

# 3. a config the new version refuses: nothing switches. The running
# commit is the last good one, not the one that was rolled back.
release BADCONFIG
before=$(md5sum app/santral | cut -d" " -f1)
restarts=$(grep -c "restart santral" log)
code=$(run)
check "bad config exits 1" '[ "$code" = 1 ]'
check "binary untouched" '[ "$(md5sum app/santral | cut -d" " -f1)" = "$before" ]'
check "no restart" '[ "$(grep -c "restart santral" log)" = "$restarts" ]'
check "reason shown" 'grep -q "auth.secret" out'
check "bad config: the message reaches the end" 'grep -q "config.yml needs the changes" out'
check "bad config: new build removed" '[ ! -f app/santral.new ]'
check "running commit read from the last good deploy" 'grep -q "running $good1" out'

# 4. local changes on the server stop the deploy
release GOOD
echo "# hand edit" >> app/deploy/santral.service
code=$(run)
check "local changes stop it" '[ "$code" = 1 ] && grep -q "local changes" out'
(cd app && git checkout -q -- .)

# 5. a start that is still migrating after the health wait is waited for
release MIGRATE
code=$(run)
check "migrating start is waited for" '[ "$code" = 0 ] && grep -q "still migrating" out'
check "migrated release stays" 'grep -q "# MIGRATE$" app/santral'
release GOOD
code=$(run)
check "next release after the migration" '[ "$code" = 0 ]'

# 6. a disk too full for the copy: refused before anything switches
release GOOD
before=$(md5sum app/santral | cut -d" " -f1)
copies=$(ls backups/pre-deploy-*.dump | wc -l)
echo 1000 > free_kb
code=$(run)
rm -f free_kb
check "full disk refused" '[ "$code" = 1 ] && grep -q "not enough disk space" out'
check "full disk: binary untouched" '[ "$(md5sum app/santral | cut -d" " -f1)" = "$before" ]'
check "full disk: no new copy" '[ "$(ls backups/pre-deploy-*.dump | wc -l)" -le "$copies" ]'
check "full disk: new build removed" '[ ! -f app/santral.new ]'

# 7. a copy that fails half way is deleted and nothing switches
newest_before=$(ls -1t backups/pre-deploy-*.dump | head -n 1)
touch dumpfail
code=$(run)
rm -f dumpfail
check "failed copy stops the deploy" '[ "$code" = 1 ] && grep -q "could not be copied" out'
check "half-written copy deleted" '[ "$(ls -1t backups/pre-deploy-*.dump | head -n 1)" = "$newest_before" ]'
check "failed copy: binary untouched" '[ "$(md5sum app/santral | cut -d" " -f1)" = "$before" ]'

# 8. the unit comes from the commit, not from the working tree an npm
# package could edit
(cd seed && echo "# unit v2" >> deploy/santral.service && git add -A && git -c user.name=t -c user.email=t@t commit -qm unit && git push -q origin HEAD:main 2>/dev/null)
touch evil
code=$(run)
rm -f evil
check "unit release exits 0" '[ "$code" = 0 ]'
check "unit taken from the commit" 'grep -q "# unit v2" "$UNIT"'
check "edited working tree ignored" '! grep -q evil "$UNIT"'
(cd app && git checkout -q -- .)

echo "deploy.sh: $pass passed, $failn failed"
[ "$failn" = 0 ]
