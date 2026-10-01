package backup

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler exposes the backup settings over HTTP.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	return id, nil
}

// Settings returns the settings and the last runs.
func (h *Handler) Settings(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	v, err := h.service.Settings(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(v)
}

// Save stores the settings.
func (h *Handler) Save(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var in Input
	if err := c.BodyParser(&in); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	v, err := h.service.Save(c.UserContext(), id, in, c.IP())
	if err != nil {
		return err
	}
	return c.JSON(v)
}

// Check reports what the service account may do in the folder.
func (h *Handler) Check(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	check, err := h.service.Check(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"folder": check, "problem": check.Problem()})
}

// Run starts a backup now.
func (h *Handler) Run(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	v, err := h.service.RunNow(c.UserContext(), id, c.IP())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(v)
}

// Router mounts the backup routes.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds the router.
func NewRouter(h *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: h, guard: guard, need: need}
}

// Routes registers the routes under /backup.
func (r *Router) Routes(api fiber.Router) {
	g := api.Group("/backup", r.guard, r.need(enums.SystemBackup))
	g.Get("/", r.handler.Settings)
	g.Put("/", r.handler.Save)
	g.Post("/check", r.handler.Check)
	g.Post("/run", r.handler.Run)
}
