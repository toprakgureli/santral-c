package setting

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
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
	res, err := h.service.Settings(c.UserContext(), id)
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

// Router mounts the settings endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a settings router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the settings routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/settings", r.guard)
	group.Get("/", r.handler.Get)
	group.Put("/", r.handler.Update)
	group.Get("/break-limit", r.handler.BreakLimit)
	group.Put("/break-limit", r.handler.UpdateBreakLimit)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}
