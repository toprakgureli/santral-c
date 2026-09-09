# santral-c

A call manager for a small contact center: a Go control plane and a browser
softphone in front of Asterisk, with a CRM-ready data model, roles and
permissions, TOTP, and detailed call logging.

See [DESIGN.md](DESIGN.md) for the architecture and the phased build plan.

## Stack

- **Control plane**: Go (Fiber, GORM, PostgreSQL, Redis), Uber Go Style Guide.
- **Media engine**: Asterisk 22 (chan_pjsip). The control plane observes and
  originates calls over AMI and provisions PJSIP endpoints by generating config
  and reloading, which keeps the runtime image free of a realtime schema.
- **Agents**: browser softphone (React + TypeScript + Tailwind, SIP.js over
  WebSocket, Opus).

## Status

- [x] Auth: JWT sessions, TOTP, forced first-login onboarding, lockout
- [x] User and role administration with audit logging
- [x] CRM contacts with E.164 lookup
- [x] Telephony schema: calls, event timeline, RTCP quality
- [x] Asterisk media engine, AMI control plane, SIP provisioning
- [x] React panel: softphone, call log, contacts, users
- [ ] Verimor trunk cutover (Phase 4, when proven and reversible)

## Running locally

Requires Docker, Go 1.26 and Node 20+.

1. **Infrastructure** (PostgreSQL, Redis, Asterisk):

   ```bash
   cp deploy/asterisk/etc/pjsip_users.conf.template deploy/asterisk/etc/pjsip_users.conf
   docker compose up -d
   ```

2. **Config**: copy `config.example.yml` to `config.yml` and set secrets. The
   `asterisk.usersConfigPath` must be the absolute path to the
   `pjsip_users.conf` mounted into the Asterisk container.

3. **Backend**:

   ```bash
   cd backend
   go run ./cmd/santral --config ../config.yml
   ```

   It runs migrations, seeds the owner account, connects to AMI and syncs
   PJSIP on startup.

4. **Frontend**:

   ```bash
   cd frontend
   npm install
   npm run dev
   ```

   The dev server proxies `/api` to the backend. Open the printed URL and sign
   in with the owner credentials from `config.yml`; you are prompted to set a
   new password on first login.

## Trying a call

Create a user with a SIP extension (for example 1005) in the Users screen; the
endpoint is provisioned automatically. Sign in as that user in a second browser
profile, allow the microphone, and dial another provisioned or static extension
(1001 and 1002 are preconfigured WebRTC endpoints, 600 is an echo test). Every
call is recorded under Calls with its event timeline.

## License

Private, internal project.
