package user

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Router mounts the user administration endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a user router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the user routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/users", r.guard)

	group.Get("/", r.need(enums.UserView), r.handler.List)
	group.Put("/me/whatsapp-template", r.handler.SetMyWhatsAppTemplate)
	group.Put("/me/avatar", r.handler.SetMyAvatar)
	group.Get("/:id/avatar", r.handler.Avatar)
	group.Post("/", r.need(enums.UserCreate), r.handler.Create)
	group.Put("/:id", r.need(enums.UserUpdate), r.handler.Update)
	group.Patch("/:id/active", r.need(enums.UserDeactivate), r.handler.SetActive)
	group.Patch("/:id/roles", r.need(enums.RoleAssign), r.handler.SetRoles)
	group.Post("/:id/password", r.need(enums.UserUpdate), r.handler.ResetPassword)
}
