#!/usr/bin/env bash
# Runs deploy/deploy.sh against a stand-in server: sudo, systemctl, curl,
# pg_dump, npm, go and nginx are small scripts, the repository is a local
# clone. Scenarios: a good release, one that does not come up (rollback),
# a config that fails the check, and local changes on the server.
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
  # The service is healthy when the installed binary says so.
  if grep -q GOOD "$APP_DIR/santral" 2>/dev/null; then echo up > "$W/health"; else echo down > "$W/health"; fi
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
echo "PGDMP"
EOF
cat > bin/install <<'EOF'
#!/usr/bin/env bash
dir="${@: -1}"; mkdir -p "$dir"
EOF
cat > bin/rsync <<'EOF'
#!/usr/bin/env bash
src="${@: -2:1}"; dst="${@: -1}"; mkdir -p "$dst"; rm -rf "$dst"/*; cp -r "$src". "$dst"
EOF
cat > bin/npm <<'EOF'
#!/usr/bin/env bash
mkdir -p dist && echo "<html>$VITE_BUILD_SHA</html>" > dist/index.html
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
export APP_DIR="$T/w/app" WEB_ROOT="$T/w/www" BACKUP_DIR="$T/w/backups" UNIT="$T/w/santral.service" GO="$T/w/bin/fakego" HEALTH_WAIT=4
printf '#!/usr/bin/env bash\n# GOOD (old)\n' > app/santral; chmod +x app/santral
cp seed/deploy/santral.service "$UNIT"
mkdir -p www && echo old > www/index.html

release() { (cd seed && echo "$1" > RELEASE && echo "$1 $RANDOM" >> deploy/santral.service.note && git add -A && git -c user.name=t -c user.email=t@t commit -qm "$1" && git push -q origin HEAD:main 2>/dev/null); }
run() { bash app/deploy/deploy.sh > "$W/out" 2>&1; echo $?; }
pass=0; failn=0
check() { if eval "$2"; then echo "PASS $1"; pass=$((pass+1)); else echo "FAIL $1"; sed 's/^/    /' "$W/out"; failn=$((failn+1)); fi; }

# 1. a good release goes live, the old binary is kept, the panel is published
release GOOD
code=$(run)
check "good release exits 0" '[ "$code" = 0 ]'
check "new binary installed" 'grep -q "# GOOD$" app/santral'
check "previous binary kept" 'grep -q "GOOD (old)" app/santral.prev'
check "panel published" 'grep -q "<html>" www/index.html'
check "database copied first" 'ls backups/pre-deploy-*.dump >/dev/null 2>&1'

# 2. a release that does not come up: the previous binary goes back in
release BAD
code=$(run)
check "failed release exits 1" '[ "$code" = 1 ]'
check "previous binary restored" 'grep -q "# GOOD$" app/santral'
check "panel left as it was" '! grep -q "$(cd app && git rev-parse --short HEAD)" www/index.html'
check "logs printed" 'grep -q "journal lines" out'
check "service healthy again" '[ "$(cat health)" = up ]'

# 3. a config the new version refuses: nothing switches
release BADCONFIG
before=$(md5sum app/santral | cut -d" " -f1)
restarts=$(grep -c "restart santral" log)
code=$(run)
check "bad config exits 1" '[ "$code" = 1 ]'
check "binary untouched" '[ "$(md5sum app/santral | cut -d" " -f1)" = "$before" ]'
check "no restart" '[ "$(grep -c "restart santral" log)" = "$restarts" ]'
check "reason shown" 'grep -q "auth.secret" out'

# 4. local changes on the server stop the deploy
release GOOD
echo "# hand edit" >> app/deploy/santral.service
code=$(run)
check "local changes stop it" '[ "$code" = 1 ] && grep -q "local changes" out'
(cd app && git checkout -q -- .)

echo "deploy.sh: $pass passed, $failn failed"
[ "$failn" = 0 ]
