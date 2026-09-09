package sip

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the SIP endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a SIP router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the SIP routes onto g.
func (r *Router) Routes(g fiber.Router) {
	g.Get("/sip/credentials", r.guard, r.handler.Credentials)
}
