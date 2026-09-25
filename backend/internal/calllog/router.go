package calllog

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the call-log endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a call-log router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the call-log routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/calls/log", r.guard)
	group.Post("/", r.handler.Record)
	group.Get("/", r.handler.Recent)
	group.Get("/lookup", r.handler.Lookup)
}
