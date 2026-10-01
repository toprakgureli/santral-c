package role

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Router mounts the role endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a role router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the role routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/roles", r.guard)
	view, manage := r.need(enums.RoleView), r.need(enums.RoleManage)
	group.Get("/permissions", view, r.handler.Permissions) // before /:id so it is not shadowed
	// Whoever may give roles needs to see them to choose one.
	group.Get("/", r.need(enums.RoleView, enums.RoleAssign), r.handler.List)
	group.Post("/", manage, r.handler.Create)
	group.Put("/:id", manage, r.handler.Update)
	group.Delete("/:id", manage, r.handler.Delete)
}
