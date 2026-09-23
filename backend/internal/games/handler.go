package games

import (
	"encoding/json"
	"io"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler exposes the games over HTTP.
type Handler struct {
	service *Service
}

// NewHandler builds a handler.
func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
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
	if err != nil || id == 0 {
		return 0, errs.Invalid("Geçersiz kimlik.", nil)
	}
	return uint(id), nil
}

// Config returns switches, catalogue and content counts.
func (h *Handler) Config(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Configuration(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// UpdateSettings flips the switches.
func (h *Handler) UpdateSettings(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req Settings
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.UpdateSettings(c.UserContext(), id, req); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type itemBody struct {
	Text    string   `json:"text"`
	Answer  string   `json:"answer"`
	Options []string `json:"options"`
	Seconds int      `json:"seconds"`
	Active  *bool    `json:"active"`
}

// Items lists content of a kind.
func (h *Handler) Items(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Items(c.UserContext(), id, c.Query("kind"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// CreateItem adds content.
func (h *Handler) CreateItem(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req itemBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.CreateItem(c.UserContext(), id, c.Query("kind"), ItemInput{Text: req.Text, Answer: req.Answer, Options: req.Options, Seconds: req.Seconds, Active: req.Active})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// UpdateItem edits content.
func (h *Handler) UpdateItem(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	iid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req itemBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.UpdateItem(c.UserContext(), id, iid, ItemInput{Text: req.Text, Answer: req.Answer, Options: req.Options, Seconds: req.Seconds, Active: req.Active}); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// DeleteItem removes content.
func (h *Handler) DeleteItem(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	iid, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.DeleteItem(c.UserContext(), id, iid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ImportItems reads a spreadsheet of content.
func (h *Handler) ImportItems(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	file, err := c.FormFile("file")
	if err != nil {
		return errs.Invalid("Dosya bulunamadı.", err)
	}
	if file.Size > 4<<20 {
		return errs.Invalid("Dosya 4 MB'tan büyük olamaz.", nil)
	}
	f, err := file.Open()
	if err != nil {
		return errs.Invalid("Dosya açılamadı.", err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return errs.Invalid("Dosya okunamadı.", err)
	}
	added, err := h.service.ImportItems(c.UserContext(), id, c.Query("kind"), file.Filename, data)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"added": added})
}

// Create opens a match in a room.
func (h *Handler) Create(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "gid")
	if err != nil {
		return err
	}
	var req struct {
		Kind    string `json:"kind"`
		Rounds  int    `json:"rounds"`
		Seconds int    `json:"seconds"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.Create(c.UserContext(), id, gid, CreateInput{Kind: req.Kind, Rounds: req.Rounds, Seconds: req.Seconds})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// Open lists a room's unfinished matches.
func (h *Handler) Open(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "gid")
	if err != nil {
		return err
	}
	res, err := h.service.Open(c.UserContext(), id, gid)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Get returns one match.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	mid, err := param(c, "id")
	if err != nil {
		return err
	}
	res, err := h.service.Get(c.UserContext(), id, mid)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

func (h *Handler) simple(fn func(c *fiber.Ctx, id, mid uint) (*GameView, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := actor(c)
		if err != nil {
			return err
		}
		mid, err := param(c, "id")
		if err != nil {
			return err
		}
		res, err := fn(c, id, mid)
		if err != nil {
			return err
		}
		return c.JSON(res)
	}
}

// Cancel scraps a match.
func (h *Handler) Cancel(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	mid, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.Cancel(c.UserContext(), id, mid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Pause stops or restarts the clock for a player on a call.
func (h *Handler) Pause(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	mid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		Paused bool `json:"paused"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.Pause(c.UserContext(), id, mid, req.Paused)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Act applies a move.
func (h *Handler) Act(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	mid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		Action  string          `json:"action"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.Act(c.UserContext(), id, mid, req.Action, req.Payload)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Leaderboard tallies a period.
func (h *Handler) Leaderboard(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Leaderboard(c.UserContext(), id, c.Query("period", "month"), c.Query("kind"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Record tallies one person's games.
func (h *Handler) Record(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	uid, err := param(c, "uid")
	if err != nil {
		return err
	}
	res, err := h.service.UserRecord(c.UserContext(), id, uid)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Router mounts the routes.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a router.
func NewRouter(h *Handler, guard fiber.Handler) *Router {
	return &Router{handler: h, guard: guard}
}

// Routes registers the routes under /games.
func (r *Router) Routes(api fiber.Router) {
	g := api.Group("/games", r.guard)
	g.Get("/config", r.handler.Config)
	g.Put("/settings", r.handler.UpdateSettings)
	g.Get("/items", r.handler.Items)
	g.Post("/items", r.handler.CreateItem)
	g.Put("/items/:id", r.handler.UpdateItem)
	g.Delete("/items/:id", r.handler.DeleteItem)
	g.Post("/items/import", r.handler.ImportItems)
	g.Get("/leaderboard", r.handler.Leaderboard)
	g.Get("/record/:uid", r.handler.Record)
	g.Post("/groups/:gid", r.handler.Create)
	g.Get("/groups/:gid/open", r.handler.Open)
	g.Get("/:id", r.handler.Get)
	g.Post("/:id/join", r.handler.simple(func(c *fiber.Ctx, id, mid uint) (*GameView, error) {
		return r.handler.service.Join(c.UserContext(), id, mid)
	}))
	g.Post("/:id/leave", r.handler.simple(func(c *fiber.Ctx, id, mid uint) (*GameView, error) {
		return r.handler.service.Leave(c.UserContext(), id, mid)
	}))
	g.Post("/:id/start", r.handler.simple(func(c *fiber.Ctx, id, mid uint) (*GameView, error) {
		return r.handler.service.Start(c.UserContext(), id, mid)
	}))
	g.Post("/:id/cancel", r.handler.Cancel)
	g.Post("/:id/pause", r.handler.Pause)
	g.Post("/:id/action", r.handler.Act)
}
