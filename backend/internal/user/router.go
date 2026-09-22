package user

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the user administration endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a user router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the user routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/users", r.guard)

	group.Get("/", r.handler.List)
	group.Put("/me/whatsapp-template", r.handler.SetMyWhatsAppTemplate)
	group.Post("/", r.handler.Create)
	group.Put("/:id", r.handler.Update)
	group.Patch("/:id/active", r.handler.SetActive)
	group.Patch("/:id/roles", r.handler.SetRoles)
	group.Post("/:id/password", r.handler.ResetPassword)
}
