package shift

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the shift endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a shift handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Current returns the actor's open shift and the day's cutoffs.
func (h *Handler) Current(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Current(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Start opens the actor's shift.
func (h *Handler) Start(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Start(c.UserContext(), id, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// End closes the actor's shift.
func (h *Handler) End(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.End(c.UserContext(), id, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Router mounts the shift endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a shift router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the shift routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/shift", r.guard)
	group.Get("/", r.handler.Current)
	group.Post("/start", r.handler.Start)
	group.Post("/end", r.handler.End)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}
