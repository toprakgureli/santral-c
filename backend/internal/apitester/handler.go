package apitester

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves HTTP endpoints for API test collections, execution and logs.
type Handler struct {
	service *Service
}

// NewHandler constructs an API tester HTTP handler.
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
		return 0, errs.Invalid("Geçersiz kayıt kimliği.", err)
	}
	return uint(raw), nil
}

// Status returns whether the optional API tester menu module is active.
func (h *Handler) Status(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	enabled, err := h.service.Status(c.UserContext(), uid)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"enabled": enabled})
}

// SetStatus toggles the optional API tester module on or off.
func (h *Handler) SetStatus(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	enabled, err := h.service.SetStatus(c.UserContext(), uid, req.Enabled, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"enabled": enabled})
}

// List returns collections and requests visible to the signed-in user.
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

// CreateCollection creates a new collection.
func (h *Handler) CreateCollection(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	var req CollectionInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	res, err := h.service.CreateCollection(c.UserContext(), uid, req, c.IP())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// ImportCollection imports a collection together with its requests atomically.
func (h *Handler) ImportCollection(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	var req ImportCollectionInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz içe aktarma verisi.", err)
	}
	res, err := h.service.ImportCollection(c.UserContext(), uid, req, c.IP())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// UpdateCollection updates a collection.
func (h *Handler) UpdateCollection(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req CollectionInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	res, err := h.service.UpdateCollection(c.UserContext(), uid, id, req, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// DeleteCollection removes a collection.
func (h *Handler) DeleteCollection(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.service.DeleteCollection(c.UserContext(), uid, id, c.IP()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// CreateRequest adds a request template under a collection.
func (h *Handler) CreateRequest(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	colID, err := parseID(c)
	if err != nil {
		return err
	}
	var req RequestInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	res, err := h.service.CreateRequest(c.UserContext(), uid, colID, req, c.IP())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// UpdateRequest updates an existing request template.
func (h *Handler) UpdateRequest(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req RequestInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	res, err := h.service.UpdateRequest(c.UserContext(), uid, id, req, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// DeleteRequest removes a request template.
func (h *Handler) DeleteRequest(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if err := h.service.DeleteRequest(c.UserContext(), uid, id, c.IP()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ExecuteProxy executes a saved request through the Santral-C backend proxy.
func (h *Handler) ExecuteProxy(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req ProxyInput
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return errs.Invalid("Geçersiz istek gövdesi.", err)
		}
	}
	res, err := h.service.ExecuteProxy(c.UserContext(), uid, id, req, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// LogClientRun records metadata of a client-side (browser or extension) execution.
func (h *Handler) LogClientRun(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req LogRunInput
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("Geçersiz istek gövdesi.", err)
	}
	if err := h.service.LogClientRun(c.UserContext(), uid, id, req, c.IP()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ListRunLogs returns recent execution logs.
func (h *Handler) ListRunLogs(c *fiber.Ctx) error {
	uid, err := actorID(c)
	if err != nil {
		return err
	}
	var reqID *uint
	if raw := c.Query("requestId"); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			return errs.Invalid("Geçersiz istek filtresi.", err)
		}
		id := uint(v)
		reqID = &id
	}
	limit := c.QueryInt("limit", 30)
	items, err := h.service.ListRunLogs(c.UserContext(), uid, reqID, limit)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": items})
}

// Router mounts the API tester routes onto g.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds an API tester router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers all API tester endpoints under /api-tester.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/api-tester", r.guard)
	view := r.need(enums.APITesterView, enums.APITesterExecute, enums.APITesterManage)
	exec := r.need(enums.APITesterExecute, enums.APITesterManage)
	manage := r.need(enums.APITesterManage)
	toggle := r.need(enums.APITesterManage, enums.SystemSettings)

	group.Get("/status", r.handler.Status)
	group.Put("/status", toggle, r.handler.SetStatus)

	group.Get("/collections", view, r.handler.List)
	group.Post("/collections", manage, r.handler.CreateCollection)
	group.Post("/collections/import", manage, r.handler.ImportCollection)
	group.Put("/collections/:id", manage, r.handler.UpdateCollection)
	group.Delete("/collections/:id", manage, r.handler.DeleteCollection)

	group.Post("/collections/:id/requests", manage, r.handler.CreateRequest)
	group.Put("/requests/:id", manage, r.handler.UpdateRequest)
	group.Delete("/requests/:id", manage, r.handler.DeleteRequest)

	group.Post("/requests/:id/proxy", exec, r.handler.ExecuteProxy)
	group.Post("/requests/:id/log", exec, r.handler.LogClientRun)
	group.Get("/logs", view, r.handler.ListRunLogs)
}
