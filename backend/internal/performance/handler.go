package performance

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
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

// Today returns the team page payload for the actor. Optional from/to query
// values (local YYYY-MM-DD, inclusive) widen the figures to a day range.
func (h *Handler) Today(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	from, to := c.Query("from"), c.Query("to")
	if from == "" && to == "" {
		res, err := h.service.Today(c.UserContext(), id)
		if err != nil {
			return err
		}
		return c.JSON(res)
	}
	if from == "" {
		from = to
	}
	if to == "" {
		to = from
	}
	res, err := h.service.Range(c.UserContext(), id, from, to)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// AgentCalls lists one agent's calls for the userId and from/to query values.
func (h *Handler) AgentCalls(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	userID := c.QueryInt("userId")
	if userID < 1 {
		return errs.Invalid("Geçersiz temsilci.", nil)
	}
	from, to := c.Query("from"), c.Query("to")
	if from == "" {
		from = to
	}
	if to == "" {
		to = from
	}
	if from == "" {
		from, to = todayLocal(), todayLocal()
	}
	res, err := h.service.AgentCalls(c.UserContext(), id, uint(userID), from, to)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Alerts lists what needs a team lead's look right now.
func (h *Handler) Alerts(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	res, err := h.service.Alerts(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Router mounts the performance endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a performance router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the performance routes onto g.
func (r *Router) Routes(g fiber.Router) {
	view := r.need(enums.PerformanceViewAll, enums.PerformanceViewRole)
	g.Get("/performance/today", r.guard, view, r.handler.Today)
	g.Get("/performance/calls", r.guard, view, r.handler.AgentCalls)
	g.Get("/performance/alerts", r.guard, r.need(enums.PerformanceLiveAlerts), r.handler.Alerts)
}
