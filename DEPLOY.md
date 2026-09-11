# Deploying santral-c to cm.toprakgureli.com

Frontend (Vite build) is served as static files by nginx; the Go backend runs
on `127.0.0.1:8090` behind nginx; PostgreSQL and Redis run on the same host;
Cloudflare provides DNS and HTTPS.

```
browser ──HTTPS──▶ Cloudflare ──HTTP/HTTPS──▶ nginx ──┬─ /            static SPA (/var/www/santral-c)
                                                       └─ /api/  proxy ▶ 127.0.0.1:8090 (santral backend)
softphone (SIP/WebRTC) ──WSS──▶ api.bulutsantralim.com   (direct from the browser, not through nginx)
```

## 0. Prerequisites on the server

Ubuntu/Debian example:

```bash
sudo apt update
sudo apt install -y nginx postgresql redis-server git rsync curl
# Go 1.26+
curl -fsSL https://go.dev/dl/go1.26.0.linux-amd64.tar.gz | sudo tar -C /usr/local -xz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
# Node 20+
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs
```

Create a service user:

```bash
sudo useradd --system --home /opt/santral-c --shell /usr/sbin/nologin santral
```

## 1. Push the repo to GitHub (one time)

The project is not on GitHub yet. From your machine, in the project root:

```bash
git remote add origin git@github.com:<you>/santral-c.git   # if not already set
git push -u origin main
```

`config.yml` is gitignored, so **no secrets are pushed** — verify with
`git status` that `config.yml` is not staged. Only `config.example.yml` and
`config.prod.example.yml` (placeholders) are in the repo.

## 2. Clone and configure on the server

```bash
sudo git clone https://github.com/<you>/santral-c.git /opt/santral-c
sudo chown -R santral:santral /opt/santral-c
cd /opt/santral-c
sudo -u santral cp config.prod.example.yml config.yml
sudo -u santral nano config.yml      # fill every change-me value
```

Generate the secrets:

```bash
openssl rand -hex 32   # auth.secret
openssl rand -hex 16   # security.mfaKey
openssl rand -hex 16   # bulutsantralim.sipKey
```

Copy the `bulutsantralim:` values (apiKey, sipDomain, sipWssUrl, stunUrl) from
your working local `config.yml`.

`security.requireMFA: true` in the prod template forces TOTP MFA for everyone at
first login. `owner.email` / `owner.password` are your easy first-login
credentials (you change the password immediately after logging in).

## 3. PostgreSQL and Redis

```bash
sudo -u postgres psql <<'SQL'
CREATE USER santral WITH PASSWORD 'the-db-password-from-config';
CREATE DATABASE santral OWNER santral;
SQL
```

Redis works out of the box on localhost:6379. The backend runs migrations
automatically on start.

## 4. First build

```bash
cd /opt/santral-c/backend && sudo -u santral /usr/local/go/bin/go build -o /opt/santral-c/santral ./cmd/santral
cd /opt/santral-c/frontend && sudo -u santral npm ci && sudo -u santral npm run build
sudo mkdir -p /var/www/santral-c
sudo rsync -a --delete /opt/santral-c/frontend/dist/ /var/www/santral-c/
```

## 5. systemd service

```bash
sudo cp /opt/santral-c/deploy/santral.service /etc/systemd/system/santral.service
sudo systemctl daemon-reload
sudo systemctl enable --now santral
systemctl status santral            # should be active; migrations ran, owner seeded
curl -fsS http://127.0.0.1:8090/healthz
```

## 6. nginx

```bash
sudo cp /opt/santral-c/deploy/nginx/cm.toprakgureli.com.conf /etc/nginx/sites-available/cm.toprakgureli.com
sudo ln -s /etc/nginx/sites-available/cm.toprakgureli.com /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

## 7. Cloudflare

- **DNS:** add an `A` record `cm` → your server's public IP, proxy **on** (orange
  cloud).
- **SSL/TLS mode:** `Flexible` works immediately with the provided HTTP nginx
  block. For end-to-end encryption use `Full (strict)`: create an Origin
  Certificate (SSL/TLS ▸ Origin Server), install it, and switch nginx to the
  `:443` block noted at the bottom of the site config.
- Open ports 80/443 to Cloudflare on the host firewall; the backend port 8090
  must **not** be exposed publicly (it only listens for nginx on loopback).

No extra Cloudflare app/rule is required beyond DNS + SSL.

## 8. First login

1. Open `https://cm.toprakgureli.com`, log in with `owner.email` /
   `owner.password`.
2. You are forced to set a new password.
3. Because `requireMFA` is on, you then set up TOTP: scan the QR with an
   authenticator app and enter the code. Done.

Create the rest of the users from **Kullanıcılar**, assign roles/permissions
from **Roller**. Each user sets their own password and MFA at their first login.

## 9. Updates

After pushing new commits to `main`, run the deploy script **as a sudo-capable
user (e.g. `ubuntu`), not as `santral`**. The script itself switches to the
`santral` user for the build steps and uses `sudo` for the system steps, so
`santral` (which is not in sudoers) cannot run it:

```bash
BRANCH=main /opt/santral-c/deploy/deploy.sh
```

It pulls (`git reset --hard origin/main`), rebuilds backend + frontend,
republishes the static files, restarts the service and reloads nginx, then hits
`/healthz`. Do NOT prefix it with `sudo -u santral` — that makes every internal
`sudo` fail with "santral is not in the sudoers file" and nothing gets rebuilt
or restarted.

## Notes

- The softphone needs a secure context for the microphone; HTTPS via Cloudflare
  satisfies this. The SIP/WebRTC WSS connects straight to
  `api.bulutsantralim.com` from the browser, so nginx needs no WebSocket proxy.
- Hosted-PBX (Verimor) CDR calls are rate limited; the backend already caches
  and polls within the limit, so no tuning is needed.
- To change the MFA requirement later, update the `mfa_required` row in
  `system_settings` (the config value only seeds the default on a fresh DB).
