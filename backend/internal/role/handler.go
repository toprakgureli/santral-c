package role

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the role endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a role handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List returns the assignable roles.
func (h *Handler) List(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	res, err := h.service.List(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}
