# Deploying santral-c to cm.toprakgureli.com

One Ubuntu host runs everything: nginx serves the built panel and forwards
`/api` to the Go backend on the loopback address, PostgreSQL and Redis run
on the same machine, and Cloudflare provides DNS and HTTPS in front.

```
browser ──HTTPS──▶ Cloudflare ──HTTPS (Full strict)──▶ nginx ──┬─ /       panel (/var/www/santral-c)
                                                               └─ /api/  ▶ 127.0.0.1:8090 (santral)
Meta, Tally ──HTTPS──▶ Cloudflare ──▶ nginx ─ /api/v1/wa/hook, /api/v1/wa/survey (rate limited, 1 MB)
softphone ──SIP over WSS──▶ api.bulutsantralim.com   (straight from the browser, not through nginx)
```

## 0. Packages

```bash
sudo apt update
sudo apt install -y nginx postgresql redis-server git rsync curl
```

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

## 1. Firewall

Only SSH and the web ports are open. The backend listens on 127.0.0.1 only
(`app.host`), so it is never reachable from outside even without a firewall,
but the firewall is the second lock.

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
| `auth.secret` | `openssl rand -hex 32` | signs session tokens |
| `security.dataKey` | `openssl rand -hex 32` | seals every credential typed into the panel |
| `security.mfaKey` | `openssl rand -hex 16` | seals TOTP secrets |
| `bulutsantralim.sipKey` | `openssl rand -hex 16` | seals SIP passwords |

Keep a copy of `dataKey`, `mfaKey` and `sipKey` somewhere safe outside the
server (a password manager). Without them a database backup cannot open the
stored credentials.

Check these values in the prod template as well:

- `app.host: "127.0.0.1"` so only nginx reaches the backend.
- `auth.accessTTL: 15m`. A signed-out or deactivated user is cut off at once
  anyway; this only bounds how long a stolen token could live.
- `auth.cookieSecure: true`, the site is HTTPS only.
- `security.requireMFA: true` asks everyone to set up TOTP at first sign-in.
- `bulutsantralim.*`: the API key comes from OIM > Bulut Santralım > Santral
  Ayarlarım; `sipDomain`, `sipWssUrl` and `stunUrl` from the PBX's WebRTC
  settings.
- `drive.*`: a Google Cloud OAuth client (Web application). Its redirect
  URI must be exactly `https://cm.toprakgureli.com/api/v1/teams/drive/callback`.

The backend refuses to start while `dataKey` is still the example value.

## 3. PostgreSQL and Redis

```bash
sudo -u postgres createuser --pwprompt santral
sudo -u postgres createdb --owner santral santral
```

Type the same password you put in `database.password`. Redis works as
installed, on localhost:6379. The backend applies the migrations itself on
every start.

## 4. First build

```bash
sudo -u santral env HOME=/opt/santral-c bash -c 'cd /opt/santral-c/backend && /usr/local/go/bin/go build -o /opt/santral-c/santral ./cmd/santral'
sudo -u santral env HOME=/opt/santral-c bash -c 'cd /opt/santral-c/frontend && npm ci && npm run build'
sudo mkdir -p /var/www/santral-c
sudo rsync -a --delete /opt/santral-c/frontend/dist/ /var/www/santral-c/
```

## 5. systemd

```bash
sudo cp /opt/santral-c/deploy/santral.service /etc/systemd/system/santral.service
sudo systemctl daemon-reload
sudo systemctl enable --now santral
systemctl status santral
curl -fsS http://127.0.0.1:8090/healthz
```

`/healthz` answers 200 when PostgreSQL and Redis both answer, and names the
running build. Logs are JSON lines with the request id and user:

```bash
journalctl -u santral -f
```

## 6. nginx

```bash
sudo cp /opt/santral-c/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com
sudo ln -s /etc/nginx/sites-available/cm.toprakgureli.com /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

What the site file does:

- restores the visitor's real IP from Cloudflare, so bans and the audit trail
  see the right address;
- limits the WhatsApp webhook and survey addresses to 10 requests a second
  per address (bursts of 100) and 1 MB bodies;
- lets `/api/v1/wa/` take 110 MB bodies with no buffering and long timeouts,
  for media and streamed exports, and does the same for chat attachments;
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

## 8. First sign-in

1. Open `https://cm.toprakgureli.com` and sign in with `owner.email` and
   `owner.password`.
2. Set a new password, then set up TOTP with an authenticator app.
3. Create users under **Kullanıcılar** and give roles under **Roller**. Each
   user sets their own password and TOTP at first sign-in.

A locked-out owner can be recovered from the server:

```bash
cd /opt/santral-c/backend && sudo -u santral env HOME=/opt/santral-c /usr/local/go/bin/go run ./cmd/resetpw -config /opt/santral-c/config.yml -email owner@toprakgureli.com -password 'Yeni-Sifre-123'
```

## 9. Integrations, all from the panel

Nothing below goes into `config.yml`; the panel seals what you type with
`dataKey`.

- **Google Drive**: Yönetim > Sistem Ayarları > Teams Dosya Depolama, link
  the account once. Chat and WhatsApp files are kept there.
- **WhatsApp number**: WhatsApp > Ayarlar > Cihazlar. Enter the phone number
  ID, WABA ID, App ID, a permanent system-user token and the app secret. The
  panel shows a callback URL and verify token; enter them in Meta App
  Dashboard > WhatsApp > Configuration and subscribe to `messages`.
- **A webhook address already registered in Meta**: if it cannot be changed,
  choose "Meta'da zaten kayıtlı bir webhook" on the number and install
  `deploy/nginx/whatsapp-existing-webhook.conf` (instructions inside it).
- **AI reply assistant**: WhatsApp > Ayarlar > Yapay zekâ.
- **Tally surveys**: WhatsApp > Ayarlar > Cihaz ayarları > Memnuniyet anketi.

## 10. Backups

The database is the only thing to back up; Redis holds nothing that is not
rebuilt on its own. A nightly dump, kept for 14 days:

```bash
sudo mkdir -p /var/backups/santral
sudo chown postgres:postgres /var/backups/santral
sudo chmod 700 /var/backups/santral
echo '30 3 * * * postgres pg_dump -Fc santral > /var/backups/santral/santral-$(date +\%F).dump && find /var/backups/santral -name "*.dump" -mtime +14 -delete' | sudo tee /etc/cron.d/santral-backup
```

Copy the dumps off the server as well (another machine or storage bucket):
a backup on the same disk does not survive losing the disk.

Try a restore once, into a separate database, so you know it works:

```bash
sudo -u postgres createdb santral_restore_check
sudo -u postgres pg_restore -d santral_restore_check "$(ls -t /var/backups/santral/*.dump | head -1)"
sudo -u postgres dropdb santral_restore_check
```

## 11. Monitoring

- Point an uptime monitor at `https://cm.toprakgureli.com/healthz`.
- `/metrics` answers only on the server itself and shows queue sizes and open
  live streams:
  ```bash
  curl -fsS http://127.0.0.1:8090/metrics
  ```

## 12. Updates

After new commits reach `main`, run the deploy script **as a sudo-capable
user (for example `ubuntu`), not as `santral`**:

```bash
BRANCH=main /opt/santral-c/deploy/deploy.sh
```

It checks the config has a data key, pulls, builds the backend and the
panel, publishes the panel, restarts the service, tests and reloads nginx and
calls `/healthz`. Migrations run when the service starts.

Two things the script does not do on its own:

- **Take a backup before an update that adds migrations**:
  ```bash
  sudo -u postgres pg_dump -Fc santral > /tmp/santral-before-update.dump
  ```
- **Install a changed nginx site**. When `deploy/nginx/` changed in the
  update, copy the file again and test it:
  ```bash
  sudo cp /opt/santral-c/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com
  sudo nginx -t
  sudo systemctl reload nginx
  ```
  If you run the site over 443 (section 7), carry your `listen` and
  `ssl_certificate` lines over to the new copy before testing it.

## 13. Replacing the data key

1. Move the current value of `security.dataKey` into
   `security.previousDataKeys`.
2. Put a new `openssl rand -hex 32` value in `security.dataKey`.
3. `sudo systemctl restart santral`. At start every stored credential is
   opened with the old key and sealed with the new one.
4. Once the log shows the service started, the old key can be removed from
   `previousDataKeys`.

## Notes

- The softphone needs a secure page for the microphone; HTTPS covers it. Its
  WSS connection goes straight to `api.bulutsantralim.com`, so nginx needs no
  WebSocket proxy.
- The PBX API is rate limited; the backend polls within the limits and keeps
  its own copy of call records, so no tuning is needed.
- To change the TOTP requirement later, use Yönetim > Sistem Ayarları >
  İki Adımlı Doğrulama in the panel. The config value only seeds the
  default on a fresh database.
