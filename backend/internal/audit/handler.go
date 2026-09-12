package audit

import (
	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the audit trail endpoint.
type Handler struct {
	reader *Reader
}

// NewHandler builds an audit handler.
func NewHandler(reader *Reader) *Handler {
	return &Handler{reader: reader}
}

// List returns a page of audit entries.
func (h *Handler) List(c *fiber.Ctx) error {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	res, err := h.reader.List(c.UserContext(), id, requests.AuditFilter{
		Action:  c.Query("action"),
		Query:   c.Query("query"),
		Page:    c.QueryInt("page", 1),
		PerPage: c.QueryInt("perPage", 0),
	})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Router mounts the audit endpoint.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds an audit router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the audit routes onto g.
func (r *Router) Routes(g fiber.Router) {
	g.Get("/audit", r.guard, r.handler.List)
}
