package contact

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Router mounts the contact endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a contact router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the contact routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/contacts", r.guard)
	view, manage := r.need(enums.ContactView), r.need(enums.ContactManage)

	group.Get("/lookup", view, r.handler.Lookup) // before /:id so it is not shadowed
	group.Get("/", view, r.handler.List)
	group.Post("/", manage, r.handler.Create)
	group.Get("/:id", view, r.handler.Get)
	group.Patch("/:id", manage, r.handler.Update)
	group.Delete("/:id", manage, r.handler.Delete)
	group.Post("/:id/phones", manage, r.handler.AddPhone)
	group.Delete("/:id/phones/:phoneId", manage, r.handler.RemovePhone)
}
