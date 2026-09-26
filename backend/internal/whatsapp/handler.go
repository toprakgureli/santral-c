package whatsapp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Handler serves the WhatsApp endpoints.
type Handler struct {
	s *Service
}

// NewHandler builds the handler.
func NewHandler(s *Service) *Handler { return &Handler{s: s} }

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}

func pid(c *fiber.Ctx, name string) (uint, error) {
	n, err := c.ParamsInt(name)
	if err != nil || n < 1 {
		return 0, errs.Invalid("Geçersiz kimlik.", err)
	}
	return uint(n), nil
}

func qid(c *fiber.Ctx, name string) uint {
	n, _ := strconv.Atoi(c.Query(name))
	if n < 0 {
		return 0
	}
	return uint(n)
}

func body(c *fiber.Ctx, v any) error {
	if err := c.BodyParser(v); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	return nil
}

// with runs fn with the actor and writes its result as JSON (or 204).
func with(fn func(c *fiber.Ctx, uid uint) (any, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid, err := actor(c)
		if err != nil {
			return err
		}
		out, err := fn(c, uid)
		if err != nil {
			return err
		}
		if out == nil {
			return c.SendStatus(fiber.StatusNoContent)
		}
		return c.JSON(out)
	}
}

// withID also reads the :id parameter.
func withID(fn func(c *fiber.Ctx, uid, id uint) (any, error)) fiber.Handler {
	return with(func(c *fiber.Ctx, uid uint) (any, error) {
		id, err := pid(c, "id")
		if err != nil {
			return nil, err
		}
		return fn(c, uid, id)
	})
}

// ---------------------------------------------------------------- public

// Verify answers Meta's webhook check.
func (h *Handler) Verify(c *fiber.Ctx) error {
	out, err := h.s.Verify(c.UserContext(), c.Params("key"), c.Query("hub.mode"), c.Query("hub.verify_token"), c.Query("hub.challenge"))
	if err != nil {
		return err
	}
	c.Set("Content-Type", "text/plain")
	return c.SendString(out)
}

// Receive stores Meta's webhook call.
func (h *Handler) Receive(c *fiber.Ctx) error {
	raw := append([]byte(nil), c.Body()...)
	if err := h.s.Receive(c.UserContext(), c.Params("key"), c.Get("X-Hub-Signature-256"), raw); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

// Tally receives a survey answer.
func (h *Handler) Tally(c *fiber.Ctx) error {
	raw := append([]byte(nil), c.Body()...)
	if err := h.s.TallyWebhook(c.UserContext(), c.Params("key"), c.Get("Tally-Signature"), raw); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

// Stream is the live stream for people who use WhatsApp but not the chat;
// chat users get the same events on the chat's stream.
func (h *Handler) Stream(c *fiber.Ctx) error {
	uid, err := actor(c)
	if err != nil {
		return err
	}
	if _, err := h.s.viewerOf(c.UserContext(), uid); err != nil {
		return err
	}
	ch, stop := h.s.push.SubscribeRaw(uid)
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer stop()
		if _, err := w.WriteString("data: {\"type\":\"hello\"}\n\n"); err != nil {
			return
		}
		if w.Flush() != nil {
			return
		}
		beat := time.NewTicker(20 * time.Second)
		defer beat.Stop()
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
			case <-beat.C:
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

// ---------------------------------------------------------------- media

// Media streams a message's file.
func (h *Handler) Media(c *fiber.Ctx) error {
	uid, err := actor(c)
	if err != nil {
		return err
	}
	id, err := pid(c, "id")
	if err != nil {
		return err
	}
	m, err := h.s.OpenMedia(c.UserContext(), uid, id, c.Get("Range"))
	if err != nil {
		return err
	}
	c.Set("Content-Type", m.Mime)
	c.Set("Cache-Control", "private, max-age=86400")
	c.Set("Accept-Ranges", "bytes")
	disp := "inline"
	if c.Query("download") == "1" {
		disp = "attachment"
	}
	if m.Name != "" {
		c.Set("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disp, pathEscape(m.Name)))
	}
	if m.ContentRange != "" {
		c.Set("Content-Range", m.ContentRange)
	}
	c.Status(m.Status)
	size := int(m.Size)
	if size <= 0 {
		size = -1
	}
	body := m.Body
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer body.Close()
		_, _ = io.Copy(w, body)
		_ = w.Flush()
	})
	if size > 0 {
		c.Context().Response.Header.SetContentLength(size)
	}
	return nil
}

func pathEscape(s string) string {
	const hex = "0123456789ABCDEF"
	out := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '.' || b == '-' || b == '_' {
			out = append(out, b)
			continue
		}
		out = append(out, '%', hex[b>>4], hex[b&15])
	}
	return string(out)
}

// SendMedia receives a file from the inbox.
func (h *Handler) SendMedia(c *fiber.Ctx, uid, id uint) (any, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, errs.Invalid("Dosya gelmedi.", err)
	}
	f, err := fh.Open()
	if err != nil {
		return nil, errs.Invalid("Dosya okunamadı.", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, mediaLimit+1))
	if err != nil {
		return nil, errs.Invalid("Dosya okunamadı.", err)
	}
	if len(data) > mediaLimit {
		return nil, errs.Invalid("Dosya en fazla 100 MB olabilir.", nil)
	}
	replyTo, _ := strconv.Atoi(c.FormValue("replyTo"))
	return h.s.SendMedia(c.UserContext(), uid, id, fh.Filename, fh.Header.Get("Content-Type"), c.FormValue("caption"), c.FormValue("clientId"), uint(replyTo), data)
}

// TemplateMedia receives a template header example.
func (h *Handler) TemplateMedia(c *fiber.Ctx, uid uint) (any, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, errs.Invalid("Dosya gelmedi.", err)
	}
	f, err := fh.Open()
	if err != nil {
		return nil, errs.Invalid("Dosya okunamadı.", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16<<20))
	if err != nil {
		return nil, errs.Invalid("Dosya okunamadı.", err)
	}
	ch, _ := strconv.Atoi(c.FormValue("channelId"))
	handle, err := h.s.UploadTemplateMedia(c.UserContext(), uid, uint(ch), fh.Header.Get("Content-Type"), data)
	if err != nil {
		return nil, err
	}
	return fiber.Map{"handle": handle}, nil
}

// ---------------------------------------------------------------- routes

// Router mounts the WhatsApp endpoints.
type Router struct {
	h     *Handler
	guard fiber.Handler
}

// NewRouter builds the router.
func NewRouter(h *Handler, guard fiber.Handler) *Router { return &Router{h: h, guard: guard} }

// Routes registers everything under /wa.
func (r *Router) Routes(g fiber.Router) {
	h, s := r.h, r.h.s
	// Meta and Tally call these without a panel session; they are
	// checked by signature instead.
	g.Get("/wa/hook/:key", h.Verify)
	g.Post("/wa/hook/:key", h.Receive)
	g.Post("/wa/survey/:key", h.Tally)

	a := g.Group("/wa", r.guard)
	a.Get("/stream", h.Stream)

	a.Get("/conversations", with(func(c *fiber.Ctx, uid uint) (any, error) {
		since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
		return s.Conversations(c.UserContext(), uid, since)
	}))
	a.Get("/conversations/resolved", with(func(c *fiber.Ctx, uid uint) (any, error) {
		before := time.Now().Add(time.Minute)
		if b := c.Query("before"); b != "" {
			if t, err := time.Parse(time.RFC3339Nano, b); err == nil {
				before = t
			}
		}
		return s.ResolvedPage(c.UserContext(), uid, before, c.Query("q"))
	}))
	a.Get("/conversations/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return s.Conversation(c.UserContext(), uid, id) }))
	a.Get("/conversations/:id/messages", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		return s.Messages(c.UserContext(), uid, id, qid(c, "before"), qid(c, "after"), qid(c, "around"))
	}))
	a.Post("/conversations/:id/messages", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in SendInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.Send(c.UserContext(), uid, id, in)
	}))
	a.Post("/conversations/:id/media", withID(h.SendMedia))
	a.Post("/conversations/:id/notes", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in struct {
			Body string `json:"body"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.Note(c.UserContext(), uid, id, in.Body)
	}))
	a.Post("/conversations/:id/read", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in struct {
			MessageID uint `json:"messageId"`
		}
		_ = c.BodyParser(&in)
		return nil, s.MarkRead(c.UserContext(), uid, id, in.MessageID)
	}))
	a.Post("/conversations/:id/unread", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.MarkUnread(c.UserContext(), uid, id) }))
	a.Post("/conversations/:id/typing", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.Typing(c.UserContext(), uid, id) }))
	a.Get("/conversations/:id/reads", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return s.Reads(c.UserContext(), uid, id) }))
	a.Post("/conversations/:id/greet", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.Greet(c.UserContext(), uid, id) }))
	a.Post("/conversations/:id/take", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.Take(c.UserContext(), uid, id) }))
	a.Post("/conversations/:id/assign", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in AssignInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.Assign(c.UserContext(), uid, id, in)
	}))
	a.Post("/conversations/:id/resolve", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.Resolve(c.UserContext(), uid, id) }))
	a.Post("/conversations/:id/reopen", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.Reopen(c.UserContext(), uid, id) }))
	a.Patch("/conversations/:id/ticket", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in TicketInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.UpdateTicket(c.UserContext(), uid, id, in)
	}))
	a.Post("/messages/:id/retry", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.Retry(c.UserContext(), uid, id) }))
	a.Get("/media/:id", h.Media)
	a.Get("/search", with(func(c *fiber.Ctx, uid uint) (any, error) {
		return s.Search(c.UserContext(), uid, c.Query("q"), qid(c, "conversation"))
	}))
	a.Get("/lookup", with(func(c *fiber.Ctx, uid uint) (any, error) {
		return s.LookupNumber(c.UserContext(), uid, c.Query("number"))
	}))
	a.Post("/start", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in struct {
			ChannelID uint   `json:"channelId"`
			Number    string `json:"number"`
			Name      string `json:"name"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.StartConversation(c.UserContext(), uid, in.ChannelID, in.Number, in.Name)
	}))
	a.Get("/contacts/:id/history", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return s.ContactHistory(c.UserContext(), uid, id) }))
	a.Patch("/contacts/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in ContactInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.UpdateContact(c.UserContext(), uid, id, in)
	}))

	// devices
	a.Get("/channels", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Channels(c.UserContext(), uid) }))
	a.Post("/channels", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in ChannelInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.CreateChannel(c.UserContext(), uid, in)
	}))
	a.Patch("/channels/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in ChannelInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.UpdateChannel(c.UserContext(), uid, id, in)
	}))
	a.Delete("/channels/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.DeleteChannel(c.UserContext(), uid, id) }))
	a.Post("/channels/:id/test", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return s.TestChannel(c.UserContext(), uid, id) }))
	a.Post("/channels/:id/subscribe", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		return nil, s.SubscribeChannel(c.UserContext(), uid, id)
	}))
	a.Put("/channels/:id/settings", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in SettingsInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.UpdateSettings(c.UserContext(), uid, id, in)
	}))
	a.Post("/channels/:id/copy-settings", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in struct {
			From     uint     `json:"from"`
			Sections []string `json:"sections"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.CopySettings(c.UserContext(), uid, id, in.From, in.Sections)
	}))
	a.Put("/channels/:id/members", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in struct {
			UserIDs []uint `json:"userIds"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.SetMembers(c.UserContext(), uid, id, in.UserIDs)
	}))
	a.Post("/channels/copy", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in struct {
			From uint   `json:"from"`
			To   uint   `json:"to"`
			What string `json:"what"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		n, err := s.CopyToChannel(c.UserContext(), uid, in.From, in.To, in.What)
		if err != nil {
			return nil, err
		}
		return fiber.Map{"copied": n}, nil
	}))

	// templates
	a.Get("/templates", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Templates(c.UserContext(), uid, qid(c, "channel")) }))
	a.Post("/templates", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in TemplateInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.CreateTemplate(c.UserContext(), uid, in)
	}))
	a.Post("/templates/sync", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in struct {
			ChannelID uint `json:"channelId"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		n, err := s.SyncTemplates(c.UserContext(), uid, in.ChannelID)
		if err != nil {
			return nil, err
		}
		return fiber.Map{"count": n}, nil
	}))
	a.Post("/templates/media", with(h.TemplateMedia))
	a.Delete("/templates/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.DeleteTemplate(c.UserContext(), uid, id) }))

	// teams, people, quick replies
	a.Get("/teams", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Teams(c.UserContext(), uid) }))
	a.Post("/teams", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in TeamInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.SaveTeam(c.UserContext(), uid, 0, in)
	}))
	a.Put("/teams/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in TeamInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.SaveTeam(c.UserContext(), uid, id, in)
	}))
	a.Delete("/teams/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.DeleteTeam(c.UserContext(), uid, id) }))
	a.Get("/agents", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Agents(c.UserContext(), uid) }))
	a.Get("/quick-replies", with(func(c *fiber.Ctx, uid uint) (any, error) {
		return s.QuickReplies(c.UserContext(), uid, qid(c, "channel"))
	}))
	a.Post("/quick-replies", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in QuickReplyView
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.SaveQuickReply(c.UserContext(), uid, 0, in)
	}))
	a.Put("/quick-replies/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in QuickReplyView
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.SaveQuickReply(c.UserContext(), uid, id, in)
	}))
	a.Delete("/quick-replies/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		return nil, s.DeleteQuickReply(c.UserContext(), uid, id)
	}))

	// automatic messages
	a.Get("/rules", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Rules(c.UserContext(), uid) }))
	a.Post("/rules", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in RuleInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.SaveRule(c.UserContext(), uid, 0, in)
	}))
	a.Put("/rules/order", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in struct {
			IDs []uint `json:"ids"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.ReorderRules(c.UserContext(), uid, in.IDs)
	}))
	a.Put("/rules/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in RuleInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.SaveRule(c.UserContext(), uid, id, in)
	}))
	a.Delete("/rules/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.DeleteRule(c.UserContext(), uid, id) }))

	// chatbots
	a.Get("/bots", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Bots(c.UserContext(), uid) }))
	a.Post("/bots", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in BotInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.CreateBot(c.UserContext(), uid, in)
	}))
	a.Post("/bots/simulate", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in SimInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.Simulate(c.UserContext(), uid, in)
	}))
	a.Get("/bots/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return s.Bot(c.UserContext(), uid, id) }))
	a.Put("/bots/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in BotInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.UpdateBot(c.UserContext(), uid, id, in)
	}))
	a.Put("/bots/:id/draft", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in BotGraph
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.SaveDraft(c.UserContext(), uid, id, in)
	}))
	a.Post("/bots/:id/publish", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return s.PublishBot(c.UserContext(), uid, id) }))
	a.Get("/bots/:id/versions", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return s.BotVersions(c.UserContext(), uid, id) }))
	a.Post("/bots/:id/restore", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in struct {
			Version int `json:"version"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.RestoreVersion(c.UserContext(), uid, id, in.Version)
	}))
	a.Post("/bots/:id/copy", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in struct {
			Name       string `json:"name"`
			ChannelIDs []uint `json:"channelIds"`
		}
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return s.CopyBot(c.UserContext(), uid, id, in.Name, in.ChannelIDs)
	}))
	a.Delete("/bots/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.DeleteBot(c.UserContext(), uid, id) }))
	a.Get("/bots/:id/report", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		days, _ := strconv.Atoi(c.Query("days"))
		return s.BotReport(c.UserContext(), uid, id, days)
	}))

	// outside systems
	a.Get("/integrations", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Integrations(c.UserContext(), uid) }))
	a.Post("/integrations", with(func(c *fiber.Ctx, uid uint) (any, error) {
		var in IntegrationInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.SaveIntegration(c.UserContext(), uid, 0, in)
	}))
	a.Put("/integrations/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in IntegrationInput
		if err := body(c, &in); err != nil {
			return nil, err
		}
		return nil, s.SaveIntegration(c.UserContext(), uid, id, in)
	}))
	a.Delete("/integrations/:id", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		return nil, s.DeleteIntegration(c.UserContext(), uid, id)
	}))
	a.Post("/integrations/:id/test", withID(func(c *fiber.Ctx, uid, id uint) (any, error) {
		var in map[string]string
		_ = c.BodyParser(&in)
		return s.TestIntegration(c.UserContext(), uid, id, in)
	}))

	// callbacks, events, reports
	a.Get("/callbacks", with(func(c *fiber.Ctx, uid uint) (any, error) {
		return s.Callbacks(c.UserContext(), uid, c.Query("all") == "1")
	}))
	a.Post("/callbacks/:id/done", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.DoneCallback(c.UserContext(), uid, id) }))
	a.Get("/events", with(func(c *fiber.Ctx, uid uint) (any, error) { return s.Events(c.UserContext(), uid) }))
	a.Post("/events/:id/retry", withID(func(c *fiber.Ctx, uid, id uint) (any, error) { return nil, s.RetryEvent(c.UserContext(), uid, id) }))
	a.Get("/reports", with(func(c *fiber.Ctx, uid uint) (any, error) {
		return s.Reports(c.UserContext(), uid, c.Query("from"), c.Query("to"), qid(c, "channel"))
	}))
}
