package escalation

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the escalation endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds an escalation router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the escalation routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/escalations", r.guard)

	group.Get("/categories", r.handler.Categories)
	group.Post("/categories", r.handler.CreateCategory)
	group.Post("/categories/import", r.handler.Import)
	group.Delete("/categories/:id", r.handler.DeleteCategory)
	group.Post("/categories/:id/reasons", r.handler.CreateReason)
	group.Delete("/reasons/:id", r.handler.DeleteReason)

	group.Get("/", r.handler.History)
	group.Post("/", r.handler.Log)
}
