package role

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the role endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a role router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the role routes onto g.
func (r *Router) Routes(g fiber.Router) {
	g.Get("/roles", r.guard, r.handler.List)
}
