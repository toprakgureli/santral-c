package contact

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the contact endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a contact handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List returns a filtered page of contacts.
func (h *Handler) List(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	filter := requests.ContactFilter{
		Query:   c.Query("query"),
		Page:    c.QueryInt("page", 1),
		PerPage: c.QueryInt("perPage", 20),
	}
	res, err := h.service.List(c.UserContext(), actorID, filter)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Lookup resolves a contact by phone number.
func (h *Handler) Lookup(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	number := c.Query("number")
	if number == "" {
		return errs.Invalid("number parametresi zorunlu.", nil)
	}
	res, err := h.service.ResolveByNumber(c.UserContext(), actorID, number)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Get returns one contact.
func (h *Handler) Get(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	id, err := param(c, "id")
	if err != nil {
		return err
	}
	res, err := h.service.Get(c.UserContext(), actorID, id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Create stores a new contact.
func (h *Handler) Create(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.ContactCreate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.Create(c.UserContext(), actorID, req, meta(c))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// Update edits a contact's core fields.
func (h *Handler) Update(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	id, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.ContactUpdate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.Update(c.UserContext(), actorID, id, req, meta(c))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Delete soft-deletes a contact.
func (h *Handler) Delete(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	id, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.UserContext(), actorID, id, meta(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// AddPhone attaches a phone number to a contact.
func (h *Handler) AddPhone(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	id, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.ContactPhoneInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.AddPhone(c.UserContext(), actorID, id, req, meta(c))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// RemovePhone detaches a phone number from a contact.
func (h *Handler) RemovePhone(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	id, err := param(c, "id")
	if err != nil {
		return err
	}
	phoneID, err := param(c, "phoneId")
	if err != nil {
		return err
	}
	if err := h.service.RemovePhone(c.UserContext(), actorID, id, phoneID, meta(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}

func param(c *fiber.Ctx, name string) (uint, error) {
	id, err := strconv.ParseUint(c.Params(name), 10, 64)
	if err != nil {
		return 0, errs.Invalid("Geçersiz kimlik.", err)
	}
	return uint(id), nil
}

func meta(c *fiber.Ctx) Meta {
	return Meta{IP: c.IP()}
}
