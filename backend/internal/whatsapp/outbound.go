package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/meta"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/store"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// windowOpen reports whether a free-form message may be sent: the
// customer wrote within the last 24 hours.
func windowOpen(conv *models.WAConversation) bool {
	return conv.LastInboundAt != nil && time.Since(*conv.LastInboundAt) < 24*time.Hour
}

// ---------------------------------------------------------------- agent sends

// TemplateParams fills a template's variables.
type TemplateParams struct {
	Header      []string `json:"header"`
	Body        []string `json:"body"`
	Buttons     []string `json:"buttons"`
	HeaderMedia string   `json:"headerMedia"` // a link for an image/video/document header
	HeaderFile  uint     `json:"headerFile"`  // or a file uploaded from the panel
	// filled by the server: Meta's id for HeaderFile, and quick reply payloads
	headerMediaID string
	quickPayloads []string
}

// SendInput is an agent's message from the inbox.
type SendInput struct {
	ClientID   string          `json:"clientId"`
	Kind       string          `json:"kind"` // text | template | reaction
	Body       string          `json:"body"`
	ReplyTo    uint            `json:"replyTo"`
	TemplateID uint            `json:"templateId"`
	Params     *TemplateParams `json:"params"`
	TargetID   uint            `json:"targetId"`
	Emoji      string          `json:"emoji"`
}

// reachable loads a conversation for a person and checks they see it.
func (s *Service) reachable(ctx context.Context, actorID, conversationID uint) (*viewer, *models.WAConversation, *models.WATicket, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, nil, nil, err
	}
	conv, ticket, err := s.repo.Conversation(ctx, conversationID)
	if err != nil {
		return nil, nil, nil, errs.NotFound("Sohbet bulunamadı.")
	}
	if ticket == nil || !v.seesTicket(ticket, s.repo.Participants(ctx, ticket.ID)) {
		return nil, nil, nil, errs.Forbidden("Bu sohbeti görme yetkin yok.")
	}
	return v, conv, ticket, nil
}

// Send queues an agent's message. It returns right away; the ticks follow
// on the live stream.
func (s *Service) Send(ctx context.Context, actorID, conversationID uint, in SendInput) (*MessageView, error) {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return nil, err
	}
	if !v.can(enums.WAReply) {
		return nil, errs.Forbidden("Müşteriye yazma yetkin yok.")
	}
	ch, err := s.repo.Channel(ctx, conv.ChannelID)
	if err != nil {
		return nil, err
	}
	contact, err := s.repo.Contact(ctx, conv.ContactID)
	if err != nil {
		return nil, err
	}
	if contact.Blocked {
		return nil, errs.Invalid("Bu müşteri engellenmiş.", nil)
	}
	msg := &models.WAMessage{
		ChannelID: ch.ID, ConversationID: conv.ID, TicketID: uintPtr(ticket.ID), Direction: "out",
		SenderKind: "agent", SenderUserID: uintPtr(actorID), Status: "queued", CreatedAt: time.Now(),
	}
	if cid := strings.TrimSpace(in.ClientID); cid != "" {
		msg.ClientID = strPtr(cid)
	}
	var payload map[string]any
	switch in.Kind {
	case "", "text":
		body := strings.TrimSpace(in.Body)
		if body == "" {
			return nil, errs.Invalid("Mesaj boş olamaz.", nil)
		}
		if len([]rune(body)) > 4096 {
			return nil, errs.Invalid("Mesaj en fazla 4096 karakter olabilir.", nil)
		}
		if !windowOpen(conv) {
			return nil, errs.Invalid("Müşterinin son mesajının üzerinden 24 saat geçti. Şablonla yazman gerekiyor.", nil)
		}
		msg.Kind, msg.Body = "text", body
		payload = map[string]any{"type": "text", "text": map[string]any{"body": body, "preview_url": strings.Contains(body, "http")}}
	case "template":
		if !v.can(enums.WATemplateSend) {
			return nil, errs.Forbidden("Şablonla mesaj gönderme yetkin yok.")
		}
		tpl, err := s.repo.Template(ctx, in.TemplateID)
		if err != nil {
			return nil, err
		}
		if tpl.WABAID != ch.WABAID {
			return nil, errs.Invalid("Bu şablon bu cihazın işletme hesabına ait değil.", nil)
		}
		if tpl.Status != "APPROVED" {
			return nil, errs.Invalid("Yalnızca Meta'nın onayladığı şablonlar gönderilebilir.", nil)
		}
		if tpl.Category == "MARKETING" && contact.OptedOut {
			return nil, errs.Invalid("Müşteri kampanya mesajlarını kapatmış. Pazarlama şablonu gönderilemez.", nil)
		}
		params := TemplateParams{}
		if in.Params != nil {
			params = *in.Params
		}
		// Blanks left empty are filled as the template says: the customer's
		// name, or the name of the person sending.
		if fill := parseFill(tpl.Fill); len(fill) > 0 {
			sender, _ := s.users.GetByID(ctx, actorID)
			for i, f := range fill {
				for len(params.Body) <= i {
					params.Body = append(params.Body, "")
				}
				if strings.TrimSpace(params.Body[i]) != "" {
					continue
				}
				switch f {
				case "customer":
					params.Body[i] = oneLine(firstName(contactView(contact).Display))
				case "agent":
					if sender != nil {
						params.Body[i] = oneLine(firstName(sender.Name))
					}
				case "agent_full":
					if sender != nil {
						params.Body[i] = oneLine(sender.Name)
					}
				}
			}
		}
		if params.HeaderFile > 0 {
			// Only a file the person may open goes to a customer.
			if _, err := s.usableFile(ctx, v.user, params.HeaderFile); err != nil {
				return nil, err
			}
			id, _, err := s.metaMediaFor(ctx, ch, params.HeaderFile)
			if err != nil {
				return nil, errs.Invalid("Başlık dosyası Meta'ya yüklenemedi. "+meta.Friendly(err), err)
			}
			params.headerMediaID = id
		}
		obj, preview, err := buildTemplate(tpl, params)
		if err != nil {
			return nil, err
		}
		msg.Kind, msg.Body, msg.SenderLabel = "template", preview, tpl.Name
		payload = map[string]any{"type": "template", "template": obj}
	case "reaction":
		target, err := s.repo.MessageInConversation(ctx, in.TargetID, conv.ID)
		if err != nil || target.WAMID == nil {
			return nil, errs.NotFound("Tepki verilecek mesaj bulunamadı.")
		}
		msg.Kind, msg.Body = "reaction", in.Emoji
		payload = map[string]any{"type": "reaction", "reaction": map[string]any{"message_id": *target.WAMID, "emoji": in.Emoji}}
	default:
		return nil, errs.Invalid("Mesaj türü tanınmadı.", nil)
	}
	if in.ReplyTo > 0 && msg.Kind != "reaction" {
		if target, err := s.repo.MessageInConversation(ctx, in.ReplyTo, conv.ID); err == nil && target.WAMID != nil {
			msg.ReplyToWAMID = target.WAMID
		}
	}
	return s.enqueue(ctx, ch, conv, ticket, msg, payload, actorID)
}

// enqueue stores an outgoing message and applies what an agent's answer
// means for the ticket.
func (s *Service) enqueue(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, msg *models.WAMessage, payload map[string]any, actorID uint) (*MessageView, error) {
	msg.Payload = strPtr(jsonString(payload))
	nt := time.Now()
	msg.NextTryAt = &nt
	before := s.audience(ctx, ticket)
	err := s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		if err := store.CreateMessage(tx, msg); err != nil {
			return err
		}
		if msg.Kind == "reaction" {
			return nil
		}
		if err := store.SetLastMessage(tx, conv.ID, msg.ID); err != nil {
			return err
		}
		if msg.SenderKind == "agent" && actorID > 0 {
			return markAnswered(tx, ticket, actorID, msg.Kind == "template")
		}
		return nil
	})
	if err != nil {
		if msg.ClientID != nil && strings.Contains(err.Error(), "client_id") {
			if existing, err := s.repo.MessageByClientID(ctx, *msg.ClientID); err == nil {
				views, _ := s.messageViews(ctx, []models.WAMessage{*existing})
				if len(views) > 0 {
					return &views[0], nil
				}
			}
		}
		return nil, errs.Internal(err)
	}
	if msg.SenderKind == "agent" && ticket.Status == "bot" {
		s.endBot(ctx, conv.ID, "handoff")
	}
	wake(s.wakeOutbox)
	s.publish(ctx, conv.ID, msg, before)
	views, err := s.messageViews(ctx, []models.WAMessage{*msg})
	if err != nil || len(views) == 0 {
		return nil, errs.Internal(err)
	}
	return &views[0], nil
}

// markAnswered records that a person answered: the wait is over, the
// person joins the ticket, and a template brings a resolved ticket back.
// It works on the locked current row, so two people answering an unowned
// ticket at once do not both become its owner.
func markAnswered(tx *gorm.DB, stale *models.WATicket, userID uint, template bool) error {
	ticket, err := store.LockTicket(tx, stale.ID)
	if err != nil {
		return err
	}
	if ticket.WaitingListedAt != nil && ticket.AwaitingSince != nil {
		wait := int(time.Since(*ticket.AwaitingSince).Seconds())
		if wait > ticket.LongestWaitSec {
			if err := store.SetLongestWait(tx, ticket.ID, wait); err != nil {
				return err
			}
		}
	}
	status := ticket.Status
	reopen := false
	if status == "bot" {
		status = "open"
	}
	if status == "resolved" && template {
		status = "open"
		reopen = true
	}
	owner := ticket.OwnerID
	if owner == nil {
		owner = uintPtr(userID)
	}
	if err := store.MarkTicketAnswered(tx, ticket.ID, status, *owner, reopen); err != nil {
		return err
	}
	role := "helper"
	if *owner == userID {
		role = "owner"
	}
	if err := store.RecordFirstReply(tx, ticket.ID, userID, role); err != nil {
		return err
	}
	if ticket.OwnerID == nil {
		if err := store.RecordClaim(tx, ticket.ID, userID); err != nil {
			return err
		}
	}
	return nil
}

// queueSystem sends a message on behalf of the device (chatbot, rule),
// which does not count as a person's answer.
func (s *Service) queueSystem(ctx context.Context, ch *models.WAChannel, conversationID, ticketID uint, kind, label, body string) {
	s.queueObject(ctx, ch, conversationID, ticketID, kind, label, "text", body, map[string]any{"type": "text", "text": map[string]any{"body": body}})
}

// queueObject stores any prepared message from the device.
func (s *Service) queueObject(ctx context.Context, ch *models.WAChannel, conversationID, ticketID uint, senderKind, label, kind, body string, payload map[string]any) {
	conv, ticket, err := s.repo.Conversation(ctx, conversationID)
	if err != nil {
		return
	}
	if kind != "template" && !windowOpen(conv) {
		return
	}
	msg := &models.WAMessage{
		ChannelID: ch.ID, ConversationID: conversationID, Direction: "out", Kind: kind, SenderKind: senderKind,
		SenderLabel: label, Body: body, Status: "queued", CreatedAt: time.Now(),
	}
	if ticketID > 0 {
		msg.TicketID = uintPtr(ticketID)
	}
	if ticket == nil {
		ticket = &models.WATicket{}
	}
	if _, err := s.enqueue(ctx, ch, conv, ticket, msg, payload, 0); err != nil {
		slog.WarnContext(ctx, "whatsapp system message could not be queued", "conversation", conversationID, "error", err)
	}
}

// ---------------------------------------------------------------- the queue

func (s *Service) outboxWorker(ctx context.Context) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	// Only this loop sends, and only one server runs, so a message still
	// 'sending' when it starts was cut off before: it is flagged at once
	// instead of holding its conversation back for minutes.
	s.flagStaleSends(ctx, 0)
	recovered := time.Now()
	for {
		if time.Since(recovered) >= time.Minute {
			s.flagStaleSends(ctx, staleSend)
			recovered = time.Now()
		}
		for s.sendBatch(ctx) {
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-s.wakeOutbox:
		}
	}
}

// staleSend is how long a message may stay 'sending'. A send takes at most
// 40 seconds; one older than this was cut off by a stop or a crash.
const staleSend = 5 * time.Minute

// sendBatch takes due messages off the queue and sends them. Taking marks
// them 'sending' in the same statement, with rows another worker holds
// skipped, so a message is never taken twice. A conversation's messages go
// out in the order they were written: a later one waits while an earlier
// one is being sent or retried.
func (s *Service) sendBatch(ctx context.Context) bool {
	list, err := s.repo.TakeDueMessages(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "whatsapp outbox could not be read", "error", err)
		return false
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	for i := range list {
		msg := &list[i]
		if ctx.Err() != nil {
			// Stopping: what was taken but not started goes back to the
			// queue untouched, so the next start sends it.
			s.requeue(list[i:])
			return false
		}
		// A send that started is finished even while the server stops, so
		// its outcome is known and it is never sent twice.
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
		err := safe.Call(func() error {
			s.sendOne(sendCtx, msg)
			return nil
		})
		cancel()
		if err != nil {
			slog.ErrorContext(ctx, "whatsapp send panicked", "message", msg.ID, "error", err)
			n, err := s.repo.SendBroke(ctx, msg.ID, "Gönderilirken beklenmeyen bir hata oldu.")
			s.finishSend(ctx, msg.ID, n, err)
		}
	}
	// Anything taken means the next message of the same conversation may
	// now be due, so the caller asks again at once.
	return len(list) > 0
}

// sendTimeout bounds one send, Meta included.
const sendTimeout = 45 * time.Second

// requeue puts messages taken for sending back into the queue.
func (s *Service) requeue(list []models.WAMessage) {
	ids := make([]uint, 0, len(list))
	for i := range list {
		ids = append(ids, list[i].ID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	warnDB(ctx, s.repo.RequeueSending(ctx, ids))
}

// finishSend reports how writing the outcome of a send went. The outcome
// only touches a message that is still 'sending', so it never overwrites a
// later state; changed is how many messages it changed.
func (s *Service) finishSend(ctx context.Context, id uint, changed int64, err error) {
	if err != nil {
		slog.ErrorContext(ctx, "whatsapp send outcome could not be stored", "message", id, "error", err)
		return
	}
	if changed == 0 {
		slog.WarnContext(ctx, "whatsapp send outcome arrived after the message left 'sending'", "message", id)
	}
}

// flagStaleSends marks messages cut off in the middle of a send as failed.
// Whether Meta got them is unknown, so they are not sent again by
// themselves; the agent sees the note and decides.
func (s *Service) flagStaleSends(ctx context.Context, age time.Duration) {
	stale, err := s.repo.FailStaleSends(ctx,
		"Gönderilip gönderilmediği anlaşılamadı. Müşteriye ulaşıp ulaşmadığını kontrol et, gerekirse tekrar gönder.",
		time.Now().Add(-age))
	if err != nil {
		slog.ErrorContext(ctx, "whatsapp stale sends could not be checked", "error", err)
		return
	}
	for i := range stale {
		slog.WarnContext(ctx, "whatsapp message was cut off while sending", "message", stale[i].ID)
		s.publish(ctx, stale[i].ConversationID, &stale[i], nil)
	}
}

func (s *Service) sendOne(ctx context.Context, msg *models.WAMessage) {
	fail := func(code int, text string) {
		n, err := s.repo.SendFailed(ctx, msg.ID, code, text)
		s.finishSend(ctx, msg.ID, n, err)
		if err := s.repo.ReloadMessage(ctx, msg); err != nil {
			slog.WarnContext(ctx, "whatsapp failed message could not be reloaded", "message", msg.ID, "error", err)
			return
		}
		s.publish(ctx, msg.ConversationID, msg, nil)
	}
	ch, err := s.repo.Channel(ctx, msg.ChannelID)
	if err != nil {
		fail(0, "Cihaz bulunamadı.")
		return
	}
	if !ch.Active {
		fail(0, "Cihaz kapalı olduğu için gönderilmedi.")
		return
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		fail(0, err.Error())
		return
	}
	contact, err := s.repo.ContactOfConversation(ctx, msg.ConversationID)
	if err != nil {
		fail(0, "Müşteri bulunamadı.")
		return
	}
	var payload map[string]any
	if msg.Payload == nil || json.Unmarshal([]byte(*msg.Payload), &payload) != nil || payload["type"] == nil {
		fail(0, "Mesaj içeriği okunamadı.")
		return
	}
	if msg.ReplyToWAMID != nil && msg.Kind != "reaction" {
		payload["context"] = map[string]any{"message_id": *msg.ReplyToWAMID}
	}
	c, cancel := context.WithTimeout(ctx, 40*time.Second)
	wamid, err := cl.Send(c, contact.WAID, payload)
	cancel()
	if err != nil {
		attempts := msg.Attempts + 1
		if meta.Retryable(err) && attempts < 6 {
			next := time.Now().Add(time.Duration(10*(1<<attempts)) * time.Second)
			n, ferr := s.repo.SendLater(ctx, msg.ID, attempts, next, meta.Friendly(err))
			s.finishSend(ctx, msg.ID, n, ferr)
			return
		}
		code := 0
		var api *meta.APIError
		if errors.As(err, &api) {
			code = api.Code
			if api.Code == 190 {
				s.recordChannelError(ctx, ch.ID, meta.Describe(190, ""))
			}
		}
		fail(code, meta.Friendly(err))
		return
	}
	n, err := s.repo.SendDone(ctx, msg.ID, wamid)
	s.finishSend(ctx, msg.ID, n, err)
	if err := s.repo.ReloadMessage(ctx, msg); err != nil {
		slog.WarnContext(ctx, "whatsapp sent message could not be reloaded", "message", msg.ID, "error", err)
		return
	}
	s.applyPending(ctx, msg)
	s.publish(ctx, msg.ConversationID, msg, nil)
}

// Retry queues a failed message again.
func (s *Service) Retry(ctx context.Context, actorID, messageID uint) error {
	msg, err := s.repo.LoadMessage(ctx, messageID)
	if err != nil {
		return errs.NotFound("Mesaj bulunamadı.")
	}
	v, _, _, err := s.reachable(ctx, actorID, msg.ConversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAReply) {
		return errs.Forbidden("Müşteriye yazma yetkin yok.")
	}
	if msg.Status != "failed" || msg.Direction != "out" {
		return errs.Invalid("Yalnızca gönderilemeyen mesaj tekrar gönderilebilir.", nil)
	}
	changed, err := s.repo.RequeueMessage(ctx, msg.ID)
	if err != nil {
		return errs.Internal(err)
	}
	if changed == 0 {
		// Someone pressed retry a moment earlier; the message is on its way.
		return errs.Conflict("Bu mesaj zaten yeniden gönderiliyor.", nil)
	}
	warnDB(ctx, s.repo.ReloadMessage(ctx, msg))
	s.publish(ctx, msg.ConversationID, msg, nil)
	wake(s.wakeOutbox)
	return nil
}
