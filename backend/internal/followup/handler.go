package followup

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the follow-up endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a follow-up handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Unreached lists the numbers owed a call: ?scope=all for everyone's,
// ?state=closed for those closed in the last week.
func (h *Handler) Unreached(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	out, err := h.service.Unreached(c.UserContext(), id, c.Query("scope") == "all", c.Query("state") != "closed")
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// Peer tells what the team knows about ?number= during a call.
func (h *Handler) Peer(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	out, err := h.service.Peer(c.UserContext(), id, c.Query("number"))
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// Claim marks a row as being called back by the actor.
func (h *Handler) Claim(c *fiber.Ctx) error { return h.act(c, h.service.Claim) }

// Unclaim lets go of a row.
func (h *Handler) Unclaim(c *fiber.Ctx) error { return h.act(c, h.service.Unclaim) }

// Drop takes a row off the list.
func (h *Handler) Drop(c *fiber.Ctx) error { return h.act(c, h.service.Drop) }

func (h *Handler) act(c *fiber.Ctx, fn func(ctx context.Context, actorID, id uint) error) error {
	uid, err := actor(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return errs.Invalid("Geçersiz kayıt.", err)
	}
	if err := fn(c.UserContext(), uid, uint(id)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Router mounts the follow-up endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
	need    middlewares.Requirer
}

// NewRouter builds a follow-up router.
func NewRouter(handler *Handler, guard fiber.Handler, need middlewares.Requirer) *Router {
	return &Router{handler: handler, guard: guard, need: need}
}

// Routes registers the follow-up routes onto g. Anyone with a phone line or
// their own calls in view uses them; everyone's list needs call.view_all,
// which the service checks.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/followups", r.guard)
	use := r.need(enums.CallOriginate, enums.CallViewOwn, enums.CDRViewOwn, enums.CallViewAll, enums.CDRViewAll)
	group.Get("/peer", use, r.handler.Peer)
	group.Get("/unreached", use, r.handler.Unreached)
	group.Post("/unreached/:id/claim", use, r.handler.Claim)
	group.Delete("/unreached/:id/claim", use, r.handler.Unclaim)
	group.Post("/unreached/:id/drop", use, r.handler.Drop)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yap.")
	}
	return id, nil
}
