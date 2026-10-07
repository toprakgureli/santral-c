package setting

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the system settings endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a settings handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Get returns the current flags.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Settings(c.UserContext(), id, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Update writes the flags.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.SettingsUpdate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.Update(c.UserContext(), id, req, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// BreakLimit returns the daily break allowance in minutes. Every signed-in
// user may read it (the break card needs it); changing it is gated.
func (h *Handler) BreakLimit(c *fiber.Ctx) error {
	if _, err := actor(c); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"minutes": h.service.BreakLimitMinutes(c.UserContext())})
}

// UpdateBreakLimit stores the daily break allowance.
func (h *Handler) UpdateBreakLimit(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req struct {
		Minutes int `json:"minutes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.SetBreakLimit(c.UserContext(), id, req.Minutes, c.IP()); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"minutes": req.Minutes})
}

// RealCall returns how long an answered call must last to count as real.
// Every signed-in user may read it: the call screens label their numbers
// with it.
func (h *Handler) RealCall(c *fiber.Ctx) error {
	if _, err := actor(c); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"seconds": h.service.RealCallSeconds(c.UserContext())})
}

// UpdateRealCall stores that threshold.
func (h *Handler) UpdateRealCall(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req struct {
		Seconds int `json:"seconds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.SetRealCallSeconds(c.UserContext(), id, req.Seconds, c.IP()); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"seconds": req.Seconds})
}

// Workday returns when the working day ends and each role's daily target.
// Every signed-in user may read it: the panel times the day's summary and
// shows the target with it.
func (h *Handler) Workday(c *fiber.Ctx) error {
	if _, err := actor(c); err != nil {
		return err
	}
	out, err := h.service.Workday(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// UpdateWorkday stores them.
func (h *Handler) UpdateWorkday(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req WorkdayInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	out, err := h.service.SetWorkday(c.UserContext(), id, req, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// Router mounts the settings endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a settings router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the settings routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/settings", r.guard)
	group.Get("/", r.handler.Get)
	group.Put("/", r.need(enums.SystemSettings), r.handler.Update)
	group.Get("/break-limit", r.handler.BreakLimit)
	group.Get("/real-call", r.handler.RealCall)
	group.Put("/real-call", r.need(enums.CallRealSeconds), r.handler.UpdateRealCall)
	group.Get("/workday", r.handler.Workday)
	group.Put("/workday", r.need(enums.AgentWorkday), r.handler.UpdateWorkday)
	group.Put("/break-limit", r.need(enums.AgentBreakLimit), r.handler.UpdateBreakLimit)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	return id, nil
}
