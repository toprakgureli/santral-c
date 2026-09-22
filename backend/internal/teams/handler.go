package teams

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"image"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	// webp decoder for the group photo check.
	_ "golang.org/x/image/webp"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the chat endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a chat handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// People lists users the chat can address.
func (h *Handler) People(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.People(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Overview returns the left column.
func (h *Handler) Overview(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Overview(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

type createBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PostPolicy  string `json:"postPolicy"`
	MemberIDs   []uint `json:"memberIds"`
}

// Create opens a group.
func (h *Handler) Create(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req createBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.Create(c.UserContext(), id, CreateInput{Name: req.Name, Description: req.Description, PostPolicy: req.PostPolicy, MemberIDs: req.MemberIDs})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// OpenDM returns or creates the direct message with a user.
func (h *Handler) OpenDM(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	other, err := param(c, "uid")
	if err != nil {
		return err
	}
	res, err := h.service.OpenDM(c.UserContext(), id, other)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Detail returns a room with members.
func (h *Handler) Detail(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	res, err := h.service.Detail(c.UserContext(), id, gid)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

type updateBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PostPolicy  string `json:"postPolicy"`
}

// Update edits a group's text and policy.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req updateBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.Update(c.UserContext(), id, gid, UpdateInput{Name: req.Name, Description: req.Description, PostPolicy: req.PostPolicy})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// The group photo follows the same rule as the profile photo: a small webp
// data URI the browser produced, checked again here.
const (
	avatarPrefix   = "data:image/webp;base64,"
	avatarMaxSide  = 512
	avatarMaxBytes = 90_000
)

// SetAvatar stores or removes the group photo.
func (h *Handler) SetAvatar(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		Avatar string `json:"avatar"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	avatar := strings.TrimSpace(req.Avatar)
	if avatar != "" {
		if !strings.HasPrefix(avatar, avatarPrefix) {
			return errs.Invalid("Fotoğraf webp biçiminde olmalı.", nil)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(avatar, avatarPrefix))
		if err != nil || len(raw) > avatarMaxBytes {
			return errs.Invalid("Fotoğraf okunamadı ya da çok büyük.", nil)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || format != "webp" || cfg.Width > avatarMaxSide || cfg.Height > avatarMaxSide {
			return errs.Invalid("Fotoğraf geçerli bir webp değil ya da 512 pikselden büyük.", nil)
		}
	}
	res, err := h.service.SetAvatar(c.UserContext(), id, gid, avatar)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Avatar serves the group photo, or 204.
func (h *Handler) Avatar(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	value, err := h.service.Avatar(c.UserContext(), id, gid)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(value, avatarPrefix) {
		return c.SendStatus(fiber.StatusNoContent)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, avatarPrefix))
	if err != nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	c.Set("Content-Type", "image/webp")
	c.Set("Cache-Control", "private, max-age=300")
	return c.Send(raw)
}

// Delete removes a group.
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.UserContext(), id, gid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type idsBody struct {
	UserIDs []uint `json:"userIds"`
}

// AddMembers seats users directly.
func (h *Handler) AddMembers(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req idsBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.AddMembers(c.UserContext(), id, gid, req.UserIDs)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Invite opens invites.
func (h *Handler) Invite(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req idsBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.Invite(c.UserContext(), id, gid, req.UserIDs)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// DecideInvite accepts or declines the caller's invite.
func (h *Handler) DecideInvite(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	iid, err := param(c, "id")
	if err != nil {
		return err
	}
	accept := c.Params("decision") == "accept"
	if err := h.service.DecideInvite(c.UserContext(), id, iid, accept); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RemoveMember unseats a user or lets the caller leave.
func (h *Handler) RemoveMember(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	uid, err := param(c, "uid")
	if err != nil {
		return err
	}
	if err := h.service.RemoveMember(c.UserContext(), id, gid, uid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type memberBody struct {
	Role    *string `json:"role"`
	CanPost *bool   `json:"canPost"`
}

// UpdateMember changes a seat.
func (h *Handler) UpdateMember(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	uid, err := param(c, "uid")
	if err != nil {
		return err
	}
	var req memberBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.UpdateMember(c.UserContext(), id, gid, uid, MemberInput{Role: req.Role, CanPost: req.CanPost})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Mute silences or restores a room for the caller.
func (h *Handler) Mute(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		Muted bool `json:"muted"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.SetMuted(c.UserContext(), id, gid, req.Muted); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Read moves the caller's read pointer.
func (h *Handler) Read(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		MessageID uint `json:"messageId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.MarkRead(c.UserContext(), id, gid, req.MessageID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Unread flags a room unread for the actor.
func (h *Handler) Unread(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.MarkUnread(c.UserContext(), id, gid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Messages pages a room's history.
func (h *Handler) Messages(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	before := uint(c.QueryInt("before", 0))
	items, more, err := h.service.Messages(c.UserContext(), id, gid, before)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": items, "more": more})
}

type sendBody struct {
	Body      string `json:"body"`
	ReplyToID uint   `json:"replyToId"`
}

// Send posts a line.
func (h *Handler) Send(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req sendBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.Send(c.UserContext(), id, gid, req.Body, req.ReplyToID)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(res)
}

// DeleteMessage soft-deletes a line.
func (h *Handler) DeleteMessage(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	mid, err := param(c, "mid")
	if err != nil {
		return err
	}
	if err := h.service.DeleteMessage(c.UserContext(), id, gid, mid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// React toggles an emoji on a line.
func (h *Handler) React(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	mid, err := param(c, "mid")
	if err != nil {
		return err
	}
	var req struct {
		Emoji string `json:"emoji"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.React(c.UserContext(), id, gid, mid, req.Emoji); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Stream pushes room events to the caller over Server-Sent Events.
func (h *Handler) Stream(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	ch, err := h.service.Subscribe(c.UserContext(), id)
	if err != nil {
		return err
	}
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer h.service.Unsubscribe(id, ch)
		if _, err := w.WriteString("data: {\"type\":\"hello\"}\n\n"); err != nil {
			return
		}
		if w.Flush() != nil {
			return
		}
		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if _, err := w.WriteString("data: "); err != nil {
					return
				}
				if _, err := w.Write(msg); err != nil {
					return
				}
				if _, err := w.WriteString("\n\n"); err != nil {
					return
				}
				if w.Flush() != nil {
					return
				}
			case <-heartbeat.C:
				if _, err := w.WriteString(": ping\n\n"); err != nil {
					return
				}
				if w.Flush() != nil {
					return
				}
			}
		}
	})
	return nil
}

// Router mounts the chat endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a chat router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the chat routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/teams", r.guard)
	group.Get("/people", r.handler.People)
	group.Get("/overview", r.handler.Overview)
	group.Get("/stream", r.handler.Stream)
	group.Post("/groups", r.handler.Create)
	group.Post("/dm/:uid", r.handler.OpenDM)
	group.Post("/invites/:id/:decision", r.handler.DecideInvite)
	group.Get("/groups/:id", r.handler.Detail)
	group.Put("/groups/:id", r.handler.Update)
	group.Delete("/groups/:id", r.handler.Delete)
	group.Put("/groups/:id/avatar", r.handler.SetAvatar)
	group.Get("/groups/:id/avatar", r.handler.Avatar)
	group.Post("/groups/:id/members", r.handler.AddMembers)
	group.Put("/groups/:id/members/:uid", r.handler.UpdateMember)
	group.Delete("/groups/:id/members/:uid", r.handler.RemoveMember)
	group.Post("/groups/:id/invites", r.handler.Invite)
	group.Post("/groups/:id/mute", r.handler.Mute)
	group.Post("/groups/:id/read", r.handler.Read)
	group.Post("/groups/:id/unread", r.handler.Unread)
	group.Get("/groups/:id/messages", r.handler.Messages)
	group.Post("/groups/:id/messages", r.handler.Send)
	group.Delete("/groups/:id/messages/:mid", r.handler.DeleteMessage)
	group.Post("/groups/:id/messages/:mid/reactions", r.handler.React)
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
		return 0, errs.Invalid("Geçersiz kimlik.", err)
	}
	return uint(id), nil
}
