package security

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the security page endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a security router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the security routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/security", r.guard)
	group.Get("/attempts", r.handler.Attempts)
	group.Get("/bans", r.handler.Bans)
	group.Delete("/bans/:id", r.handler.Unban)
}
