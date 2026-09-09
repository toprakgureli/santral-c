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
	group.Post("/", r.handler.Create)
	group.Patch("/:id/active", r.handler.SetActive)
	group.Post("/:id/password", r.handler.ResetPassword)
}
