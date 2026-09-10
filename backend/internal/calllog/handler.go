package calllog

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the call-log endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a call-log handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Record ingests one phase of a softphone call.
func (h *Handler) Record(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.CallLogEvent
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	if err := h.service.Record(c.UserContext(), id, req); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Recent returns the panel's recent call history.
func (h *Handler) Recent(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Recent(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}
