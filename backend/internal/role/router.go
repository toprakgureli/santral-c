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
	group := g.Group("/roles", r.guard)
	group.Get("/permissions", r.handler.Permissions) // before /:id so it is not shadowed
	group.Get("/", r.handler.List)
	group.Post("/", r.handler.Create)
	group.Put("/:id", r.handler.Update)
	group.Delete("/:id", r.handler.Delete)
}
