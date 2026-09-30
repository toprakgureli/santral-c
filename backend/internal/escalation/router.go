package escalation

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Router mounts the escalation endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds an escalation router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the escalation routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/escalations", r.guard)

	group.Get("/categories", r.handler.Categories)
	manage := r.need(enums.EscalationManage)
	group.Post("/categories", manage, r.handler.CreateCategory)
	group.Post("/categories/import", manage, r.handler.Import)
	group.Put("/categories/order", manage, r.handler.ReorderCategories)
	group.Put("/categories/:id/reasons/order", manage, r.handler.ReorderReasons)
	group.Delete("/categories/:id", manage, r.handler.DeleteCategory)
	group.Post("/categories/:id/reasons", manage, r.handler.CreateReason)
	group.Delete("/reasons/:id", manage, r.handler.DeleteReason)

	group.Get("/", r.need(enums.EscalationSearch), r.handler.History)
	group.Get("/list", r.handler.List)
	group.Get("/agents", r.handler.Agents)
	group.Post("/", r.handler.Log)
	group.Post("/none", r.handler.LogNone)
}
