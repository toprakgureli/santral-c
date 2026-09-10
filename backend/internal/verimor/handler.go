package verimor

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the telephony endpoints backed by Bulutsantralim.
type Handler struct {
	service *Service
}

// NewHandler builds a Verimor handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Webphone returns the embedded softphone URL for the logged-in agent.
func (h *Handler) Webphone(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.WebphoneURL(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Credentials returns the logged-in agent's SIP registration data.
func (h *Handler) Credentials(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Credentials(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// SetCredentials stores a user's SIP extension and password.
func (h *Handler) SetCredentials(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	targetID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return errs.Invalid("Geçersiz kullanıcı kimliği.", err)
	}
	var req requests.SIPCredentials
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	if err := h.service.SetCredentials(c.UserContext(), id, uint(targetID), req.Extension, req.Password); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Calls returns a page of call records.
func (h *Handler) Calls(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Calls(c.UserContext(), id, Filter{
		Direction: c.Query("direction"),
		Number:    c.Query("number"),
		Page:      c.QueryInt("page", 1),
		Limit:     c.QueryInt("perPage", 20),
	})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Extensions lists extensions with live status (for transfer shortcuts).
func (h *Handler) Extensions(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Extensions(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Queues lists call queues (for transfer shortcuts).
func (h *Handler) Queues(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Queues(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Status sets the actor's do-not-disturb state.
func (h *Handler) Status(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.AgentStatus
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.SetStatus(c.UserContext(), id, req.DND); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Stats returns today's call totals.
func (h *Handler) Stats(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Stats(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Originate starts a click-to-call from the actor's extension.
func (h *Handler) Originate(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.CallOriginate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	uuid, err := h.service.Originate(c.UserContext(), id, req.To)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"callUuid": uuid})
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}
