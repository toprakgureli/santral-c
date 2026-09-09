package telephony

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the call endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a telephony router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the call routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/calls", r.guard)

	group.Get("/", r.handler.List)
	group.Post("/originate", r.handler.Originate)
	group.Get("/:id", r.handler.Get)
}
