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
	g.Get("/sip/credentials", r.guard, r.handler.Credentials)
	g.Post("/users/:id/sip", r.guard, r.handler.SetCredentials)
	g.Get("/calls", r.guard, r.handler.Calls)
	g.Post("/calls/originate", r.guard, r.handler.Originate)
	g.Get("/pbx/extensions", r.guard, r.handler.Extensions)
	g.Get("/pbx/stream", r.guard, r.handler.Stream)
	g.Get("/pbx/queues", r.guard, r.handler.Queues)
	g.Get("/pbx/status", r.guard, r.handler.Status)
	g.Post("/pbx/status", r.guard, r.handler.SetStatus)
	g.Get("/pbx/stats", r.guard, r.handler.Stats)
}
