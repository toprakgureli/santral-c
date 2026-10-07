package notice

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the notice endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a notice handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List returns the actor's notices, only those after ?after= when given.
func (h *Handler) List(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	after, _ := strconv.ParseUint(c.Query("after"), 10, 64)
	out, err := h.service.List(c.UserContext(), id, uint(after))
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// Read marks the actor's notices read.
func (h *Handler) Read(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req struct {
		IDs []uint `json:"ids"`
		All bool   `json:"all"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.MarkRead(c.UserContext(), id, req.IDs, req.All); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Router mounts the notice endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a notice router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the notice routes onto g. Every signed-in person reads
// and clears their own notices.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/notices", r.guard)
	group.Get("/", r.handler.List)
	group.Post("/read", r.handler.Read)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	return id, nil
}
