package custommenu

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the custom menu HTTP endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a custom menu handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func actorID(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok || id == 0 {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	return id, nil
}

func parseID(c *fiber.Ctx) (uint, error) {
	raw, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || raw == 0 {
		return 0, errs.Invalid("Geçersiz menü kimliği.", err)
	}
	return uint(raw), nil
}

// Visible returns the active custom menus the signed-in user may see.
func (h *Handler) Visible(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	res, err := h.service.Visible(c.UserContext(), uid)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Check inspects whether a custom menu's target URL allows iframe embedding.
func (h *Handler) Check(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	res, err := h.service.Check(c.UserContext(), uid, id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// List returns all custom menus (admin).
func (h *Handler) List(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	res, err := h.service.List(c.UserContext(), uid)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Roles returns all selectable roles for the menu permission form.
func (h *Handler) Roles(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	res, err := h.service.Roles(c.UserContext(), uid)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Create creates a new custom menu item.
func (h *Handler) Create(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	var req Input
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	res, err := h.service.Create(c.UserContext(), uid, req, c.IP())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// Update updates an existing custom menu item.
func (h *Handler) Update(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req Input
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	res, err := h.service.Update(c.UserContext(), uid, id, req, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Delete deletes a custom menu item.
func (h *Handler) Delete(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.UserContext(), uid, id, c.IP()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Reorder updates the display order of custom menu items.
func (h *Handler) Reorder(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	if err := h.service.Reorder(c.UserContext(), uid, req.IDs); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Router mounts the custom menu routes.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a custom menu router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the custom menu endpoints onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/menus", r.guard)
	manage := r.need(enums.MenuManage)

	group.Get("/visible", r.handler.Visible)
	group.Get("/:id/check", r.handler.Check)

	group.Get("/", manage, r.handler.List)
	group.Get("/roles", manage, r.handler.Roles)
	group.Post("/", manage, r.handler.Create)
	group.Put("/order", manage, r.handler.Reorder)
	group.Put("/:id", manage, r.handler.Update)
	group.Delete("/:id", manage, r.handler.Delete)
}
