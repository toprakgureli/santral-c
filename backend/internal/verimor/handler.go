package verimor

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
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
	if err := h.service.SetCredentials(c.UserContext(), id, uint(targetID), req.Extension, req.Password); err != nil {
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
	if err := h.service.ProvisionSIP(c.UserContext(), id, uint(targetID), strings.TrimSpace(req.Extension)); err != nil {
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
	ok, fail, err := h.service.SyncAllSIP(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"synced": ok, "failed": fail})
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
		Page:      c.QueryInt("page", 1),
		Limit:     c.QueryInt("perPage", 20),
	})
	if err != nil {
		return err
	}
	return c.JSON(res)
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
	res, err := h.service.Recording(c.UserContext(), id, uuid)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	// Buffer the whole recording (they are small) and send it with a
	// Content-Length so the browser's audio element can seek freely.
	const maxRecording = 64 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, maxRecording))
	if err != nil {
		return errs.New(errs.CodeConflict, 502, "Çağrı kaydı okunamadı.", err)
	}
	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	c.Set("Content-Type", contentType)
	c.Set("Accept-Ranges", "bytes")
	c.Set("Cache-Control", "private, max-age=3600")
	if c.Query("download") != "" {
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="kayit-%s.mp3"`, uuid))
	}
	// Honour a Range request so the audio element can seek (it expects 206).
	if start, end, ok := parseRange(c.Get("Range"), len(data)); ok {
		c.Status(fiber.StatusPartialContent)
		c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		return c.Send(data[start : end+1])
	}
	return c.Send(data)
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

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no") // disable nginx buffering for this response

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer h.service.StreamStop(ch)
		writeSSE(w, initial)
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
				writeSSE(w, msg)
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

func writeSSE(w *bufio.Writer, data []byte) {
	_, _ = w.WriteString("data: ")
	_, _ = w.Write(data)
	_, _ = w.WriteString("\n\n")
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

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}
