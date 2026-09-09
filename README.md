# santral-c

A call manager for a small contact center: a Go control plane and a browser
softphone in front of Asterisk, with a CRM-ready data model, virtual keys and
roles, TOTP, and detailed call logging.

Early development. See [DESIGN.md](DESIGN.md) for the architecture and the
phased build plan.

## Stack

- **Control plane**: Go (Fiber, GORM, PostgreSQL, Redis), Uber Go Style Guide.
- **Media engine**: Asterisk (chan_pjsip + ARI), Verimor SIP trunk.
- **Agents**: browser softphone (React + TypeScript + Tailwind + shadcn/ui,
  SIP.js over WSS, Opus).

## Status

- [x] Project skeleton, auth schema, role and permission taxonomy
- [] Auth port (JWT, TOTP, onboarding, lockout, guard)
- [ ] Phase 0 Asterisk (two softphones through our Asterisk)
- [ ] Phase 1 Go control plane (ARI, CDR, events, quality)
- [ ] Phase 2 React panel and softphone

## License

Private, internal project.
