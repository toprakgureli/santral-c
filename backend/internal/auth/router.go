package auth

import (
	"net"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
)

// Router mounts the auth endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	trusted []*net.IPNet
}

// NewRouter builds an auth router. trusted are the office's addresses,
// which the per-address sign-in limit leaves out.
func NewRouter(handler *Handler, guard fiber.Handler, trusted []*net.IPNet) *Router {
	return &Router{handler: handler, guard: guard, trusted: trusted}
}

// Routes registers the auth routes onto g.
func (r *Router) Routes(g fiber.Router) {
	// Every browser gets a device id here, so wrong passwords and request
	// limits are counted per browser, not per office address.
	group := g.Group("/auth", middlewares.Device(r.handler.cfg.CookieSecure))

	credentials := func() fiber.Handler { return middlewares.RateLimitBy(20, time.Minute, middlewares.PerDevice) }
	// A browser that drops its cookie is a new browser every time, so the
	// steps that check a password or a code are also limited per address,
	// for every address but the office's.
	outside := middlewares.RateLimitOutside(30, time.Minute, r.trusted)

	group.Post("/login", outside, credentials(), r.handler.Login)
	group.Post("/refresh", middlewares.RateLimitBy(30, time.Minute, middlewares.PerCookie(r.handler.cfg.RefreshCookieName)), r.handler.Refresh)
	group.Post("/logout", r.handler.Logout)
	group.Get("/me", r.guard, r.handler.Me)
	group.Post("/mfa/verify", outside, credentials(), r.handler.MFAVerify)
	group.Post("/mfa/enroll", outside, credentials(), r.handler.MFAEnroll)
	group.Post("/mfa/enroll/verify", outside, credentials(), r.handler.MFAEnrollVerify)
	group.Post("/password/change", outside, credentials(), r.handler.PasswordChange)
	group.Post("/password", r.guard, credentials(), r.handler.ChangeOwnPassword)
	group.Post("/logout/everywhere", r.guard, r.handler.LogoutEverywhere)
	group.Post("/mfa/setup", r.guard, r.handler.MFASetup)
	group.Post("/mfa/enable", r.guard, r.handler.MFAEnable)
}
