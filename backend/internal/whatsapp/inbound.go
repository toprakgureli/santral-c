package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// MediaRef is a media file as stored on a message.
type MediaRef struct {
	MetaID   string `json:"metaId,omitempty"`
	StoreID  string `json:"storeId,omitempty"` // file id in our storage
	Mime     string `json:"mime,omitempty"`
	Name     string `json:"name,omitempty"`
	Size     int64  `json:"size,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Voice    bool   `json:"voice,omitempty"`
	Animated bool   `json:"animated,omitempty"`
	Failed   string `json:"failed,omitempty"` // why it could not be kept
}

func unixTime(s string) *time.Time {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return nil
	}
	t := time.Unix(n, 0)
	return &t
}

// inboundShape turns Meta's message into our row's kind, body, media and
// payload.
func inboundShape(m *hookMessage) (kind, body string, media *MediaRef, payload any) {
	pick := func(k string, md *hookMedia) (string, string, *MediaRef, any) {
		if md == nil {
			return k, "", nil, nil
		}
		return k, md.Caption, &MediaRef{MetaID: md.ID, Mime: md.MimeType, SHA256: md.SHA256, Name: md.Filename, Voice: md.Voice, Animated: md.Animated}, nil
	}
	switch m.Type {
	case "text":
		if m.Text != nil {
			return "text", m.Text.Body, nil, nil
		}
	case "image":
		return pick("image", m.Image)
	case "video":
		return pick("video", m.Video)
	case "audio":
		return pick("audio", m.Audio)
	case "document":
		return pick("document", m.Document)
	case "sticker":
		return pick("sticker", m.Sticker)
	case "location":
		if m.Location != nil {
			label := strings.TrimSpace(m.Location.Name + " " + m.Location.Address)
			if label == "" {
				label = "Konum"
			}
			return "location", label, nil, m.Location
		}
	case "contacts":
		return "contacts", "Kişi kartı", nil, m.Contacts
	case "interactive":
		if m.Interactive != nil {
			if r := m.Interactive.ButtonReply; r != nil {
				return "interactive", r.Title, nil, map[string]string{"type": "button", "id": r.ID, "title": r.Title}
			}
			if r := m.Interactive.ListReply; r != nil {
				return "interactive", r.Title, nil, map[string]string{"type": "list", "id": r.ID, "title": r.Title, "description": r.Description}
			}
		}
	case "button":
		if m.Button != nil {
			return "button", m.Button.Text, nil, map[string]string{"payload": m.Button.Payload}
		}
	case "reaction":
		if m.Reaction != nil {
			return "reaction", m.Reaction.Emoji, nil, map[string]string{"messageId": m.Reaction.MessageID, "emoji": m.Reaction.Emoji}
		}
	}
	return "unsupported", "Bu mesaj türü panelde gösterilemiyor, müşterinin telefonunda görülebilir.", nil, nil
}

// inboundResult carries what the transaction learned to the steps after it.
type inboundResult struct {
	msg      *models.WAMessage
	conv     *models.WAConversation
	ticket   *models.WATicket
	contact  *models.WAContact
	created  bool // a new ticket was opened
	reopened bool // a resolved ticket was opened again
	first    bool // the first message ever from this customer on this device
	// a tap on a call survey button: the survey and the button
	surveyID  uint
	surveyIdx int
}

func (s *Service) onInbound(ctx context.Context, ch *models.WAChannel, m *hookMessage, profileName string) error {
	kind, body, media, payload := inboundShape(m)
	at := unixTime(m.Timestamp)
	if at == nil {
		t := time.Now()
		at = &t
	}
	res := &inboundResult{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		contact, err := upsertContact(tx, m.From, profileName)
		if err != nil {
			return err
		}
		res.contact = contact
		if len(m.Referral) > 0 && string(m.Referral) != "null" {
			ref := string(m.Referral)
			if err := tx.Exec("UPDATE wa_contacts SET source = ? WHERE id = ?", ref, contact.ID).Error; err != nil {
				return err
			}
		}
		conv, first, err := upsertConversation(tx, ch.ID, contact.ID)
		if err != nil {
			return err
		}
		res.first = first
		msg := &models.WAMessage{
			ChannelID: ch.ID, ConversationID: conv.ID, Direction: "in", Kind: kind, WAMID: strPtr(m.ID),
			SenderKind: "customer", Body: body, Status: "received", WATimestamp: at, CreatedAt: time.Now(),
		}
		if media != nil {
			msg.Media = strPtr(jsonString(media))
		}
		if payload != nil {
			msg.Payload = strPtr(jsonString(payload))
		}
		if len(m.Referral) > 0 && string(m.Referral) != "null" {
			msg.Referral = strPtr(string(m.Referral))
		}
		if m.Context != nil && m.Context.ID != "" {
			msg.ReplyToWAMID = strPtr(m.Context.ID)
		}
		tx2 := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "wamid"}}, DoNothing: true}).Create(msg)
		if tx2.Error != nil {
			return tx2.Error
		}
		if tx2.RowsAffected == 0 || msg.ID == 0 {
			// Seen before: Meta sent it again. Nothing more to do.
			res.msg = nil
			return nil
		}
		res.msg = msg
		if id, idx, ok := callSurveyAnswer(m); ok {
			// An answer to the survey after a phone call: kept in the
			// conversation, but it opens no support ticket.
			res.conv, res.surveyID, res.surveyIdx = conv, id, idx
			return tx.Exec("UPDATE wa_conversations SET last_inbound_at = ?, last_message_id = ?, last_message_at = ? WHERE id = ?", *at, msg.ID, time.Now(), conv.ID).Error
		}
		if kind == "reaction" {
			res.conv = conv
			return nil
		}
		// Ticket: open one, or bring a resolved one back.
		ticket, created, reopened, err := touchTicket(tx, conv, at)
		if err != nil {
			return err
		}
		res.ticket, res.created, res.reopened = ticket, created, reopened
		if err := tx.Exec("UPDATE wa_messages SET ticket_id = ? WHERE id = ?", ticket.ID, msg.ID).Error; err != nil {
			return err
		}
		msg.TicketID = uintPtr(ticket.ID)
		if err := tx.Exec(`UPDATE wa_conversations SET last_inbound_at = ?, last_message_id = ?, last_message_at = ?,
			unread = unread + 1, ticket_id = ? WHERE id = ?`, *at, msg.ID, time.Now(), ticket.ID, conv.ID).Error; err != nil {
			return err
		}
		res.conv = conv
		return nil
	})
	if err != nil {
		return err
	}
	if res.msg == nil {
		return nil
	}
	if res.surveyID > 0 {
		s.onCallSurveyTap(ctx, ch, res.conv, res.contact, res.surveyID, res.surveyIdx)
		return nil
	}
	s.afterInbound(ctx, ch, res)
	return nil
}

// afterInbound runs everything that follows a stored customer message.
// None of it may lose the message, so failures are only logged.
func (s *Service) afterInbound(ctx context.Context, ch *models.WAChannel, res *inboundResult) {
	msg := res.msg
	if msg.Kind == "reaction" {
		s.publishReaction(ctx, res.conv.ID, msg)
		return
	}
	if msg.Media != nil {
		go s.keepMedia(context.WithoutCancel(ctx), ch, msg.ID)
	}
	set := parseSettings(ch.Settings)
	text := strings.TrimSpace(msg.Body)

	// "DUR": the customer leaves marketing messages.
	if msg.Kind == "text" && matchesWord(text, set.OptOutKeywords) {
		_ = s.db.WithContext(ctx).Exec("UPDATE wa_contacts SET opted_out = true WHERE id = ?", res.contact.ID).Error
		if strings.TrimSpace(set.OptOutReply) != "" {
			s.queueSystem(ctx, ch, res.conv.ID, res.ticket.ID, "automation", "Kampanya izni", set.OptOutReply)
		}
	}

	ticket := res.ticket
	handled := false
	switch {
	case res.created || res.reopened:
		// A chatbot for this device greets new and returning customers;
		// otherwise the ticket goes to a person.
		if s.startBot(ctx, ch, res.conv, ticket, msg) {
			handled = true
		} else {
			s.distribute(ctx, ch, ticket.ID)
		}
		if res.created {
			s.runAutomations(ctx, ch, "ticket_created", res.conv, ticket, msg)
		} else {
			s.runAutomations(ctx, ch, "ticket_reopened", res.conv, ticket, msg)
			s.notifyReopen(ctx, ticket)
		}
		if res.first {
			s.runAutomations(ctx, ch, "first_message", res.conv, ticket, msg)
		}
	case ticket.Status == "bot":
		handled = s.continueBot(ctx, ch, res.conv, ticket, msg)
	}
	if !handled || ticket.Status != "bot" {
		s.runAutomations(ctx, ch, "message_in", res.conv, ticket, msg)
		if set.Hours.Enabled && !set.Hours.Open(time.Now()) {
			s.runAutomations(ctx, ch, "outside_hours", res.conv, ticket, msg)
		}
	}
	s.handleSurveyReply(ctx, ch, res.conv, msg)
	s.publish(ctx, res.conv.ID, msg, nil)
}

func matchesWord(text string, words []string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	for _, w := range words {
		if strings.ToLower(strings.TrimSpace(w)) == t {
			return true
		}
	}
	return false
}

func upsertContact(tx *gorm.DB, waID, profileName string) (*models.WAContact, error) {
	var c models.WAContact
	err := tx.Raw(`INSERT INTO wa_contacts (wa_id, peer_key, profile_name) VALUES (?, ?, ?)
		ON CONFLICT (wa_id) DO UPDATE SET
			profile_name = CASE WHEN EXCLUDED.profile_name <> '' THEN EXCLUDED.profile_name ELSE wa_contacts.profile_name END,
			updated_at = now()
		RETURNING *`, waID, phone.Key(waID), profileName).Scan(&c).Error
	if err != nil {
		return nil, fmt.Errorf("müşteri kaydedilemedi: %w", err)
	}
	return &c, nil
}

func upsertConversation(tx *gorm.DB, channelID, contactID uint) (*models.WAConversation, bool, error) {
	var c models.WAConversation
	err := tx.Raw(`INSERT INTO wa_conversations (channel_id, contact_id) VALUES (?, ?)
		ON CONFLICT (channel_id, contact_id) DO UPDATE SET channel_id = EXCLUDED.channel_id
		RETURNING *`, channelID, contactID).Scan(&c).Error
	if err != nil {
		return nil, false, fmt.Errorf("sohbet kaydedilemedi: %w", err)
	}
	var inserted bool
	_ = tx.Raw("SELECT last_message_id IS NULL FROM wa_conversations WHERE id = ?", c.ID).Scan(&inserted).Error
	// Lock the row for the rest of the transaction.
	if err := tx.Raw("SELECT * FROM wa_conversations WHERE id = ? FOR UPDATE", c.ID).Scan(&c).Error; err != nil {
		return nil, false, err
	}
	return &c, inserted, nil
}

// touchTicket makes sure the conversation has an open ticket and marks it
// as waiting for our answer.
func touchTicket(tx *gorm.DB, conv *models.WAConversation, at *time.Time) (*models.WATicket, bool, bool, error) {
	var t models.WATicket
	created, reopened := false, false
	if conv.TicketID != nil {
		if err := tx.Raw("SELECT * FROM wa_tickets WHERE id = ? FOR UPDATE", *conv.TicketID).Scan(&t).Error; err != nil {
			return nil, false, false, err
		}
	}
	switch {
	case t.ID == 0:
		t = models.WATicket{ConversationID: conv.ID, ChannelID: conv.ChannelID, ContactID: conv.ContactID, Status: "open", Priority: "normal", Tags: "[]", AwaitingSince: at}
		if err := tx.Create(&t).Error; err != nil {
			return nil, false, false, err
		}
		if err := tx.Raw("SELECT * FROM wa_tickets WHERE id = ?", t.ID).Scan(&t).Error; err != nil {
			return nil, false, false, err
		}
		created = true
	case t.Status == "resolved":
		// Only the customer writing (or us sending a template) brings a
		// resolved ticket back. It keeps its number and history.
		if err := tx.Exec(`UPDATE wa_tickets SET status = 'open', reopen_count = reopen_count + 1, resolved_at = NULL,
			awaiting_since = ?, waiting_listed_at = NULL, updated_at = now() WHERE id = ?`, *at, t.ID).Error; err != nil {
			return nil, false, false, err
		}
		t.Status, t.ReopenCount, t.ResolvedAt, t.AwaitingSince, t.WaitingListedAt = "open", t.ReopenCount+1, nil, at, nil
		reopened = true
	case t.AwaitingSince == nil:
		if err := tx.Exec("UPDATE wa_tickets SET awaiting_since = ?, updated_at = now() WHERE id = ?", *at, t.ID).Error; err != nil {
			return nil, false, false, err
		}
		t.AwaitingSince = at
	default:
		_ = tx.Exec("UPDATE wa_tickets SET updated_at = now() WHERE id = ?", t.ID).Error
	}
	return &t, created, reopened, nil
}

// ---------------------------------------------------------------- statuses

var statusRank = map[string]int{"queued": 0, "sent": 1, "delivered": 2, "read": 3}

func (s *Service) onStatus(ctx context.Context, ch *models.WAChannel, st *hookStatus) error {
	var msg models.WAMessage
	err := s.db.WithContext(ctx).Where("wamid = ?", st.ID).Limit(1).Find(&msg).Error
	if err != nil {
		return err
	}
	if msg.ID == 0 {
		// The status came before we stored the message's id; keep it.
		raw, _ := json.Marshal(st)
		return s.db.WithContext(ctx).Exec("INSERT INTO wa_pending_statuses (wamid, status, payload) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", st.ID, st.Status, string(raw)).Error
	}
	s.applyStatus(ctx, &msg, st)
	return nil
}

// applyStatus moves a message's ticks forward (never back) and records a
// failure with its reason.
func (s *Service) applyStatus(ctx context.Context, msg *models.WAMessage, st *hookStatus) {
	at := unixTime(st.Timestamp)
	if at == nil {
		t := time.Now()
		at = &t
	}
	fields := map[string]any{}
	if len(st.Pricing) > 0 && string(st.Pricing) != "null" {
		fields["pricing"] = string(st.Pricing)
	}
	switch st.Status {
	case "sent", "delivered", "read":
		if msg.Status != "failed" && statusRank[st.Status] > statusRank[msg.Status] {
			fields["status"] = st.Status
		}
		switch st.Status {
		case "sent":
			if msg.SentAt == nil {
				fields["sent_at"] = *at
			}
		case "delivered":
			if msg.DeliveredAt == nil {
				fields["delivered_at"] = *at
			}
		case "read":
			if msg.ReadAt == nil {
				fields["read_at"] = *at
			}
			if msg.DeliveredAt == nil {
				fields["delivered_at"] = *at
			}
		}
	case "failed":
		code, text := 0, "Mesaj iletilemedi."
		if len(st.Errors) > 0 {
			code = st.Errors[0].Code
			text = describeCode(code, st.Errors[0].Title+" "+st.Errors[0].ErrorData.Details)
		}
		fields["status"], fields["failed_at"], fields["error_code"], fields["error_text"] = "failed", *at, code, text
	}
	if len(fields) == 0 {
		return
	}
	if err := s.db.WithContext(ctx).Model(&models.WAMessage{}).Where("id = ?", msg.ID).Updates(fields).Error; err != nil {
		slog.WarnContext(ctx, "whatsapp status could not be saved", "message", msg.ID, "error", err)
		return
	}
	_ = s.db.WithContext(ctx).First(msg, msg.ID).Error
	s.publish(ctx, msg.ConversationID, msg, nil)
}

// applyPending applies statuses that arrived before the message's id.
func (s *Service) applyPending(ctx context.Context, msg *models.WAMessage) {
	if msg.WAMID == nil {
		return
	}
	var rows []struct {
		Payload string
	}
	_ = s.db.WithContext(ctx).Raw("DELETE FROM wa_pending_statuses WHERE wamid = ? RETURNING payload", *msg.WAMID).Scan(&rows).Error
	for _, r := range rows {
		var st hookStatus
		if json.Unmarshal([]byte(r.Payload), &st) == nil {
			s.applyStatus(ctx, msg, &st)
		}
	}
}

// publishReaction sends the reacted-to message again with its reactions.
func (s *Service) publishReaction(ctx context.Context, conversationID uint, reaction *models.WAMessage) {
	var p struct {
		MessageID string `json:"messageId"`
	}
	if reaction.Payload != nil {
		_ = json.Unmarshal([]byte(*reaction.Payload), &p)
	}
	var target models.WAMessage
	if p.MessageID != "" {
		_ = s.db.WithContext(ctx).Where("wamid = ?", p.MessageID).Limit(1).Find(&target).Error
	}
	if target.ID == 0 {
		s.publish(ctx, conversationID, nil, nil)
		return
	}
	s.publish(ctx, conversationID, &target, nil)
}
