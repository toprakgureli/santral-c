package security

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the security page endpoints.
type Handler struct {
	admin *Admin
}

// NewHandler builds a security handler.
func NewHandler(admin *Admin) *Handler {
	return &Handler{admin: admin}
}

// Attempts returns a page of login attempts.
func (h *Handler) Attempts(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	f := requests.SecurityFilter{
		Email:   c.Query("email"),
		IP:      c.Query("ip"),
		Page:    c.QueryInt("page", 1),
		PerPage: c.QueryInt("perPage", 0),
	}
	if v := c.Query("success"); v != "" {
		ok := v == "true"
		f.Success = &ok
	}
	res, err := h.admin.Attempts(c.UserContext(), id, f)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Bans lists the active IP bans.
func (h *Handler) Bans(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.admin.Bans(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Unban lifts one IP ban.
func (h *Handler) Unban(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	banID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || banID == 0 {
		return errs.Invalid("Geçersiz kayıt numarası.", err)
	}
	if err := h.admin.Unban(c.UserContext(), id, uint(banID), c.IP()); err != nil {
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
