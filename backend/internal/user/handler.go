package user

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the user administration endpoints.
type Handler struct {
	service IManagement
}

// NewHandler builds a user handler.
func NewHandler(service IManagement) *Handler {
	return &Handler{service: service}
}

// Create provisions a new account with roles and a forced password change.
func (h *Handler) Create(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.UserCreate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.CreateUser(c.UserContext(), actorID, req, meta(c))
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// List returns a filtered page of users.
func (h *Handler) List(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	filter := requests.UserFilter{
		Query:   c.Query("query"),
		Page:    c.QueryInt("page", 1),
		PerPage: c.QueryInt("perPage", 20),
	}
	if raw := c.Query("roleId"); raw != "" {
		if id, convErr := strconv.ParseUint(raw, 10, 64); convErr == nil {
			roleID := uint(id)
			filter.RoleID = &roleID
		}
	}
	if raw := c.Query("active"); raw != "" {
		if active, convErr := strconv.ParseBool(raw); convErr == nil {
			filter.Active = &active
		}
	}
	res, err := h.service.List(c.UserContext(), actorID, filter)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// SetActive activates or deactivates a user.
func (h *Handler) SetActive(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	targetID, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.UserActive
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.SetActive(c.UserContext(), actorID, targetID, req.Active, meta(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ResetPassword sets a new password and forces a change at next login.
func (h *Handler) ResetPassword(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	targetID, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.UserPassword
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	if err := h.service.ResetPassword(c.UserContext(), actorID, targetID, req.Password, meta(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Update edits a user's profile and role assignment.
func (h *Handler) Update(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	targetID, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.UserUpdate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.UpdateUser(c.UserContext(), actorID, targetID, req, meta(c))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// SetRoles replaces a user's role assignment.
func (h *Handler) SetRoles(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	targetID, err := param(c, "id")
	if err != nil {
		return err
	}
	var req requests.UserRoles
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.SetRoles(c.UserContext(), actorID, targetID, req.RoleIDs, meta(c))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// SetMyWhatsAppTemplate stores the caller's own WhatsApp follow-up text.
func (h *Handler) SetMyWhatsAppTemplate(c *fiber.Ctx) error {
	actorID, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.WhatsAppTemplate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.SetWhatsAppTemplate(c.UserContext(), actorID, req.Template)
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

func param(c *fiber.Ctx, name string) (uint, error) {
	raw := c.Params(name)
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, errs.Invalid("Geçersiz kullanıcı kimliği.", err)
	}
	return uint(id), nil
}

func meta(c *fiber.Ctx) Meta {
	return Meta{IP: c.IP()}
}
