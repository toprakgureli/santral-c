package teams

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	// webp decoder for the group photo check.
	_ "golang.org/x/image/webp"

	"github.com/toprakgureli/santral-c/backend/configs"
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
		Mute  string `json:"mute"`
		Muted *bool  `json:"muted"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	level := req.Mute
	if level == "" && req.Muted != nil {
		level = map[bool]string{true: "mentions", false: "none"}[*req.Muted]
	}
	if err := h.service.SetMuted(c.UserContext(), id, gid, level); err != nil {
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
	if around := uint(c.QueryInt("around", 0)); around > 0 {
		items, older, newer, err := h.service.MessagesAround(c.UserContext(), id, gid, around)
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"items": items, "more": older, "moreNewer": newer})
	}
	if after := uint(c.QueryInt("after", 0)); after > 0 {
		items, more, err := h.service.MessagesAfter(c.UserContext(), id, gid, after)
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"items": items, "more": false, "moreNewer": more})
	}
	before := uint(c.QueryInt("before", 0))
	items, more, err := h.service.Messages(c.UserContext(), id, gid, before)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": items, "more": more, "moreNewer": false})
}

// Search finds lines in a room.
func (h *Handler) Search(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	items, err := h.service.Search(c.UserContext(), id, gid, c.Query("q"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": items})
}

// Media lists a room's shared files.
func (h *Handler) Media(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	items, more, err := h.service.Media(c.UserContext(), id, gid, c.Query("kind"), uint(c.QueryInt("before", 0)))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": items, "more": more})
}

type sendBody struct {
	Body          string `json:"body"`
	ReplyToID     uint   `json:"replyToId"`
	MentionIDs    []uint `json:"mentionIds"`
	MentionsAll   bool   `json:"mentionsAll"`
	AttachmentIDs []uint `json:"attachmentIds"`
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
	res, err := h.service.Send(c.UserContext(), id, gid, req.Body, req.ReplyToID, req.MentionIDs, req.MentionsAll, req.AttachmentIDs)
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

// Edit rewrites one of the caller's own lines.
func (h *Handler) Edit(c *fiber.Ctx) error {
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
	var req sendBody
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.EditMessage(c.UserContext(), id, gid, mid, req.Body, req.MentionIDs, req.MentionsAll)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Typing relays "X is writing" to the room.
func (h *Handler) Typing(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	if err := h.service.Typing(c.UserContext(), id, gid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Presence records whether the caller is looking at a room.
func (h *Handler) Presence(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req struct {
		Room uint `json:"room"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.SetPresence(c.UserContext(), id, req.Room); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Receipts lists who received and read a line, and when.
func (h *Handler) Receipts(c *fiber.Ctx) error {
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
	res, err := h.service.MessageReceipts(c.UserContext(), id, gid, mid)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// ---------------------------------------------------------------- attachments

// BeginUpload opens a Drive upload session for a file.
func (h *Handler) BeginUpload(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	gid, err := param(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		Name string `json:"name"`
		Mime string `json:"mime"`
		Size int64  `json:"size"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	origin := c.Get("Origin")
	if origin == "" {
		origin = strings.TrimRight(configs.Cnf.App.PublicURL, "/")
	}
	res, err := h.service.BeginUpload(c.UserContext(), id, gid, UploadInput{Name: req.Name, Mime: req.Mime, Size: req.Size}, origin)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// FinishUpload verifies the uploaded file and marks it ready.
func (h *Handler) FinishUpload(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	aid, err := param(c, "aid")
	if err != nil {
		return err
	}
	var req struct {
		DriveID    string `json:"driveId"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		DurationMs int    `json:"durationMs"`
		Thumb      string `json:"thumb"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	res, err := h.service.FinishUpload(c.UserContext(), id, aid, FinishInput{DriveID: req.DriveID, Width: req.Width, Height: req.Height, DurationMs: req.DurationMs, Thumb: req.Thumb})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// CancelUpload drops a pending upload.
func (h *Handler) CancelUpload(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	aid, err := param(c, "aid")
	if err != nil {
		return err
	}
	if err := h.service.CancelUpload(c.UserContext(), id, aid); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Attachment streams a file from Drive to a seated reader. Range requests
// pass through, so video can seek.
func (h *Handler) Attachment(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	aid, err := param(c, "aid")
	if err != nil {
		return err
	}
	a, err := h.service.OpenAttachment(c.UserContext(), id, aid)
	if err != nil {
		return err
	}
	resp, err := h.service.StreamAttachment(c.UserContext(), a, c.Get("Range"))
	if err != nil {
		return err
	}
	c.Status(resp.StatusCode)
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(k); v != "" {
			c.Set(k, v)
		}
	}
	if resp.Header.Get("Content-Type") == "" {
		c.Set("Content-Type", a.Mime)
	}
	c.Set("Accept-Ranges", "bytes")
	c.Set("Cache-Control", "private, max-age=86400")
	disposition := "inline"
	if c.Query("download") == "1" || a.Kind == "file" {
		disposition = "attachment"
	}
	c.Set("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, url.PathEscape(a.Name)))
	size := -1
	if resp.ContentLength >= 0 {
		size = int(resp.ContentLength)
	}
	c.Context().SetBodyStream(resp.Body, size)
	return nil
}

// Thumb serves the small preview kept in the database.
func (h *Handler) Thumb(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	aid, err := param(c, "aid")
	if err != nil {
		return err
	}
	a, err := h.service.OpenAttachment(c.UserContext(), id, aid)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(a.Thumb, thumbPrefix) {
		return c.SendStatus(fiber.StatusNoContent)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(a.Thumb, thumbPrefix))
	if err != nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	c.Set("Content-Type", "image/webp")
	c.Set("Cache-Control", "private, max-age=86400")
	return c.Send(raw)
}

// ---------------------------------------------------------------- drive admin

// DriveStatus reports the storage connection to the settings card.
func (h *Handler) DriveStatus(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.DriveState(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// DriveConnect sends the administrator to Google.
func (h *Handler) DriveConnect(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	target, err := h.service.DriveConnectURL(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.Redirect(target, fiber.StatusFound)
}

// DriveCallback is where Google returns with the code.
func (h *Handler) DriveCallback(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	if msg := c.Query("error"); msg != "" {
		return c.Redirect("/settings?drive=error&reason="+url.QueryEscape(msg), fiber.StatusFound)
	}
	if _, err := h.service.DriveCallback(c.UserContext(), id, c.Query("state"), c.Query("code")); err != nil {
		return c.Redirect("/settings?drive=error&reason="+url.QueryEscape(err.Error()), fiber.StatusFound)
	}
	return c.Redirect("/settings?drive=ok", fiber.StatusFound)
}

// DriveDisconnect forgets the linked account.
func (h *Handler) DriveDisconnect(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	if err := h.service.DriveDisconnect(c.UserContext(), id); err != nil {
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
	group.Put("/groups/:id/messages/:mid", r.handler.Edit)
	group.Get("/groups/:id/messages/:mid/receipts", r.handler.Receipts)
	group.Get("/groups/:id/search", r.handler.Search)
	group.Get("/groups/:id/media", r.handler.Media)
	group.Post("/groups/:id/typing", r.handler.Typing)
	group.Post("/groups/:id/uploads", r.handler.BeginUpload)
	group.Post("/uploads/:aid/finish", r.handler.FinishUpload)
	group.Delete("/uploads/:aid", r.handler.CancelUpload)
	group.Get("/attachments/:aid", r.handler.Attachment)
	group.Get("/attachments/:aid/thumb", r.handler.Thumb)
	group.Get("/drive/status", r.handler.DriveStatus)
	group.Get("/drive/connect", r.handler.DriveConnect)
	group.Get("/drive/callback", r.handler.DriveCallback)
	group.Post("/drive/disconnect", r.handler.DriveDisconnect)
	group.Post("/presence", r.handler.Presence)
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
