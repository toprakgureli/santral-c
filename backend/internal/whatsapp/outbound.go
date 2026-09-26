package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// windowOpen reports whether a free-form message may be sent: the
// customer wrote within the last 24 hours.
func windowOpen(conv *models.WAConversation) bool {
	return conv.LastInboundAt != nil && time.Since(*conv.LastInboundAt) < 24*time.Hour
}

// describeCode puts Meta's error codes into words an agent understands.
func describeCode(code int, fallback string) string {
	switch code {
	case 190:
		return "Cihazın erişim anahtarı geçersiz ya da süresi dolmuş. Yöneticinin cihaz ayarlarından yenilemesi gerekiyor."
	case 10, 200, 3:
		return "Erişim anahtarının bu işlem için izni yok."
	case 131047:
		return "Müşterinin son mesajının üzerinden 24 saat geçti. Artık yalnızca şablonla yazılabilir."
	case 131026:
		return "Mesaj müşteriye ulaşmadı. Numara WhatsApp kullanmıyor olabilir ya da uygulaması çok eski olabilir."
	case 131049:
		return "Meta bu pazarlama mesajını müşteriye iletmedi. Kısa süre içinde çok sayıda pazarlama mesajı almış olabilir."
	case 131050:
		return "Müşteri pazarlama mesajlarını kapatmış."
	case 131051:
		return "Bu mesaj türü desteklenmiyor."
	case 131052:
		return "Müşterinin gönderdiği dosya indirilemedi."
	case 131053:
		return "Dosya Meta'ya yüklenemedi. Biçimi ya da boyutu uygun olmayabilir."
	case 132000:
		return "Şablondaki değişken sayısı ile girilen değerler tutmuyor."
	case 132001:
		return "Şablon bulunamadı ya da bu dilde onaylı değil."
	case 132005:
		return "Şablon değişkenleri çok uzun."
	case 132007:
		return "Şablon metni Meta kurallarına uymuyor."
	case 132012:
		return "Şablon değişkenlerinin biçimi yanlış."
	case 132015:
		return "Şablon düşük kalite nedeniyle Meta tarafından duraklatıldı."
	case 132016:
		return "Şablon Meta tarafından kapatıldı."
	case 130429:
		return "Gönderim hızı sınırına takıldı. Biraz sonra tekrar denenecek."
	case 131056:
		return "Bu müşteriye çok kısa sürede çok fazla mesaj gönderildi. Biraz bekleyip tekrar deneyin."
	case 131048:
		return "Meta, numaranın gönderimlerini geçici olarak kısıtladı (spam şüphesi)."
	case 131042:
		return "Meta işletme hesabında ödeme sorunu var."
	case 131031, 368:
		return "Meta bu hesabı kurallar nedeniyle kısıtlamış."
	case 133010:
		return "Numara WhatsApp Business'a kayıtlı değil."
	case 131021:
		return "Gönderen ve alıcı aynı numara olamaz."
	case 100:
		if f := strings.TrimSpace(fallback); f != "" {
			return "Meta isteği anlamadı: " + f
		}
		return "Meta isteği anlamadı."
	}
	if f := strings.TrimSpace(fallback); f != "" {
		return f
	}
	return "Meta bir hata bildirdi."
}

// friendlyError turns any error from Meta into a sentence.
func friendlyError(err error) string {
	var api *APIError
	if errors.As(err, &api) {
		return describeCode(api.Code, api.Message+" "+api.Details)
	}
	return "Meta'ya ulaşılamadı, internet bağlantısı ya da Meta tarafında geçici bir sorun olabilir."
}

func retryable(err error) bool {
	var api *APIError
	if !errors.As(err, &api) {
		return true
	}
	if api.Status >= 500 {
		return true
	}
	switch api.Code {
	case 1, 2, 4, 80007, 130429, 131000, 131016, 133004, 131056:
		return true
	}
	return false
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
	conv, ticket, err := s.loadConv(ctx, conversationID)
	if err != nil {
		return nil, nil, nil, errs.NotFound("Sohbet bulunamadı.")
	}
	if ticket == nil || !v.seesTicket(ticket, s.participantSet(ctx, ticket.ID)) {
		return nil, nil, nil, errs.Forbidden("Bu sohbeti görme yetkiniz yok.")
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
		return nil, errs.Forbidden("Müşteriye yazma yetkiniz yok.")
	}
	ch, err := s.channel(ctx, conv.ChannelID)
	if err != nil {
		return nil, err
	}
	contact, err := s.contact(ctx, conv.ContactID)
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
	var meta map[string]any
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
			return nil, errs.Invalid("Müşterinin son mesajının üzerinden 24 saat geçti. Şablonla yazmanız gerekiyor.", nil)
		}
		msg.Kind, msg.Body = "text", body
		meta = map[string]any{"type": "text", "text": map[string]any{"body": body, "preview_url": strings.Contains(body, "http")}}
	case "template":
		if !v.can(enums.WATemplateSend) {
			return nil, errs.Forbidden("Şablonla mesaj gönderme yetkiniz yok.")
		}
		tpl, err := s.template(ctx, in.TemplateID)
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
					params.Body[i] = firstName(contactView(contact).Display)
				case "agent":
					if sender != nil {
						params.Body[i] = firstName(sender.Name)
					}
				case "agent_full":
					if sender != nil {
						params.Body[i] = sender.Name
					}
				}
			}
		}
		if params.HeaderFile > 0 {
			id, _, err := s.metaMediaFor(ctx, ch, params.HeaderFile)
			if err != nil {
				return nil, errs.Invalid("Başlık dosyası Meta'ya yüklenemedi. "+friendlyError(err), err)
			}
			params.headerMediaID = id
		}
		obj, preview, err := buildTemplate(tpl, params)
		if err != nil {
			return nil, err
		}
		msg.Kind, msg.Body, msg.SenderLabel = "template", preview, tpl.Name
		meta = map[string]any{"type": "template", "template": obj}
	case "reaction":
		var target models.WAMessage
		if err := s.db.WithContext(ctx).Where("id = ? AND conversation_id = ?", in.TargetID, conv.ID).First(&target).Error; err != nil || target.WAMID == nil {
			return nil, errs.NotFound("Tepki verilecek mesaj bulunamadı.")
		}
		msg.Kind, msg.Body = "reaction", in.Emoji
		meta = map[string]any{"type": "reaction", "reaction": map[string]any{"message_id": *target.WAMID, "emoji": in.Emoji}}
	default:
		return nil, errs.Invalid("Mesaj türü tanınmadı.", nil)
	}
	if in.ReplyTo > 0 && msg.Kind != "reaction" {
		var target models.WAMessage
		if err := s.db.WithContext(ctx).Where("id = ? AND conversation_id = ?", in.ReplyTo, conv.ID).First(&target).Error; err == nil && target.WAMID != nil {
			msg.ReplyToWAMID = target.WAMID
		}
	}
	return s.enqueue(ctx, ch, conv, ticket, msg, meta, actorID)
}

// enqueue stores an outgoing message and applies what an agent's answer
// means for the ticket.
func (s *Service) enqueue(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, msg *models.WAMessage, meta map[string]any, actorID uint) (*MessageView, error) {
	msg.Payload = strPtr(jsonString(meta))
	nt := time.Now()
	msg.NextTryAt = &nt
	before := s.audience(ctx, ticket)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		if msg.Kind == "reaction" {
			return nil
		}
		if err := tx.Exec("UPDATE wa_conversations SET last_message_id = ?, last_message_at = now() WHERE id = ?", msg.ID, conv.ID).Error; err != nil {
			return err
		}
		if msg.SenderKind == "agent" && actorID > 0 {
			return markAnswered(tx, ticket, actorID, msg.Kind == "template")
		}
		return nil
	})
	if err != nil {
		if msg.ClientID != nil && strings.Contains(err.Error(), "client_id") {
			var existing models.WAMessage
			if s.db.WithContext(ctx).Where("client_id = ?", *msg.ClientID).First(&existing).Error == nil {
				views, _ := s.messageViews(ctx, []models.WAMessage{existing})
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
func markAnswered(tx *gorm.DB, ticket *models.WATicket, userID uint, template bool) error {
	if ticket.WaitingListedAt != nil && ticket.AwaitingSince != nil {
		wait := int(time.Since(*ticket.AwaitingSince).Seconds())
		if wait > ticket.LongestWaitSec {
			if err := tx.Exec("UPDATE wa_tickets SET longest_wait_sec = ? WHERE id = ?", wait, ticket.ID).Error; err != nil {
				return err
			}
		}
	}
	status := ticket.Status
	reopen := ""
	if status == "bot" {
		status = "open"
	}
	if status == "resolved" && template {
		status = "open"
		reopen = ", reopen_count = reopen_count + 1, resolved_at = NULL"
	}
	owner := ticket.OwnerID
	if owner == nil {
		owner = uintPtr(userID)
	}
	if err := tx.Exec(`UPDATE wa_tickets SET awaiting_since = NULL, waiting_listed_at = NULL, status = ?, owner_id = ?,
		first_response_at = COALESCE(first_response_at, now()), updated_at = now()`+reopen+` WHERE id = ?`, status, *owner, ticket.ID).Error; err != nil {
		return err
	}
	role := "helper"
	if *owner == userID {
		role = "owner"
	}
	if err := tx.Exec(`INSERT INTO wa_ticket_participants (ticket_id, user_id, role, first_reply_at) VALUES (?, ?, ?, now())
		ON CONFLICT (ticket_id, user_id) DO UPDATE SET first_reply_at = COALESCE(wa_ticket_participants.first_reply_at, now())`, ticket.ID, userID, role).Error; err != nil {
		return err
	}
	if ticket.OwnerID == nil {
		if err := tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, to_user, by_user) VALUES (?, 'claim', ?, ?)", ticket.ID, userID, userID).Error; err != nil {
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
func (s *Service) queueObject(ctx context.Context, ch *models.WAChannel, conversationID, ticketID uint, senderKind, label, kind, body string, meta map[string]any) {
	conv, ticket, err := s.loadConv(ctx, conversationID)
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
	if _, err := s.enqueue(ctx, ch, conv, ticket, msg, meta, 0); err != nil {
		slog.WarnContext(ctx, "whatsapp system message could not be queued", "conversation", conversationID, "error", err)
	}
}

// ---------------------------------------------------------------- the queue

func (s *Service) outboxWorker(ctx context.Context) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
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

// sendBatch sends due messages. A conversation's messages go out in the
// order they were written: a later one waits while an earlier one retries.
func (s *Service) sendBatch(ctx context.Context) bool {
	var list []models.WAMessage
	err := s.db.WithContext(ctx).Raw(`SELECT m.* FROM wa_messages m
		WHERE m.status = 'queued' AND m.next_try_at <= now()
		  AND NOT EXISTS (SELECT 1 FROM wa_messages p WHERE p.conversation_id = m.conversation_id AND p.status = 'queued' AND p.id < m.id)
		ORDER BY m.id LIMIT 20`).Scan(&list).Error
	if err != nil {
		slog.ErrorContext(ctx, "whatsapp outbox could not be read", "error", err)
		return false
	}
	for i := range list {
		s.sendOne(ctx, &list[i])
	}
	return len(list) == 20
}

func (s *Service) sendOne(ctx context.Context, msg *models.WAMessage) {
	fail := func(code int, text string) {
		_ = s.db.WithContext(ctx).Exec("UPDATE wa_messages SET status = 'failed', failed_at = now(), error_code = ?, error_text = ?, attempts = attempts + 1 WHERE id = ?", code, text, msg.ID).Error
		_ = s.db.WithContext(ctx).First(msg, msg.ID).Error
		s.publish(ctx, msg.ConversationID, msg, nil)
	}
	ch, err := s.channel(ctx, msg.ChannelID)
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
	contact, err := s.contactOfConversation(ctx, msg.ConversationID)
	if err != nil {
		fail(0, "Müşteri bulunamadı.")
		return
	}
	var meta map[string]any
	if msg.Payload == nil || json.Unmarshal([]byte(*msg.Payload), &meta) != nil || meta["type"] == nil {
		fail(0, "Mesaj içeriği okunamadı.")
		return
	}
	if msg.ReplyToWAMID != nil && msg.Kind != "reaction" {
		meta["context"] = map[string]any{"message_id": *msg.ReplyToWAMID}
	}
	c, cancel := context.WithTimeout(ctx, 40*time.Second)
	wamid, err := cl.Send(c, contact.WAID, meta)
	cancel()
	if err != nil {
		attempts := msg.Attempts + 1
		if retryable(err) && attempts < 6 {
			next := time.Now().Add(time.Duration(10*(1<<attempts)) * time.Second)
			_ = s.db.WithContext(ctx).Exec("UPDATE wa_messages SET attempts = ?, next_try_at = ?, error_text = ? WHERE id = ?", attempts, next, friendlyError(err), msg.ID).Error
			return
		}
		code := 0
		var api *APIError
		if errors.As(err, &api) {
			code = api.Code
			if api.Code == 190 {
				s.recordChannelError(ctx, ch.ID, describeCode(190, ""))
			}
		}
		fail(code, friendlyError(err))
		return
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_messages SET wamid = ?, status = 'sent', sent_at = now(), attempts = attempts + 1, error_text = '' WHERE id = ?", wamid, msg.ID).Error; err != nil {
		slog.ErrorContext(ctx, "whatsapp sent message could not be updated", "message", msg.ID, "error", err)
	}
	_ = s.db.WithContext(ctx).First(msg, msg.ID).Error
	s.applyPending(ctx, msg)
	s.publish(ctx, msg.ConversationID, msg, nil)
}

// Retry queues a failed message again.
func (s *Service) Retry(ctx context.Context, actorID, messageID uint) error {
	var msg models.WAMessage
	if err := s.db.WithContext(ctx).First(&msg, messageID).Error; err != nil {
		return errs.NotFound("Mesaj bulunamadı.")
	}
	v, _, _, err := s.reachable(ctx, actorID, msg.ConversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAReply) {
		return errs.Forbidden("Müşteriye yazma yetkiniz yok.")
	}
	if msg.Status != "failed" || msg.Direction != "out" {
		return errs.Invalid("Yalnızca gönderilemeyen mesaj tekrar gönderilebilir.", nil)
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_messages SET status = 'queued', next_try_at = now(), attempts = 0, error_code = NULL, error_text = '', failed_at = NULL WHERE id = ?", msg.ID).Error; err != nil {
		return errs.Internal(err)
	}
	_ = s.db.WithContext(ctx).First(&msg, msg.ID).Error
	s.publish(ctx, msg.ConversationID, &msg, nil)
	wake(s.wakeOutbox)
	return nil
}

func (s *Service) contact(ctx context.Context, id uint) (*models.WAContact, error) {
	var c models.WAContact
	if err := s.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, errs.NotFound("Müşteri bulunamadı.")
	}
	return &c, nil
}

func (s *Service) contactOfConversation(ctx context.Context, conversationID uint) (*models.WAContact, error) {
	var c models.WAContact
	err := s.db.WithContext(ctx).Raw("SELECT c.* FROM wa_contacts c JOIN wa_conversations v ON v.contact_id = c.id WHERE v.id = ?", conversationID).Scan(&c).Error
	if err != nil || c.ID == 0 {
		return nil, fmt.Errorf("contact of conversation %d not found", conversationID)
	}
	return &c, nil
}
