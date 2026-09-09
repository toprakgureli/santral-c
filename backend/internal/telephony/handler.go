package telephony

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the call endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a telephony handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Originate starts an outbound call from the actor's extension.
func (h *Handler) Originate(c *fiber.Ctx) error {
	actorID, err := actor(c)
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
	if err := h.service.Originate(c.UserContext(), actorID, req); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusAccepted)
}

// List returns a filtered page of calls.
func (h *Handler) List(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	filter := requests.CallFilter{
		Direction:   c.Query("direction"),
		Disposition: c.Query("disposition"),
		Number:      c.Query("number"),
		Page:        c.QueryInt("page", 1),
		PerPage:     c.QueryInt("perPage", 20),
	}
	if raw := c.Query("userId"); raw != "" {
		if id, convErr := strconv.ParseUint(raw, 10, 64); convErr == nil {
			uid := uint(id)
			filter.UserID = &uid
		}
	}
	if t := parseTime(c.Query("from")); t != nil {
		filter.From = t
	}
	if t := parseTime(c.Query("to")); t != nil {
		filter.To = t
	}
	res, err := h.service.List(c.UserContext(), actorID, filter)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Get returns a call with its timeline and quality samples.
func (h *Handler) Get(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return errs.Invalid("Geçersiz çağrı kimliği.", err)
	}
	res, err := h.service.Get(c.UserContext(), actorID, uint(id))
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

func parseTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t
	}
	return nil
}
