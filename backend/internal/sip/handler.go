package sip

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the SIP endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a SIP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Credentials returns the logged-in user's softphone registration data.
func (h *Handler) Credentials(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	res, err := h.service.Credentials(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}
