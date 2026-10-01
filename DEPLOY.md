# Deploying santral-c to cm.toprakgureli.com

One Ubuntu host runs everything: nginx serves the built panel and forwards
`/api` to the Go backend on the loopback address, PostgreSQL and Redis run
on the same machine, and Cloudflare provides DNS and HTTPS in front. The
monitoring stack (Prometheus, Grafana, Jaeger) runs on the same host in
Docker and listens on 127.0.0.1 only.

```
browser ──HTTPS──▶ Cloudflare ──HTTPS (Full strict)──▶ nginx ──┬─ /       panel (/var/www/santral-c)
                                                               └─ /api/  ▶ 127.0.0.1:8090 (santral)
Meta, Tally ──HTTPS──▶ Cloudflare ──▶ nginx ─ /api/v1/wa/hook, /api/v1/wa/survey (rate limited, 1 MB)
softphone ──SIP over WSS──▶ api.bulutsantralim.com   (straight from the browser, not through nginx)
santral ──every 6 hours──▶ Google Shared Drive (database copies)
you ──SSH tunnel──▶ Grafana 127.0.0.1:3000, Prometheus 127.0.0.1:9090, Jaeger 127.0.0.1:16686
```

## 0. Packages

```bash
sudo apt update
sudo apt install -y nginx postgresql redis-server git rsync curl
```

`postgresql` brings `pg_dump`, which the backups need.

Go 1.26 or newer (the module asks for its exact toolchain and Go fetches it
on the first build):

```bash
curl -fsSL https://go.dev/dl/go1.26.8.linux-amd64.tar.gz | sudo tar -C /usr/local -xz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
```

Node 20 or newer:

```bash
curl -fsSL https://deb.nodesource.com/setup_24.x | sudo -E bash -
sudo apt install -y nodejs
```

The service user:

```bash
sudo useradd --system --home /opt/santral-c --shell /usr/sbin/nologin santral
```

The user you deploy with (for example `ubuntu`) needs passwordless sudo;
`deploy.sh` refuses to run otherwise.

## 1. Firewall

Only SSH and the web ports are open. The backend listens on 127.0.0.1 only
(`app.host`), so it is never reachable from outside even without a firewall,
but the firewall is the second lock. The monitoring stack also listens on
127.0.0.1 only, so it needs no rule.

```bash
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw enable
```

If a WhatsApp number uses a webhook address already registered in Meta on
port 5001 (see section 9), open that port too:

```bash
sudo ufw allow 5001/tcp
```

## 2. Code and config

```bash
sudo git clone https://github.com/toprakgureli/santral-c.git /opt/santral-c
sudo chown -R santral:santral /opt/santral-c
sudo -u santral cp /opt/santral-c/config.prod.example.yml /opt/santral-c/config.yml
sudo chmod 600 /opt/santral-c/config.yml
sudo -u santral nano /opt/santral-c/config.yml
```

Fill every `change-me` value. Generate the random ones with:

| Key | Command | What it does |
|---|---|---|
| `auth.secret` | `openssl rand -hex 32` | signs access tokens and the steps of signing in (read section 13 before changing it) |
| `security.dataKey` | `openssl rand -hex 32` | seals every credential typed into the panel and signs survey links |
| `security.mfaKey` | `openssl rand -hex 16` | seals TOTP secrets |
| `bulutsantralim.sipKey` | `openssl rand -hex 16` | seals SIP passwords |

Keep a copy of `dataKey`, `mfaKey` and `sipKey` somewhere safe outside the
server (a password manager). Without them a database backup cannot open the
stored credentials.

Check these values in the prod template as well:

- `app.host: "127.0.0.1"` so only nginx reaches the backend.
- `auth.accessTTL: 15m`. A signed-out or deactivated user is cut off at once
  anyway; this only bounds how long a stolen token could live.
- `auth.refreshTTL: 168h`. A sign-in lasts at most a week and renewing does
  not extend it, so everyone signs in again once a week. The backend never
  allows more than seven days, whatever is written here.
- `auth.cookieSecure: true`, the site is HTTPS only.
- `security.requireMFA: true` asks everyone to set up TOTP at first sign-in.
- `security.trustedIPs`: the office's public address (see below).
- `bulutsantralim.*`: the API key comes from OIM > Bulut Santralım > Santral
  Ayarlarım; `sipDomain`, `sipWssUrl` and `stunUrl` from the PBX's WebRTC
  settings.
- `drive.*`: a Google Cloud OAuth client (Web application). Its redirect
  URI must be exactly `https://cm.toprakgureli.com/api/v1/teams/drive/callback`.
- `telemetry.otlpEndpoint: "http://127.0.0.1:4318"` sends traces to Jaeger
  (section 11); `telemetry.sampleRatio: 0.2` traces one request in five.
  Leave `otlpEndpoint` empty to turn traces off.

### Sign-in limits and the office address

Wrong passwords are counted three ways:

| Counted per | Limit | What happens |
|---|---|---|
| browser (the `santral_device` cookie) | `security.deviceFailureLimit`, default 5 | that browser waits 5 minutes |
| account | 10 | the account is locked for `security.accountLockDuration`, default 15m |
| address, only outside `security.trustedIPs` | `security.ipFailureLimit`, default 100 | the address is banned for `security.ipBanDuration`, default 15m |

All counts run over `security.attemptWindow` (15m). An account that fails
from `security.distinctIPLimit` (5) different addresses also gets those
untrusted addresses banned and is locked. Request limits on the sign-in
steps are per browser (20 a minute) and on renewal per session (30 a
minute), never per address.

Put the office's public address in `security.trustedIPs`, so a bad morning
of typos never bans the office. It takes single addresses and CIDR ranges,
separated by commas:

```yaml
security:
  trustedIPs: "203.0.113.10"           # or a range: "203.0.113.0/28"
```

`203.0.113.10` is an example; replace it with your office's address. To
find it, sign in from an office computer and look at Yönetim > Sistem
Ayarları > Giriş Kayıtları. A ban can be lifted early under Banlı IP
Adresleri on the same page.

### Checking the config

The backend checks `config.yml` before it touches the database, and a live
server (`app.development: live`) refuses to start on any `HATA` line. Once
the binary is built (section 4) you can run the same check by hand:

```bash
sudo -u santral /opt/santral-c/santral -check-config -config /opt/santral-c/config.yml
```

It prints `HATA` (stops the server) and `UYARI` (reported, the server still
starts) lines, or `config.yml uygun`. It stops on an empty, example or
too short `auth.secret` or `security.dataKey`, an empty or example `mfaKey`,
a missing `sipKey` or API key while `bulutsantralim.enabled` is true,
`accessTTL` over 24 hours, `refreshTTL` not longer than `accessTTL`,
missing database or Redis settings, `auth.cookieSecure: false` on a live
server and an unreadable `trustedIPs` entry. It warns about `app.host` not
on the loopback, an empty `trustedProxies`, the example owner password,
an `ipFailureLimit` under 50 with no `trustedIPs`, and a `refreshTTL` over
seven days. It does not connect to the database. `deploy.sh` runs it with
every new build.

Only one server may run on a database: the backend takes a PostgreSQL lock
at start, and a second one stops with "another santral server is already
running on this database".

## 3. PostgreSQL and Redis

```bash
sudo -u postgres createuser --pwprompt santral
sudo -u postgres createdb --owner santral santral
```

Type the same password you put in `database.password`. Redis works as
installed, on localhost:6379. The backend applies the migrations itself on
every start.

## 4. First build

The build is stamped with the git commit the same way `deploy.sh` does it,
so the sidebar shows which version runs:

```bash
SHA=$(sudo -u santral env HOME=/opt/santral-c git -C /opt/santral-c rev-parse --short HEAD)
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
sudo -u santral env HOME=/opt/santral-c bash -c "cd /opt/santral-c/backend && /usr/local/go/bin/go build -trimpath -ldflags '-X main.version=$SHA -X main.buildTime=$NOW' -o /opt/santral-c/santral ./cmd/santral"
sudo -u santral env HOME=/opt/santral-c bash -c "cd /opt/santral-c/frontend && npm ci && VITE_BUILD_SHA=$SHA VITE_BUILD_TIME=$NOW npm run build"
sudo -u santral /opt/santral-c/santral -check-config -config /opt/santral-c/config.yml
sudo mkdir -p /var/www/santral-c
sudo rsync -a --delete /opt/santral-c/frontend/dist/ /var/www/santral-c/
```

Fix every `HATA` line before going on.

## 5. systemd

```bash
sudo cp /opt/santral-c/deploy/santral.service /etc/systemd/system/santral.service
sudo systemctl daemon-reload
sudo systemctl enable --now santral
systemctl status santral
curl -fsS http://127.0.0.1:8090/healthz
```

`/healthz` answers 200 when PostgreSQL and Redis both answer. Asked on the
server itself it also names the parts and the running build; from outside
it says only `ok` or `down`. Logs are JSON lines with the request id, the
user and the trace id:

```bash
journalctl -u santral -f
```

The unit locks the service down. `ProtectSystem=strict` and `ProtectHome`
make the whole file system read-only to it, and `PrivateTmp` gives it a
private `/tmp`: the backend writes nothing on disk except there (the
backup dump is made there and removed after the upload). So code running
inside the service can change neither its own binary nor the deploy script.
It also runs without capabilities, with a narrowed set of system calls, at
most 2 GB of memory, and restarts always. Logs go to the journal, never to
files. Stopping waits up to 60 seconds: the backend closes the live
streams, lets open requests finish, then waits for background work.

## 6. nginx

```bash
sudo cp /opt/santral-c/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com
sudo ln -s /etc/nginx/sites-available/cm.toprakgureli.com /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

What the site file does:

- restores the visitor's real IP from Cloudflare, so bans, sign-in limits
  and the audit trail see the right address;
- limits the WhatsApp webhook and survey addresses to 10 requests a second
  per address (bursts of 100) and 1 MB bodies;
- lets `/api/v1/wa/` take 110 MB bodies with no buffering and long timeouts,
  for media and streamed exports, and does the same for chat attachments;
- forwards `/healthz` for the uptime monitor and never forwards `/metrics`;
- never caches `index.html`, caches hashed assets for a year.

Always run `sudo nginx -t` before a reload: a broken file keeps the old
configuration running, a reload without the test can take the site down.

## 7. Cloudflare

- **DNS**: an `A` record `cm` pointing at the server, proxied (orange cloud).
- **SSL/TLS mode: Full (strict).** Flexible would carry passwords and
  session cookies over plain HTTP between Cloudflare and the server.
  1. SSL/TLS > Origin Server > Create Certificate, for `cm.toprakgureli.com`.
  2. Save the certificate and key on the server:
     ```bash
     sudo mkdir -p /etc/ssl/cloudflare
     sudo nano /etc/ssl/cloudflare/cm.toprakgureli.com.pem
     sudo nano /etc/ssl/cloudflare/cm.toprakgureli.com.key
     sudo chmod 600 /etc/ssl/cloudflare/cm.toprakgureli.com.key
     ```
  3. In the nginx site, change the two `listen 80` lines of the main server
     block to `listen 443 ssl http2;` and `listen [::]:443 ssl http2;`, add
     the two `ssl_certificate` lines shown at the bottom of the file, and
     add the small port 80 redirect block shown there.
  4. `sudo nginx -t`, reload, then switch Cloudflare to Full (strict).
- SSL/TLS > Edge Certificates: turn on **Always Use HTTPS**.

Because `cm` is proxied, SSH does not go through that name. Use the
server's own address for SSH (section 11).

## 8. First sign-in

1. Open `https://cm.toprakgureli.com` and sign in with `owner.email` and
   `owner.password`.
2. Set a new password, then set up TOTP with an authenticator app.
3. Create users under **Kullanıcılar** and give roles under **Roller**. Each
   user sets their own password and TOTP at first sign-in.
4. Check the roles in **Roller**. Listening to recordings needs
   `call.record_access`, which only Yönetici (and the owner) has at first; give it to every
   role that should listen. `call.view_peers` (see the number and contact a
   colleague is talking to) and `call.transfer_external` (hand a call to a
   number outside the phone system, always audited) start on every default
   role; take them away where they are not wanted. `system.backup` (section
   10) is held only by the owner at first.

Users are never deleted, only deactivated (Kullanıcılar), which signs them
out at once and keeps their history.

A locked-out owner can be recovered from the server. The command asks for
the new password twice, so it never lands in the shell history:

```bash
cd /opt/santral-c/backend && sudo -u santral env HOME=/opt/santral-c /usr/local/go/bin/go run ./cmd/resetpw -config /opt/santral-c/config.yml -email owner@toprakgureli.com
```

It clears TOTP, unlocks the account, signs the user out everywhere, asks for
a new password at the next sign-in and writes the change to the audit log.
An unknown email is refused; `-create` makes it a new owner account.

## 9. Integrations, all from the panel

Nothing below goes into `config.yml`; the panel seals what you type with
`dataKey`.

- **Google Drive**: Yönetim > Sistem Ayarları > Teams Dosya Depolama, link
  the account once (needs `system.settings`). Chat and WhatsApp files are
  kept there.
- **WhatsApp number**: WhatsApp > Ayarlar > Cihazlar. Enter the phone number
  ID, WABA ID, App ID, a permanent system-user token and the app secret. The
  panel shows a callback URL and verify token; enter them in Meta App
  Dashboard > WhatsApp > Configuration and subscribe to `messages`.
- **A webhook address already registered in Meta**: if it cannot be changed,
  choose "Meta'da zaten kayıtlı bir webhook" on the number and install
  `deploy/nginx/whatsapp-existing-webhook.conf` (instructions inside it).
- **Return to the same agent**: WhatsApp > Ayarlar > Cihaz ayarları >
  Chatbot. A customer who writes again within the set minutes after their
  chat was resolved goes straight back to the agent who had it, without the
  chatbot. 0 leaves it to the chatbot.
- **AI reply assistant**: WhatsApp > Ayarlar > Yapay zekâ.
- **Tally surveys**: WhatsApp > Ayarlar > Cihaz ayarları > Memnuniyet anketi.
- **Database backups**: section 10.

Removing a WhatsApp number or chatbot keeps its history. The panel has no
"delete everything" action for them, on purpose.

## 10. Backups

The database is the only thing to back up; Redis holds nothing that is not
rebuilt on its own.

### Copies to a Shared Drive

Every six hours the backend dumps the whole database (`pg_dump`, custom
format) and uploads it to a folder in a Google Workspace Shared Drive
through a service account. The account must be only a **Contributor** in
that Shared Drive: it can add files and cannot delete them, so even a
server taken over cannot wipe its own backups. Before every upload the
backend asks Drive what the account may do and refuses when the folder is
not in a Shared Drive or when the account could delete. Files are named
`santral-YYYY-MM-DD-HHMM.dump` (Istanbul time) and the backend never
removes, renames or replaces one, so the folder only grows. Keep an eye on
the Shared Drive's storage.

Setup (once):

1. In the Google Cloud Console, pick or create a project and enable the
   **Google Drive API** (APIs & Services > Library).
2. IAM & Admin > Service Accounts > **Create service account**. It needs no
   project roles.
3. Open the account > Keys > Add key > Create new key > **JSON**. A file
   downloads; its `client_email` is the account's address.
4. In Google Drive, create a Shared Drive (or use one) and under Manage
   members add that address with the role **Contributor**. Not Content
   manager, not Manager: those can delete.
5. Create a folder inside the Shared Drive and copy its address from the
   browser.
6. In the panel, sign in with someone who has `system.backup` and open
   Yönetim > Sistem Ayarları > **Veritabanı Yedeği**. Paste the folder
   address into "Ortak Drive klasörü" and the whole JSON file into "Servis
   hesabı anahtarı", then press **Kaydet**. The key is sealed with
   `dataKey`.
7. Press **Bağlantıyı denetle**. It must say "Ortak Drive: evet · Ekleyebilir:
   evet · Silebilir: hayır".
8. Turn on **Otomatik yedekleme**, press **Kaydet**, then **Şimdi yedekle**
   once. The run shows in the list below with its size.
9. Delete the downloaded JSON file from your computer.

Each dump holds the whole database: password hashes, customer messages and
sealed credentials. Give access to the Shared Drive only to the people who
must restore. The alerts in section 11 tell you when a backup is late or
failed.

### Restoring a copy

Never put a dump in a world-readable place such as `/tmp`. Keep it in
`/var/backups/santral`, which only `postgres` can open.

1. Download the file from the Drive folder, then send it from your computer
   straight into that folder (replace `198.51.100.20` with your server's
   address and the file name with yours):
   ```bash
   ssh ubuntu@198.51.100.20 'sudo -u postgres sh -c "umask 077 && cat > /var/backups/santral/restore.dump"' < santral-2026-10-01-0900.dump
   ```
   Delete the downloaded file from your computer afterwards.
2. Always restore into a scratch database first and look at it:
   ```bash
   sudo -u postgres createdb santral_restore_check
   sudo -u postgres pg_restore --no-owner --dbname=santral_restore_check /var/backups/santral/restore.dump
   sudo -u postgres psql -d santral_restore_check -c 'SELECT count(*) FROM users;'
   sudo -u postgres psql -d santral_restore_check -c 'SELECT max(created_at) FROM wa_messages;'
   sudo -u postgres dropdb santral_restore_check
   ```
3. Only when it looks right, swap it in. The current database is kept under
   another name until you are sure:
   ```bash
   sudo systemctl stop santral
   sudo -u postgres psql -c 'ALTER DATABASE santral RENAME TO santral_before_restore;'
   sudo -u postgres createdb --owner santral santral
   sudo -u postgres pg_restore --no-owner --role=santral --dbname=santral /var/backups/santral/restore.dump
   sudo systemctl start santral
   curl -fsS http://127.0.0.1:8090/healthz
   ```
   The stored credentials open only with the same `dataKey`, `mfaKey` and
   `sipKey` the dump was made with.

Try steps 1 and 2 once after setting up backups, so you know it works.

### Local copies before each deploy

`deploy.sh` also dumps the database before every switch into
`/var/backups/santral`, named `pre-deploy-` plus the date, the time and the
commit that was running, and keeps the last ten. The folder is open only to `postgres` (mode 700), so list it as
`postgres`; a plain `ls` from your own shell sees nothing:

```bash
sudo -u postgres ls -lt /var/backups/santral
```

Restore one the same way as above, with its path in place of
`/var/backups/santral/restore.dump`. To find the newest:

```bash
sudo -u postgres sh -c 'ls -1t /var/backups/santral/pre-deploy-*.dump | head -1'
```

These copies sit on the same disk as the database, so they are not a
backup on their own; the Drive copies are.

## 11. Monitoring

- Point an uptime monitor at `https://cm.toprakgureli.com/healthz`.
- `/metrics` answers only on the server itself, in the Prometheus format:
  ```bash
  curl -fsS http://127.0.0.1:8090/metrics
  ```

### The monitoring stack

`deploy/observability` runs Prometheus (keeps the numbers for 30 days),
Jaeger (traces) and Grafana (a ready dashboard and alerts) in Docker. All
three listen on 127.0.0.1 only:

| Service | Address on the server |
|---|---|
| Grafana | 127.0.0.1:3000 |
| Prometheus | 127.0.0.1:9090 |
| Jaeger web screen | 127.0.0.1:16686 |
| Jaeger trace intake (OTLP over HTTP) | 127.0.0.1:4318 |

Start it:

```bash
sudo apt install -y docker.io docker-compose-v2
sudo docker compose -f /opt/santral-c/deploy/observability/docker-compose.yml up -d
sudo docker compose -f /opt/santral-c/deploy/observability/docker-compose.yml ps
```

Prometheus reads the backend's `/metrics` every 15 seconds. For traces, keep
`telemetry.otlpEndpoint: "http://127.0.0.1:4318"` in `config.yml` and
restart the backend (`sudo systemctl restart santral`).

### Looking at it from your laptop

Nothing is open to the internet, so use an SSH tunnel. Run one of these on
your laptop and leave it open; `198.51.100.20` stands for your server's own
public address, replace it:

```bash
ssh -N -L 3000:127.0.0.1:3000 ubuntu@198.51.100.20     # Grafana:    http://localhost:3000
ssh -N -L 9090:127.0.0.1:9090 ubuntu@198.51.100.20     # Prometheus: http://localhost:9090
ssh -N -L 16686:127.0.0.1:16686 ubuntu@198.51.100.20   # Jaeger:     http://localhost:16686
```

Grafana starts with its own default account (`admin` / `admin`) and asks
for a new password at the first sign-in; set a strong one right away. The
dashboard is under Dashboards > santral-c > **santral-c genel bakış**:
requests, error rate, response times, the slowest routes, live streams,
WhatsApp queues, after-call surveys, calls waiting for the phone system's
record, backups, database connections, memory and CPU, and a link to the
traces. Grafana starts in Turkish; the menu names below are the English
ones.

### Alerts

These rules ship in `grafana/provisioning/alerting/rules.yml`:

| Alert | Fires when |
|---|---|
| Sunucu kapalı | Prometheus cannot reach the backend for 2 minutes |
| Yedek gecikti | backups are on and the last good one is over 7 hours old, for 10 minutes |
| Son yedek başarısız | the most recent backup failed |
| İşlenemeyen WhatsApp bildirimi | some of Meta's notices could not be processed, for 5 minutes |
| WhatsApp gönderimi takıldı | the oldest waiting message has waited over 10 minutes, for 5 minutes |
| Mesaj sonrası işler birikiyor | over 200 customer messages wait for their follow-up work, for 10 minutes |
| Sunucu hataları arttı | over 5% of requests end in a server error, for 10 minutes |

Grafana sends alerts only to a contact point you add:

1. Alerting > Contact points > **Add contact point**. Pick the type (a chat
   webhook works as is; e-mail needs SMTP set up for Grafana first), fill
   it in and press **Test**.
2. Alerting > Notification policies: edit the default policy and choose
   that contact point.

When `deploy/observability` changes in an update, `deploy.sh` reminds you
to run the `docker compose ... up -d` line above again.

## 12. Updates

After new commits reach `main`, run the deploy script **as a sudo-capable
user (for example `ubuntu`), not as `santral`**:

```bash
BRANCH=main /opt/santral-c/deploy/deploy.sh
```

What it does, in order:

1. Refuses to run when the checkout on the server has local changes to
   tracked files (it lists them); `FORCE=1` overrides that and the changes
   are lost.
2. Pulls `origin/$BRANCH` and builds the backend as `santral.new` next to
   the running binary, stamped with the commit.
3. Checks `config.yml` with the new binary (`-check-config`). On a `HATA`
   line it stops; nothing was switched and the running version keeps
   running.
4. Builds the panel.
5. Dumps the database into `/var/backups/santral` (section 10) and keeps
   the last ten of these copies.
6. Installs `deploy/santral.service` when it changed, keeping the old one.
7. Keeps the running binary as `santral.prev`, puts the new one in place
   and restarts the service. Migrations run at that start.
8. Waits up to a minute for `/healthz`. If it does not pass, it prints the
   last 60 log lines, puts `santral.prev` (and the old unit) back and
   restarts. If even that does not come up, it prints the path of the dump
   from step 5. The panel files are not touched.
9. Only after the backend is healthy, publishes the panel to
   `/var/www/santral-c`.
10. Warns when `deploy/nginx` or `deploy/observability` changed, with the
    commands to install them; tests nginx with `nginx -t` and reloads it
    only if the test passes.

These environment variables override its defaults:

| Variable | Default |
|---|---|
| `APP_DIR` | `/opt/santral-c` |
| `WEB_ROOT` | `/var/www/santral-c` |
| `BRANCH` | `main` |
| `GO` | `/usr/local/go/bin/go` |
| `PORT` | `8090` |
| `DB_NAME` | `santral` |
| `BACKUP_DIR` | `/var/backups/santral` |
| `HEALTH_WAIT` | `60` (seconds) |
| `UNIT` | `/etc/systemd/system/santral.service` |
| `FORCE` | unset; `1` deploys over local changes |

When `deploy/nginx/` changed, install the site file again and test it:

```bash
sudo cp /opt/santral-c/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com
sudo nginx -t
sudo systemctl reload nginx
```

If you run the site over 443 (section 7), carry your `listen` and
`ssl_certificate` lines over to the new copy before testing it.

`deploy/test/deploy_test.sh` runs the script against a stand-in server
(a good release, one that does not come up, a config that fails the check,
local changes on the server).

## 13. Replacing keys

### The data key

1. Move the current value of `security.dataKey` into
   `security.previousDataKeys` (comma separated if there are several).
2. Put a new `openssl rand -hex 32` value in `security.dataKey`.
3. `sudo systemctl restart santral`. At start every stored credential
   (WhatsApp, outside systems, Drive, AI key, survey secrets, the backup
   key) is opened with the old key and sealed with the new one. Survey links
   and the Drive link state signed with the old key still check while it is
   in `previousDataKeys`.
4. Once the log shows the service started, the old key can be removed from
   `previousDataKeys`. Survey links sent before the change stop working then.

Without the old key in `previousDataKeys` every stored credential has to be
typed in again.

### The session secret

`auth.secret` is not the data key. It signs the 15-minute access tokens and
the short tokens between the steps of signing in. It is also still accepted
for survey links sent before they were signed with the data key, and at
start it opens WhatsApp and Drive secrets saved before the data key existed
so they can be sealed again. Changing it and restarting:

- refuses every access token at once; open panels renew from their sign-in
  session and carry on, so nobody has to sign in again;
- ends every half-finished sign-in (a TOTP step, a first-login password
  change), which has to start again;
- breaks survey links sent before survey links were signed with the data
  key.

## Notes

- The softphone needs a secure page for the microphone; HTTPS covers it. Its
  WSS connection goes straight to `api.bulutsantralim.com`, so nginx needs no
  WebSocket proxy.
- The PBX API is rate limited; the backend polls within the limits and keeps
  its own copy of call records, so no tuning is needed.
- To change the TOTP requirement later, use Yönetim > Sistem Ayarları >
  İki Adımlı Doğrulama in the panel. The config value only seeds the
  default on a fresh database.
