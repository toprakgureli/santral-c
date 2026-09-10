package role

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
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
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.List(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Permissions returns the permission catalog grouped by module.
func (h *Handler) Permissions(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Permissions(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Create adds a custom role.
func (h *Handler) Create(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.RoleCreate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.Create(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// Update edits a role.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	roleID, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.RoleUpdate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.Update(c.UserContext(), id, roleID, req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Delete removes a custom role.
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	roleID, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.UserContext(), id, roleID); err != nil {
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
