package calllog

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Router mounts the call-log endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a call-log router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the call-log routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/calls/log", r.guard)
	view := r.need(enums.CDRViewAll, enums.CallViewAll, enums.CDRViewOwn, enums.CallViewOwn)
	group.Post("/", r.need(enums.CallOriginate, enums.CDRViewOwn, enums.CallViewOwn), r.handler.Record)
	group.Get("/", view, r.handler.Recent)
	group.Get("/lookup", view, r.handler.Lookup)
}
