package verimor

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/internal/sse"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

// Handler serves the telephony endpoints backed by Bulutsantralim.
type Handler struct {
	service *Service
}

// NewHandler builds a Verimor handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Webphone returns the embedded softphone URL for the logged-in agent.
func (h *Handler) Webphone(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.WebphoneURL(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Credentials returns the logged-in agent's SIP registration data.
func (h *Handler) Credentials(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Credentials(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// SetCredentials stores a user's SIP extension and password.
func (h *Handler) SetCredentials(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	targetID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return errs.Invalid("Geçersiz kullanıcı kimliği.", err)
	}
	var req requests.SIPCredentials
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	if err := h.service.SetCredentials(c.UserContext(), id, uint(targetID), req.Extension, req.Password, c.IP()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// SyncCredentials pulls the SIP password for a user's extension from Verimor and
// stores it, so the admin only supplies the extension.
func (h *Handler) SyncCredentials(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	targetID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return errs.Invalid("Geçersiz kullanıcı kimliği.", err)
	}
	var req struct {
		Extension string `json:"extension"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := h.service.ProvisionSIP(c.UserContext(), id, uint(targetID), strings.TrimSpace(req.Extension), c.IP()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// SyncAllCredentials pulls SIP passwords from Verimor for every user that has an
// extension assigned.
func (h *Handler) SyncAllCredentials(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	ok, failed, err := h.service.SyncAllSIP(c.UserContext(), id, c.IP())
	if err != nil {
		return err
	}
	exts := make([]string, len(failed))
	for i, f := range failed {
		exts[i] = f.Extension
	}
	return c.JSON(fiber.Map{"synced": ok, "failed": len(failed), "failedExtensions": exts, "failures": failed})
}

// Calls returns a page of call records.
func (h *Handler) Calls(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Calls(c.UserContext(), id, Filter{
		Direction: c.Query("direction"),
		Number:    c.Query("number"),
		Scope:     c.Query("scope"),
		From:      c.Query("from"),
		To:        c.Query("to"),
		Page:      c.QueryInt("page", 1),
		Limit:     c.QueryInt("perPage", 20),
	})
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// ExportCalls streams the filtered call list as a CSV download.
func (h *Handler) ExportCalls(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	data, err := h.service.ExportCalls(c.UserContext(), id, Filter{
		Direction: c.Query("direction"),
		Number:    c.Query("number"),
		Scope:     c.Query("scope"),
		From:      c.Query("from"),
		To:        c.Query("to"),
	})
	if err != nil {
		return err
	}
	name := "cagrilar"
	if from := safeName(c.Query("from")); from != "" {
		name += "-" + from
		if to := safeName(c.Query("to")); to != "" && to != from {
			name += "_" + to
		}
	}
	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
	return c.Send(data)
}

// Extensions lists extensions with live status (for transfer shortcuts).
func (h *Handler) Extensions(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Extensions(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// Queues lists call queues (for transfer shortcuts).
func (h *Handler) Queues(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Queues(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": res})
}

// SetStatus records the actor's presence state.
func (h *Handler) SetStatus(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.AgentStatus
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	state := req.State
	if state == "" {
		// Backward compatibility with the earlier boolean payload.
		if req.DND {
			state = "dnd"
		} else {
			state = "available"
		}
	}
	if err := h.service.SetStatus(c.UserContext(), id, state); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Status returns the actor's current presence state.
func (h *Handler) Status(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Status(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Stats returns today's call totals.
func (h *Handler) Stats(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Stats(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Recording streams a call's recording (audio) through the backend so the panel
// can play or download it same-origin. Add ?download=1 to force a download.
func (h *Handler) Recording(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	uuid := c.Params("uuid")
	if uuid == "" {
		return errs.Invalid("Çağrı kimliği zorunlu.", nil)
	}
	f, err := h.service.Recording(c.UserContext(), id, uuid, RecordingAccess{
		IP:       c.IP(),
		Download: c.Query("download") != "",
	})
	if err != nil {
		return err
	}
	middlewares.FileHeaders(c, f.Type, "kayit-"+safeName(uuid)+".mp3", c.Query("download") != "")
	c.Set("Accept-Ranges", "bytes")
	c.Set("Cache-Control", "private, max-age=3600")
	// Honour a Range request so the audio element can seek (it expects 206).
	if start, end, ok := parseRange(c.Get("Range"), len(f.Data)); ok {
		c.Status(fiber.StatusPartialContent)
		c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(f.Data)))
		return c.Send(f.Data[start : end+1])
	}
	return c.Send(f.Data)
}

// safeName keeps only characters that are safe in a download file name.
func safeName(v string) string {
	out := make([]rune, 0, len(v))
	for _, r := range v {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_' {
			out = append(out, r)
		}
	}
	return string(out)
}

// parseRange handles a single "bytes=start-end" range against a body of `size`.
func parseRange(header string, size int) (int, int, bool) {
	if size == 0 || !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, 0, false // multi-range not supported
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false
	}
	startStr, endStr := spec[:dash], spec[dash+1:]
	var start, end int
	switch {
	case startStr == "" && endStr != "": // suffix: last N bytes
		n, err := strconv.Atoi(endStr)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		start, end = size-n, size-1
	case startStr != "":
		s, err := strconv.Atoi(startStr)
		if err != nil || s < 0 || s >= size {
			return 0, 0, false
		}
		start = s
		if endStr == "" {
			end = size - 1
		} else {
			e, err := strconv.Atoi(endStr)
			if err != nil || e < start {
				return 0, 0, false
			}
			end = e
			if end >= size {
				end = size - 1
			}
		}
	default:
		return 0, 0, false
	}
	return start, end, true
}

// Stream pushes live agent-list updates to the panel over Server-Sent Events,
// so presence changes appear without polling.
func (h *Handler) Stream(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	initial, ch, err := h.service.StreamStart(c.UserContext(), id)
	if err != nil {
		return err
	}
	return sse.Serve(c, sse.Stream{
		First:   initial,
		Events:  ch,
		Close:   func() { h.service.StreamStop(ch) },
		Allowed: func(ctx context.Context) error { return h.service.StreamAllowed(ctx, id, ch) },
	})
}

// Originate starts a click-to-call from the actor's extension.
func (h *Handler) Originate(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.CallOriginate
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	uuid, err := h.service.Originate(c.UserContext(), id, req.To)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"callUuid": uuid})
}

// Transfer lets the softphone hand the call in progress to another number.
func (h *Handler) Transfer(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req requests.CallTransfer
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	if err := h.service.AuthorizeTransfer(c.UserContext(), id, req.CallID, strings.TrimSpace(req.Target)); err != nil {
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
