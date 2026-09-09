package auth

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
)

// Router mounts the auth endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds an auth router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the auth routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/auth")

	credentials := func() fiber.Handler { return middlewares.RateLimit(20, time.Minute) }

	group.Post("/login", credentials(), r.handler.Login)
	group.Post("/refresh", middlewares.RateLimit(60, time.Minute), r.handler.Refresh)
	group.Post("/logout", r.handler.Logout)
	group.Get("/me", r.guard, r.handler.Me)
	group.Post("/mfa/verify", credentials(), r.handler.MFAVerify)
	group.Post("/mfa/enroll", credentials(), r.handler.MFAEnroll)
	group.Post("/mfa/enroll/verify", credentials(), r.handler.MFAEnrollVerify)
	group.Post("/password/change", credentials(), r.handler.PasswordChange)
	group.Post("/mfa/setup", r.guard, r.handler.MFASetup)
	group.Post("/mfa/enable", r.guard, r.handler.MFAEnable)
}
