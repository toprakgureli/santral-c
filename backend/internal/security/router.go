package security

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Router mounts the security page endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a security router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the security routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/security", r.guard, r.need(enums.SystemLogs))
	group.Get("/attempts", r.handler.Attempts)
	group.Get("/bans", r.handler.Bans)
	// Reading the logs is not enough to lift a ban.
	group.Delete("/bans/:id", r.need(enums.SystemSettings), r.handler.Unban)
}
