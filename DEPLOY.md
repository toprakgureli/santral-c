# Deploying santral-c to cm.toprakgureli.com

One Ubuntu host runs everything: nginx serves the built panel and forwards
`/api` to the Go backend on the loopback address, PostgreSQL and Redis run
on the same machine, and Cloudflare provides DNS and HTTPS in front. The
monitoring stack (Prometheus, Grafana, Jaeger) runs on the same host in
Docker and listens on 127.0.0.1 only.

```
browser ──HTTPS──▶ Cloudflare ──HTTPS (Full strict)──▶ nginx :443 ──┬─ /       panel (/var/www/santral-c)
                                 (Origin Certificate) (also :80)    └─ /api/  ▶ 127.0.0.1:8090 (santral)
Meta, Tally ──HTTPS──▶ Cloudflare ──▶ nginx :443 ─ /api/v1/wa/hook, /api/v1/wa/survey (rate limited, 1 MB)
Meta (address already registered) ──▶ nginx :5001 ─ registered webhook paths only (optional, section 9)
softphone ──SIP over WSS──▶ api.bulutsantralim.com   (straight from the browser, not through nginx)
santral ──every 6 hours──▶ Google Shared Drive (database copies)
you ──SSH tunnel──▶ Grafana 127.0.0.1:3000, Prometheus 127.0.0.1:9090, Jaeger 127.0.0.1:16686
                    (node_exporter 127.0.0.1:9100 reports the disks to Prometheus)
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
- `database.statementTimeout: 30s`: PostgreSQL stops any single query of the
  backend that runs longer (the same 30 seconds nginx waits for an `/api/`
  answer). Migrations and the backups' `pg_dump` run without it; the
  WhatsApp clean-up of old rows may take up to 10 minutes.
- `database.dataPath: /var/lib/postgresql`: a folder on the disk that holds
  the database files. The system warnings (section 11) report when that disk
  fills up. Change it if the database lives elsewhere.

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
server refuses to start on any `HATA` line. Only `app.development: test`
(or `development`) is a test setup; a missing, mistyped or unknown value
counts as live, with an `UYARI` line naming it. Once
the binary is built (section 4) you can run the same check by hand:

```bash
sudo -u santral /opt/santral-c/santral -check-config -config /opt/santral-c/config.yml
```

It prints `HATA` (stops the server) and `UYARI` (reported, the server still
starts) lines, or `config.yml uygun`. It stops on an empty, example or
too short `auth.secret` or `security.dataKey`, a `previousDataKeys` entry
the server could not use (shorter than 32 characters), an empty or example
`mfaKey`, a missing `sipKey` or API key while `bulutsantralim.enabled` is true,
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
every start, on a connection of its own named `santral-migrate` with no
statement timeout, and logs "migrating the database" and "database is up to
date". Before that it rebuilds any index a stopped concurrent build left
unusable (PostgreSQL marks it invalid and `CREATE INDEX ... IF NOT EXISTS`
would skip it for ever); one that cannot be rebuilt is dropped, so its
migration makes it again.

## 4. First build

The build is stamped with the git commit the same way `deploy.sh` does it,
so the sidebar shows which version runs:

```bash
SHA=$(sudo -u santral env HOME=/opt/santral-c git -C /opt/santral-c rev-parse --short HEAD)
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ID=$(od -An -N6 -tx1 /dev/urandom | tr -d ' 
')
sudo -u santral env HOME=/opt/santral-c bash -c "cd /opt/santral-c/backend && /usr/local/go/bin/go build -trimpath -ldflags '-X main.version=$SHA -X main.buildTime=$NOW -X main.buildID=$ID' -o /opt/santral-c/santral ./cmd/santral"
sudo -u santral env HOME=/opt/santral-c bash -c "cd /opt/santral-c/frontend && npm ci --ignore-scripts && VITE_BUILD_ID=$ID npm run build"
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
most 2 GB of memory (`GOMEMLIMIT=1500MiB` makes Go collect garbage harder
well before that), and restarts always. Logs go to the journal, never to
files. Stopping waits up to 60 seconds: the backend closes the live
streams, lets open requests finish, then waits for background work.

The backend bounds every connection, so a client that stops half way
cannot hold one for ever: a request must arrive within 30 seconds (10
minutes when it announces a body over 1 MB, an upload), an answer must go
out within 2 minutes (an hour for GET requests: downloads, exports,
recordings; 24 hours for the live streams, the paths ending in `/stream`),
and an idle kept-alive connection closes after 75 seconds. The time a
handler works is not counted. Paths are case sensitive: `/API/v1/...` is not
`/api/v1/...` and answers 404.

## 6. nginx

The site file serves HTTPS with the Cloudflare Origin Certificate, so put
the certificate in place first (section 7, steps 1 and 2); without it
`nginx -t` fails and nothing changes.

```bash
sudo cp /opt/santral-c/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com
sudo ln -s /etc/nginx/sites-available/cm.toprakgureli.com /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

What the site file does:

- restores the visitor's real IP from Cloudflare, so bans, sign-in limits
  and the audit trail see the right address;
- answers on 443 with the Origin Certificate and on 80, so Cloudflare can be
  switched to Full (strict) without a gap;
- limits the WhatsApp webhook and survey addresses to 50 requests a second
  per address (bursts of 300) and 1 MB bodies;
- lets only the three WhatsApp upload addresses (`/api/v1/wa/files`,
  `/api/v1/wa/templates/media`, `/api/v1/wa/conversations/:id/media`)
  take bodies up to 110 MB, and only after asking the backend whether the
  request carries a valid session, before the body is read; at most four
  such uploads run at once per address. Every other address takes 8 MB at
  most. The session check uses nginx's auth_request module, which Ubuntu's
  nginx includes (`nginx -V 2>&1 | grep -o http_auth_request_module`
  prints its name);
- gives the rest of `/api/v1/wa/` unbuffered answers and 300-second
  timeouts, for media and streamed exports;
- streams chat attachments (`/api/v1/teams/attachments/`) from Drive to the
  browser without buffering and with 900-second timeouts; their uploads go
  from the browser straight to Drive, so they keep the site's 8 MB body
  limit;
- forwards `/healthz` for the uptime monitor and never forwards `/metrics`;
- never caches `index.html`, caches hashed assets for a year;
- sends the security headers with every answer, the panel page included:
  no file-type guessing, no showing the panel inside another site, HTTPS
  only for browsers.

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
  3. Install the site file (section 6). It already listens on 443 with these
     two files and still answers on 80.
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
   10) is held only by the owner at first; `system.health` (the system
   warnings, section 11) by the owner and Yönetici.

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
  That port reaches only the registered webhook paths: nginx turns away
  `/api`, `/healthz` and `/metrics` in any spelling, and marks the requests
  (`X-Santral-Entry: existing-hook`) so the backend refuses them too.
  Capitals in the registered path do not matter.
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

Every six hours (counted from the start of the last good copy) the backend
dumps the whole database (`pg_dump`, custom format) and uploads it to a
folder in a Google Workspace Shared Drive through a service account. A
failed copy is tried again after 30 minutes, then after an hour, two hours
and so on, at most six hours apart. The dump may take an hour and the
upload another hour; past that they are given up and the run is recorded as
failed, so a stuck connection never stops the backups. A run cut off by a
restart is closed as failed at the next start. The account must be only a **Contributor** in
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
`/var/backups/santral`, which only `postgres` can open. `deploy.sh` makes
that folder; before the first deploy, make it yourself:

```bash
sudo install -d -m 700 -o postgres -g postgres /var/backups/santral
```

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
commit that was running, and keeps the last ten. It removes the old ones
before the dump, and refuses to deploy when the disk would not hold the new
copy with 1 GB to spare (`DUMP_MARGIN_MB`); a copy that fails half way is
deleted. The folder is open only to `postgres` (mode 700), so list it as
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
  `santral_dependency_up{dep="postgres"}` and `{dep="redis"}` come from the
  same checks `/healthz` runs and read 0 while that part is down; the numbers
  read from the database are missing then.

### System warnings in the panel

People holding `system.health` ("Sistem uyarılarını görür"; at first the
owner and Yönetici) see the server's warnings as cards under the top bar,
each saying what is wrong and what to do. The backend checks once a minute:

| Warning | When |
|---|---|
| Sunucunun diski doluyor | the disk holding `database.dataPath` is 85% full (critical from 95%) |
| Redis cevap vermiyor | Redis does not answer |
| Veritabanı yedeği gecikti | backups are on and the last good one is over 7 hours old (critical after a day), or none worked an hour after they were switched on |
| WhatsApp bildirimleri işlenemedi | a Meta notice was given up on in the last hour |
| WhatsApp mesajları gönderilemiyor | the oldest message waiting to go out has waited 10 minutes (critical after 30) |
| Gelen mesajların işleri birikti | over 200 customer messages wait for their follow-up work |

The panel asks `GET /api/v1/system/health` every minute. A card closed with
its cross stays closed until the warning changes (the disk fills another 5%,
a new notice fails). When PostgreSQL itself is down the panel cannot ask at
all, since every signed-in request needs the database; the Grafana alert
"Veritabanı ya da Redis kapalı" covers that.

### The monitoring stack

`deploy/observability` runs Prometheus (keeps the numbers for 30 days),
Jaeger (traces), Grafana (a ready dashboard and alerts) and node_exporter
(the host's disks, memory and processor; it sees the host's file system
read-only) in Docker. All of them listen on 127.0.0.1 only:

| Service | Address on the server |
|---|---|
| Grafana | 127.0.0.1:3000 |
| Prometheus | 127.0.0.1:9090 |
| Jaeger web screen | 127.0.0.1:16686 |
| Jaeger trace intake (OTLP over HTTP) | 127.0.0.1:4318 |
| node_exporter | 127.0.0.1:9100 |

Start it:

```bash
sudo apt install -y docker.io docker-compose-v2
sudo docker compose -f /opt/santral-c/deploy/observability/docker-compose.yml up -d
sudo docker compose -f /opt/santral-c/deploy/observability/docker-compose.yml ps
```

Prometheus reads the backend's `/metrics` and node_exporter every 15
seconds. For traces, keep
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
record, backups, database connections, memory and CPU, whether PostgreSQL
and Redis answer, how full the disks are, and a link to the traces. Grafana starts in Turkish; the menu names below are the English
ones.

### Alerts

These rules ship in `grafana/provisioning/alerting/rules.yml`:

| Alert | Fires when |
|---|---|
| Sunucu kapalı | Prometheus cannot reach the backend for 2 minutes |
| Veritabanı ya da Redis kapalı | the backend cannot reach PostgreSQL or Redis for 2 minutes (`santral_dependency_up` is 0) |
| Kök disk doluyor | the root file system is over 85% full, for 5 minutes |
| Veritabanı diski doluyor | the disk holding `database.dataPath` is over 85% full, for 5 minutes |
| Disk ölçülemiyor | Prometheus cannot reach node_exporter for 5 minutes |
| Yedek gecikti | backups are on and the last good one is over 7 hours old, for 10 minutes; also when the numbers are missing that long (no backend, no backups) |
| Son yedek başarısız | the most recent backup failed |
| İşlenemeyen WhatsApp bildirimi | a Meta notice was given up on in the last hour, for 5 minutes; it clears by itself an hour after the last failure |
| WhatsApp gönderimi takıldı | the oldest waiting message has waited over 10 minutes, for 5 minutes |
| Mesaj sonrası işler birikiyor | over 200 customer messages wait for their follow-up work, for 10 minutes |
| Sunucu hataları arttı | over 5% of requests end in a server error, for 10 minutes |

While PostgreSQL is down every number the backend reads from the database
is missing. "Veritabanı ya da Redis kapalı" fires then; the other rules keep
the state they had (`noDataState: KeepLast`) instead of turning OK. "Sunucu
kapalı", "Yedek gecikti" and "Disk ölçülemiyor" alert on missing numbers,
since there the missing number is the problem itself.

Grafana sends alerts only to a contact point you add:

1. Alerting > Contact points > **Add contact point**. Pick the type (a chat
   webhook works as is; e-mail needs SMTP set up for Grafana first), fill
   it in and press **Test**.
2. Alerting > Notification policies: edit the default policy and choose
   that contact point.

When `deploy/observability` changes in an update, `deploy.sh` reminds you
to recreate the stack. Prometheus and Grafana read their files (scrape
targets, alert rules) only when their containers are created, so a plain
`up -d` would leave the new alerts unloaded:

```bash
sudo docker compose -f /opt/santral-c/deploy/observability/docker-compose.yml up -d --force-recreate
```

The collected numbers and Grafana's settings live in volumes and stay.

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
2. Reads the running commit from `/var/lib/santral-deploy/deployed-sha`,
   written after every good deploy (the checkout's HEAD can be a commit
   that was rolled back). Pulls `origin/$BRANCH`, takes
   `deploy/santral.service` from that commit with `git show` before
   anything is built, and builds the backend as `santral.new` next to the
   running binary, stamped with the commit.
3. Checks `config.yml` with the new binary (`-check-config`). On a `HATA`
   line it stops; nothing was switched and the running version keeps
   running.
4. Builds the panel with `npm ci --ignore-scripts`: no npm package runs an
   install script on the server (the build needs none).
5. Removes old local copies, checks the disk has room for a new one (the
   size of the last copy, or of the database, plus half, plus 1 GB) and
   dumps the database into `/var/backups/santral` (section 10). Without the
   room, or when the dump fails (the half-written file is deleted), it
   stops and nothing was switched.
6. Installs the unit from step 2 when it differs from the installed one,
   keeping the old one. The working tree's copy is never installed, so a
   build step cannot change what root runs.
7. Keeps the running binary as `santral.prev`, puts the new one in place
   and restarts the service. Migrations run at that start.
8. Waits up to a minute for `/healthz`, longer (up to `MIGRATE_WAIT`) while
   the new version is still migrating the database (its `santral-migrate`
   connection shows in `pg_stat_activity`). If it does not pass, it prints
   the last 60 log lines, puts `santral.prev` (and the old unit) back,
   resets the checkout to the running commit and restarts. If even that
   does not come up, it prints the path of the dump from step 5. The panel
   files are not touched.
9. Only after the backend is healthy, publishes the panel to
   `/var/www/santral-c` and writes the commit down as the running one.
10. Warns when `deploy/nginx` or `deploy/observability` changed since the
    running commit, with the commands to install them; tests nginx with
    `nginx -t` and reloads it only if the test passes.

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
| `MIGRATE_WAIT` | `1800` (seconds; how long a start that is still migrating is waited for) |
| `DUMP_MARGIN_MB` | `1024` (room left free after the local copy) |
| `STATE_DIR` | `/var/lib/santral-deploy` (where the running commit is written) |
| `UNIT` | `/etc/systemd/system/santral.service` |
| `FORCE` | unset; `1` deploys over local changes |

When `deploy/nginx/` changed, install the site file again and test it:

```bash
sudo cp /opt/santral-c/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com
sudo nginx -t
sudo systemctl reload nginx
```

The file already carries the HTTPS lines, so a copy installs over the old
one as it is; `nginx -t` refuses it while the certificate is missing.

`deploy/test/deploy_test.sh` runs the script against a stand-in server
(a good release, one that does not come up, a config that fails the check,
local changes on the server, a start that is still migrating, a disk too
full for the copy, a copy that fails half way, an npm package that edits
the unit file in the working tree).

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
