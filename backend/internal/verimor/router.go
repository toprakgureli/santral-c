package verimor

import (
	"github.com/gofiber/fiber/v2"
)

// Router mounts the telephony endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a Verimor router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the telephony routes onto g.
func (r *Router) Routes(g fiber.Router) {
	g.Get("/webphone", r.guard, r.handler.Webphone)
	g.Get("/calls", r.guard, r.handler.Calls)
	g.Post("/calls/originate", r.guard, r.handler.Originate)
}
