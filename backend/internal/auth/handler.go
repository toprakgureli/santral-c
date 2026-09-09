package auth

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the auth HTTP endpoints.
type Handler struct {
	cfg     configs.Auth
	service IService
}

// NewHandler builds an auth handler.
func NewHandler(cfg configs.Auth, service IService) *Handler {
	return &Handler{cfg: cfg, service: service}
}

// Login validates credentials and returns a session or a challenge.
func (h *Handler) Login(c *fiber.Ctx) error {
	var req requests.Login
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	result, err := h.service.Login(c.UserContext(), req, metaFrom(c))
	if err != nil {
		return err
	}
	if result.MFARequired || result.MFASetupRequired {
		return c.JSON(responses.LoginChallenge{
			MFARequired:      result.MFARequired,
			MFASetupRequired: result.MFASetupRequired,
			MFAToken:         result.MFAToken,
		})
	}
	return h.session(c, result)
}

// Refresh rotates the refresh cookie into a new session.
func (h *Handler) Refresh(c *fiber.Ctx) error {
	result, err := h.service.Refresh(c.UserContext(), c.Cookies(h.cfg.RefreshCookieName), metaFrom(c))
	if err != nil {
		h.clearSession(c)
		return err
	}
	return h.session(c, result)
}

// Logout revokes the session and clears cookies.
func (h *Handler) Logout(c *fiber.Ctx) error {
	if err := h.service.Logout(c.UserContext(), c.Cookies(h.cfg.CookieName), c.Cookies(h.cfg.RefreshCookieName)); err != nil {
		return err
	}
	h.clearSession(c)
	return c.SendStatus(fiber.StatusNoContent)
}

// Me returns the authenticated user.
func (h *Handler) Me(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	res, err := h.service.Me(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// MFAVerify completes the login second factor.
func (h *Handler) MFAVerify(c *fiber.Ctx) error {
	var req requests.MFAVerify
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	result, err := h.service.MFAVerify(c.UserContext(), req.Token, req.Code, metaFrom(c))
	if err != nil {
		return err
	}
	return h.session(c, result)
}

// MFAEnroll begins forced enrollment during login.
func (h *Handler) MFAEnroll(c *fiber.Ctx) error {
	var req requests.MFAEnroll
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.MFAEnroll(c.UserContext(), req.Token)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// MFAEnrollVerify completes forced enrollment during login.
func (h *Handler) MFAEnrollVerify(c *fiber.Ctx) error {
	var req requests.MFAEnrollVerify
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	result, err := h.service.MFAEnrollVerify(c.UserContext(), req.Token, req.Code, metaFrom(c))
	if err != nil {
		return err
	}
	return h.session(c, result)
}

// MFASetup starts voluntary TOTP setup for a logged-in user.
func (h *Handler) MFASetup(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	res, err := h.service.MFASetup(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// MFAEnable verifies a code and turns on TOTP.
func (h *Handler) MFAEnable(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	var req requests.MFACode
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	if err := h.service.MFAEnable(c.UserContext(), id, req.Code); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// PasswordChange completes the forced first-login password step.
func (h *Handler) PasswordChange(c *fiber.Ctx) error {
	var req requests.PasswordChange
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	result, err := h.service.PasswordChange(c.UserContext(), req, metaFrom(c))
	if err != nil {
		return err
	}
	return h.session(c, result)
}

func (h *Handler) session(c *fiber.Ctx, result *LoginResult) error {
	if result.PasswordChangeRequired {
		return c.JSON(responses.LoginChallenge{
			PasswordChangeRequired: true,
			PasswordToken:          result.PasswordToken,
		})
	}
	h.setSession(c, result)
	return c.JSON(responses.Login{User: result.User})
}

func (h *Handler) setSession(c *fiber.Ctx, result *LoginResult) {
	h.cookie(c, h.cfg.CookieName, result.Token, result.ExpiresAt)
	h.cookie(c, h.cfg.RefreshCookieName, result.RefreshToken, result.RefreshExpiresAt)
}

func (h *Handler) clearSession(c *fiber.Ctx) {
	past := time.Now().Add(-time.Hour)
	h.cookie(c, h.cfg.CookieName, "", past)
	h.cookie(c, h.cfg.RefreshCookieName, "", past)
}

func (h *Handler) cookie(c *fiber.Ctx, name, value string, expires time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    value,
		Expires:  expires,
		Path:     "/",
		HTTPOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: "Lax",
	})
}

func metaFrom(c *fiber.Ctx) RequestMeta {
	agent := c.Get(fiber.HeaderUserAgent)
	if len(agent) > 255 {
		agent = agent[:255]
	}
	return RequestMeta{IP: c.IP(), UserAgent: agent}
}
