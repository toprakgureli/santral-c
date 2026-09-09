package contact

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the contact endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a contact router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the contact routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/contacts", r.guard)

	group.Get("/lookup", r.handler.Lookup) // before /:id so it is not shadowed
	group.Get("/", r.handler.List)
	group.Post("/", r.handler.Create)
	group.Get("/:id", r.handler.Get)
	group.Patch("/:id", r.handler.Update)
	group.Delete("/:id", r.handler.Delete)
	group.Post("/:id/phones", r.handler.AddPhone)
	group.Delete("/:id/phones/:phoneId", r.handler.RemovePhone)
}
