# santral-c

English | [Türkçe](README.tr.md)

santral-c is the call manager we use on top of Bulutsantralim, Verimor's hosted
PBX. Agents take and make calls from a browser softphone inside the panel, see
who is on the line and who is on a break, look up the caller in the contact
book, log escalations, and listen to recordings. Managers get the same panel
plus user, role and permission administration, login security logs and an
audit trail.

The hosted PBX keeps doing what it does well (SIP trunks, queues, recording).
santral-c adds the agent-facing layer around it: presence, call history with
filters, contacts, escalations, and a proper identity model with TOTP.

## What is in the box

**Softphone and presence**

- SIP.js softphone registering to Bulutsantralim over WSS, with mute, hold,
  transfer and DTMF. One registration per browser, shared across tabs.
- Floating call bar that follows you across pages, and an optional Chrome
  extension that puts the same call controls on every website
  (`extension/`, see its README).
- Agent presence: available, break, back office, do not disturb. Non-available
  states engage DND on the PBX so the agent stops receiving calls. Daily totals
  per state are shown on the dashboard.
- Live agent list over Server-Sent Events with a polling fallback.

**Calls**

- Call history from Verimor's CDR with direction, disposition, duration and
  recording playback. Filter by date (today by default, presets for yesterday,
  last 7 or 30 days, this month, custom range), direction, phone number, own
  calls or a specific extension.
- Recent calls are served from a warm in-memory window the poller keeps up to
  date, so the common views are instant and the rate-limited API is not hit on
  every page load. Older dates go to Verimor's own server-side query.
- CSV export of the current filter (`cdr.export`).
- Click to call from anywhere a number is shown.

**Contacts and escalations**

- Contact book with E.164 lookup, multiple numbers per contact, caller
  identification during a call.
- Escalation catalog (category and reason) managed by admins; agents log an
  escalation for the caller on the line, and a search page shows a number's
  escalation history.

**Identity and administration**

- Login with e-mail and password, TOTP second factor, forced first-login
  password change and optional forced MFA enrollment (a system setting).
- Users: profile, roles, SIP account (pulled from Verimor or entered by hand),
  password reset with a generated temporary password and a ready-to-send
  welcome message, activate and deactivate.
- Roles with a permission catalog grouped by module. Copy a role to start a new
  one. An admin cannot grant a permission they do not hold themselves.
- Login attempts, IP bans with unban, and an audit trail of every privileged
  action (who, what, when, from which IP).

## Stack

- **Backend**: Go 1.26, Fiber, GORM, PostgreSQL, Redis, goose migrations.
  argon2id password hashing, JWT access tokens with hashed refresh sessions,
  TOTP secrets encrypted at rest. Written to the Uber Go style guide.
- **Frontend**: React 18, TypeScript, Vite, Tailwind v4, SIP.js.
- **Telephony**: Bulutsantralim REST API (CDR, user statuses, queues,
  originate) and the WebRTC gateway for the softphone.

`DESIGN.md` is the original design note from when the plan was to run our own
Asterisk. The identity and data model sections still apply; the media engine
was replaced by the hosted PBX before the first release.

## Running locally

You need Docker, Go 1.26 and Node 20 or newer.

1. Start PostgreSQL and Redis:

   ```bash
   docker compose up -d
   ```

2. Copy `config.example.yml` to `config.yml` and fill in the secrets. The
   `bulutsantralim` block needs your API key (OIM, Bulut Santralım, Santral
   Ayarlarım), the SIP domain and the WSS URL. `config.yml` is gitignored.

3. Run the backend. It applies migrations, seeds the permission catalog, the
   system roles and the owner account on first start:

   ```bash
   cd backend
   go run ./cmd/santral -config ../config.yml
   ```

4. Run the frontend. The dev server proxies `/api` to the backend; point it at
   another port with `VITE_API_TARGET` if you changed `app.port`:

   ```bash
   cd frontend
   npm install
   npm run dev
   ```

Sign in with the owner credentials from `config.yml`. You will be asked to set
a new password, and to enroll TOTP if `security.requireMFA` is on.

Then create the agents under **Kullanıcılar**, give each one their extension
and pull the SIP password with "Verimor'dan çek" (the extension must be
attached to a personnel record in OIM). Each agent gets a temporary password
and the welcome text to paste into a message.

Checks before pushing:

```bash
cd backend && go build ./... && go vet ./... && go test ./...
cd frontend && npm run build
```

## Deploying

Production runs on a single Ubuntu host: nginx serves the Vite build and
proxies `/api` to the Go binary under systemd, PostgreSQL and Redis on the same
machine, Cloudflare in front. `DEPLOY.md` has the full walkthrough. Updates are
one command on the server, run as a sudo-capable user:

```bash
cd /opt/santral-c && bash deploy/deploy.sh
```

The build is stamped with the git SHA and time; the sidebar shows it so you can
tell at a glance whether the running version is the latest.

## Recovering a locked-out owner

`backend/cmd/resetpw` sets a user's password straight in the database, clears
MFA and unlocks the account. If the e-mail does not exist it is created as an
invisible admin:

```bash
cd backend
go run ./cmd/resetpw -config ../config.yml -email owner@example.com -password 'NewPass123!'
```

## Things worth knowing about the Verimor API

These cost us time, so they are written down here.

- `/cdrs` without a date range returns only today's calls, and pages deeper
  than the first are slow enough to time out. The backend therefore keeps a
  rolling window from page 1 and filters in memory; past dates use
  `start_stamp_from` and `start_stamp_to`, which are complete but take around
  fifteen seconds per page.
- The `number` filter does not work for short internal extensions; it returns
  nearly the whole tenant. Extension filtering is done in process by matching
  the `1014 (9021...)` and `9021... (1008)` party formats.
- `/user_statuses` takes about twenty seconds and is limited to a couple of
  calls a minute. It is polled in the background; the panel never calls it
  directly.
- Bursts of requests answer with 429. The poller spaces its calls and backs off
  after a throttle.
- The webphone page that exposes SIP passwords is served only to Turkish IPs.
  From a server abroad you get the OIM login page instead, so "Verimor'dan çek"
  works from a machine in Turkey; otherwise enter the password by hand.

## Repository layout

```
backend/     Go service: cmd/santral (server), cmd/resetpw, internal/* modules, migrations
frontend/    React panel and softphone
extension/   Chrome MV3 mini widget that relays the panel's call to every tab
deploy/      deploy.sh, systemd unit, nginx site
```

## License

Private, internal project.
