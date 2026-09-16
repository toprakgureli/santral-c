package performance

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the team performance endpoint.
type Handler struct {
	service *Service
}

// NewHandler builds a performance handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Today returns the team page payload for the actor.
func (h *Handler) Today(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	res, err := h.service.Today(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Router mounts the performance endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a performance router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the performance routes onto g.
func (r *Router) Routes(g fiber.Router) {
	g.Get("/performance/today", r.guard, r.handler.Today)
}
