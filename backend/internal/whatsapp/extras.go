package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// ---------------------------------------------------------------- teams

// TeamView is an agent team.
type TeamView struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	MemberIDs []uint `json:"memberIds"`
}

// Teams lists the agent teams; anyone in the module may read them.
func (s *Service) Teams(ctx context.Context, actorID uint) ([]TeamView, error) {
	if _, err := s.viewerOf(ctx, actorID); err != nil {
		return nil, err
	}
	var list []models.WATeam
	if err := s.db.WithContext(ctx).Order("name").Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]TeamView, 0, len(list))
	for _, t := range list {
		v := TeamView{ID: t.ID, Name: t.Name, Color: t.Color, MemberIDs: []uint{}}
		_ = s.db.WithContext(ctx).Raw("SELECT user_id FROM wa_team_members WHERE team_id = ? ORDER BY user_id", t.ID).Scan(&v.MemberIDs).Error
		if v.MemberIDs == nil {
			v.MemberIDs = []uint{}
		}
		out = append(out, v)
	}
	return out, nil
}

// TeamInput is the team form.
type TeamInput struct {
	Name      string `json:"name"`
	Color     string `json:"color"`
	MemberIDs []uint `json:"memberIds"`
}

// SaveTeam creates (id 0) or updates a team with its members.
func (s *Service) SaveTeam(ctx context.Context, actorID, id uint, in TeamInput) error {
	if _, err := s.require(ctx, actorID, enums.WATeamManage, "Ekip düzenleme yetkiniz yok."); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return errs.Invalid("Ekibe bir ad verin.", nil)
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id == 0 {
			t := &models.WATeam{Name: name, Color: in.Color}
			if err := tx.Create(t).Error; err != nil {
				return err
			}
			id = t.ID
		} else if err := tx.Exec("UPDATE wa_teams SET name = ?, color = ? WHERE id = ?", name, in.Color, id).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM wa_team_members WHERE team_id = ?", id).Error; err != nil {
			return err
		}
		for _, uid := range in.MemberIDs {
			if err := tx.Exec("INSERT INTO wa_team_members (team_id, user_id) VALUES (?, ?) ON CONFLICT DO NOTHING", id, uid).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return errs.Internal(err)
	}
	s.forget()
	return nil
}

// DeleteTeam removes a team; its tickets keep going without it.
func (s *Service) DeleteTeam(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WATeamManage, "Ekip düzenleme yetkiniz yok."); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Delete(&models.WATeam{}, id).Error; err != nil {
		return errs.Internal(err)
	}
	s.forget()
	return nil
}

// AgentView is someone who can work in the module, for pickers.
type AgentView struct {
	PersonView
	ChannelIDs []uint `json:"channelIds"`
	Online     bool   `json:"online"`
	Available  bool   `json:"available"`
	CanReply   bool   `json:"canReply"`
}

// Agents lists everyone who may use the module, with where they work and
// whether they are available now.
func (s *Service) Agents(ctx context.Context, actorID uint) ([]AgentView, error) {
	if _, err := s.viewerOf(ctx, actorID); err != nil {
		return nil, err
	}
	viewers, err := s.loadViewers(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	var ids []uint
	for id := range viewers {
		ids = append(ids, id)
	}
	people := s.people(ctx, ids)
	online := s.push.OnlineUsers(ids)
	var avail []uint
	_ = s.db.WithContext(ctx).Raw(`SELECT sh.user_id FROM shifts sh LEFT JOIN agent_presence ap ON ap.user_id = sh.user_id
		WHERE sh.ended_at IS NULL AND COALESCE(ap.state, 'available') = 'available'`).Scan(&avail).Error
	am := map[uint]bool{}
	for _, a := range avail {
		am[a] = true
	}
	out := make([]AgentView, 0, len(ids))
	for id, v := range viewers {
		if v.user.IsInvisibleAdmin() && id != actorID {
			continue
		}
		a := AgentView{PersonView: people[id], Online: online[id], Available: am[id], CanReply: v.can(enums.WAReply), ChannelIDs: []uint{}}
		for ch := range v.channels {
			a.ChannelIDs = append(a.ChannelIDs, ch)
		}
		out = append(out, a)
	}
	return out, nil
}

// ---------------------------------------------------------------- quick replies

// QuickReplyView is a ready answer.
type QuickReplyView struct {
	ID         uint   `json:"id"`
	Shortcut   string `json:"shortcut"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	ChannelIDs []uint `json:"channelIds"`
}

// QuickReplies lists ready answers; with a device, only that device's.
func (s *Service) QuickReplies(ctx context.Context, actorID, channelID uint) ([]QuickReplyView, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Order("shortcut")
	if channelID > 0 {
		q = q.Where("channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", channelID))
	} else if !v.can(enums.WAQuickReply) {
		return nil, errs.Forbidden("Hazır yanıtları düzenleme yetkiniz yok.")
	}
	var list []models.WAQuickReply
	if err := q.Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]QuickReplyView, 0, len(list))
	for _, r := range list {
		out = append(out, QuickReplyView{ID: r.ID, Shortcut: r.Shortcut, Title: r.Title, Body: r.Body, ChannelIDs: parseIDs(r.ChannelIDs)})
	}
	return out, nil
}

// SaveQuickReply creates (id 0) or updates a ready answer.
func (s *Service) SaveQuickReply(ctx context.Context, actorID, id uint, in QuickReplyView) error {
	if _, err := s.require(ctx, actorID, enums.WAQuickReply, "Hazır yanıtları düzenleme yetkiniz yok."); err != nil {
		return err
	}
	in.Shortcut = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(in.Shortcut, "/")))
	in.Body = strings.TrimSpace(in.Body)
	if in.Shortcut == "" || strings.ContainsAny(in.Shortcut, " \t") || in.Body == "" {
		return errs.Invalid("Kısayol boşluk içermemeli, metin boş olmamalı.", nil)
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = in.Shortcut
	}
	if in.ChannelIDs == nil {
		in.ChannelIDs = []uint{}
	}
	r := models.WAQuickReply{ID: id, Shortcut: in.Shortcut, Title: strings.TrimSpace(in.Title), Body: in.Body, ChannelIDs: jsonString(in.ChannelIDs), CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	if id == 0 {
		return s.db.WithContext(ctx).Create(&r).Error
	}
	return s.db.WithContext(ctx).Model(&models.WAQuickReply{}).Where("id = ?", id).Updates(map[string]any{
		"shortcut": r.Shortcut, "title": r.Title, "body": r.Body, "channel_ids": r.ChannelIDs, "updated_at": time.Now()}).Error
}

// DeleteQuickReply removes a ready answer.
func (s *Service) DeleteQuickReply(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WAQuickReply, "Hazır yanıtları düzenleme yetkiniz yok."); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&models.WAQuickReply{}, id).Error
}

// CopyToChannel copies a device's quick replies, rules or chatbots to
// another device as independent copies.
func (s *Service) CopyToChannel(ctx context.Context, actorID, from, to uint, what string) (int, error) {
	if from == to {
		return 0, errs.Invalid("Kaynak ve hedef cihaz aynı olamaz.", nil)
	}
	n := 0
	switch what {
	case "quick_replies":
		if _, err := s.require(ctx, actorID, enums.WAQuickReply, "Hazır yanıtları düzenleme yetkiniz yok."); err != nil {
			return 0, err
		}
		// The same answer is simply switched on for the target number too; a
		// shortcut the target already has is left alone.
		src, dst := fmt.Sprintf("[%d]", from), fmt.Sprintf("[%d]", to)
		res := s.db.WithContext(ctx).Exec(`UPDATE wa_quick_replies q SET channel_ids = q.channel_ids || ?::jsonb, updated_at = now()
			WHERE q.channel_ids @> ?::jsonb AND NOT q.channel_ids @> ?::jsonb
			AND NOT EXISTS (SELECT 1 FROM wa_quick_replies o WHERE o.id <> q.id AND lower(o.shortcut) = lower(q.shortcut) AND o.channel_ids @> ?::jsonb)`,
			dst, src, dst, dst)
		if res.Error != nil {
			return 0, errs.Internal(res.Error)
		}
		n = int(res.RowsAffected)
	case "rules":
		if _, err := s.require(ctx, actorID, enums.WAAutomation, "Otomatik mesajları düzenleme yetkiniz yok."); err != nil {
			return 0, err
		}
		var list []models.WAAutomation
		_ = s.db.WithContext(ctx).Where("channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", from)).Find(&list).Error
		for _, r := range list {
			c := models.WAAutomation{Name: r.Name, Active: false, ChannelIDs: jsonString([]uint{to}), Trigger: r.Trigger, Conditions: r.Conditions, Actions: r.Actions,
				CooldownMin: r.CooldownMin, Position: r.Position, CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
			if s.db.WithContext(ctx).Create(&c).Error == nil {
				n++
			}
		}
	default:
		return 0, errs.Invalid("Neyin kopyalanacağı tanınmadı.", nil)
	}
	return n, nil
}

// ---------------------------------------------------------------- customers

// ContactInput edits a customer.
type ContactInput struct {
	Name     *string   `json:"name"`
	Tags     *[]string `json:"tags"`
	Note     *string   `json:"note"`
	OptedOut *bool     `json:"optedOut"`
	Blocked  *bool     `json:"blocked"`
}

// UpdateContact edits a customer's card.
func (s *Service) UpdateContact(ctx context.Context, actorID, id uint, in ContactInput) (*ContactView, error) {
	if _, err := s.require(ctx, actorID, enums.WAContactManage, "Müşteri bilgilerini düzenleme yetkiniz yok."); err != nil {
		return nil, err
	}
	fields := map[string]any{"updated_at": time.Now()}
	if in.Name != nil {
		fields["name"] = strings.TrimSpace(*in.Name)
	}
	if in.Tags != nil {
		fields["tags"] = jsonString(cleanTags(*in.Tags))
	}
	if in.Note != nil {
		fields["note"] = strings.TrimSpace(*in.Note)
	}
	if in.OptedOut != nil {
		fields["opted_out"] = *in.OptedOut
	}
	if in.Blocked != nil {
		fields["blocked"] = *in.Blocked
	}
	if err := s.db.WithContext(ctx).Model(&models.WAContact{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return nil, errs.Internal(err)
	}
	c, err := s.contact(ctx, id)
	if err != nil {
		return nil, err
	}
	var convs []uint
	_ = s.db.WithContext(ctx).Raw("SELECT id FROM wa_conversations WHERE contact_id = ?", id).Scan(&convs).Error
	for _, cid := range convs {
		s.publish(ctx, cid, nil, nil)
	}
	v := contactView(c)
	return &v, nil
}

// HistoryItem is one past ticket of a customer.
type HistoryItem struct {
	ConversationID uint       `json:"conversationId"`
	TicketID       uint       `json:"ticketId"`
	Number         int64      `json:"number"`
	ChannelName    string     `json:"channelName"`
	Status         string     `json:"status"`
	Owner          string     `json:"owner"`
	CreatedAt      time.Time  `json:"createdAt"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	Rating         *int       `json:"rating,omitempty"`
	Messages       int64      `json:"messages"`
}

// ContactHistory lists a customer's tickets on every device the person
// sees, newest first.
func (s *Service) ContactHistory(ctx context.Context, actorID, contactID uint) ([]HistoryItem, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ConversationID uint
		TicketID       uint
		Number         int64
		ChannelID      uint
		ChannelName    string
		Status         string
		Owner          string
		CreatedAt      time.Time
		ResolvedAt     *time.Time
		Rating         *int
		Messages       int64
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT t.conversation_id, t.id AS ticket_id, t.number, t.channel_id, c.name AS channel_name, t.status,
		COALESCE(u.name, '') AS owner, t.created_at, t.resolved_at, t.rating,
		(SELECT count(*) FROM wa_messages m WHERE m.ticket_id = t.id AND m.direction IN ('in','out')) AS messages
		FROM wa_tickets t JOIN wa_channels c ON c.id = t.channel_id LEFT JOIN users u ON u.id = t.owner_id
		WHERE t.contact_id = ? ORDER BY t.id DESC LIMIT 50`, contactID).Scan(&rows).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := []HistoryItem{}
	for _, r := range rows {
		if !v.seesChannel(r.ChannelID) {
			continue
		}
		out = append(out, HistoryItem{ConversationID: r.ConversationID, TicketID: r.TicketID, Number: r.Number, ChannelName: r.ChannelName, Status: r.Status,
			Owner: r.Owner, CreatedAt: r.CreatedAt, ResolvedAt: r.ResolvedAt, Rating: r.Rating, Messages: r.Messages})
	}
	return out, nil
}

// LookupNumber finds the WhatsApp conversations of a phone number, for
// the number search and the call screen.
func (s *Service) LookupNumber(ctx context.Context, actorID uint, number string) ([]ConversationView, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	key := phone.Key(number)
	if len(key) < 3 {
		return []ConversationView{}, nil
	}
	var convs []models.WAConversation
	if err := s.db.WithContext(ctx).Where("contact_id IN (SELECT id FROM wa_contacts WHERE peer_key = ?) AND ticket_id IS NOT NULL", key).Order("last_message_at DESC NULLS LAST").Limit(20).Find(&convs).Error; err != nil {
		return nil, errs.Internal(err)
	}
	visible, _ := s.filterVisible(ctx, v, convs)
	return s.summaries(ctx, visible)
}

// StartConversation opens (or finds) a conversation with a number, so an
// agent can write first with a template.
func (s *Service) StartConversation(ctx context.Context, actorID, channelID uint, number, name string) (*ConversationView, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !v.can(enums.WATemplateSend) || !v.seesChannel(channelID) {
		return nil, errs.Forbidden("Bu cihazdan yeni sohbet başlatma yetkiniz yok.")
	}
	e164, err := phone.Normalize(number)
	if err != nil {
		return nil, errs.Invalid("Numara anlaşılamadı. Örnek: 0530 123 45 67", err)
	}
	waID := strings.TrimPrefix(e164, "+")
	var convID uint
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := upsertContact(tx, waID, "")
		if err != nil {
			return err
		}
		if n := strings.TrimSpace(name); n != "" && c.Name == "" {
			_ = tx.Exec("UPDATE wa_contacts SET name = ? WHERE id = ?", n, c.ID).Error
		}
		conv, _, err := upsertConversation(tx, channelID, c.ID)
		if err != nil {
			return err
		}
		convID = conv.ID
		if conv.TicketID == nil {
			t := models.WATicket{ConversationID: conv.ID, ChannelID: channelID, ContactID: c.ID, Status: "open", Priority: "normal", Tags: "[]", OwnerID: uintPtr(actorID)}
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE wa_conversations SET ticket_id = ?, last_message_at = now() WHERE id = ?", t.ID, conv.ID).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, 'owner') ON CONFLICT DO NOTHING", t.ID, actorID).Error
		}
		return nil
	})
	if err != nil {
		return nil, errs.Internal(err)
	}
	s.publish(ctx, convID, nil, nil)
	return s.Conversation(ctx, actorID, convID)
}

// ---------------------------------------------------------------- outside systems

// IntegrationView is an outside system a chatbot may ask. Header values
// are secret and never sent back; only their names.
type IntegrationView struct {
	ID          uint     `json:"id"`
	Name        string   `json:"name"`
	Method      string   `json:"method"`
	URL         string   `json:"url"`
	Body        string   `json:"body"`
	TimeoutSec  int      `json:"timeoutSec"`
	HeaderNames []string `json:"headerNames"`
}

// Integrations lists the outside systems.
func (s *Service) Integrations(ctx context.Context, actorID uint) ([]IntegrationView, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	var list []models.WAIntegration
	if err := s.db.WithContext(ctx).Order("name").Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]IntegrationView, 0, len(list))
	for _, in := range list {
		v := IntegrationView{ID: in.ID, Name: in.Name, Method: in.Method, URL: in.URL, Body: in.Body, TimeoutSec: in.TimeoutSec, HeaderNames: []string{}}
		var hs map[string]string
		if json.Unmarshal([]byte(s.open(in.HeadersEnc)), &hs) == nil {
			for k := range hs {
				v.HeaderNames = append(v.HeaderNames, k)
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// IntegrationInput is the form; Headers replaces the stored ones when
// given.
type IntegrationInput struct {
	Name       string             `json:"name"`
	Method     string             `json:"method"`
	URL        string             `json:"url"`
	Body       string             `json:"body"`
	TimeoutSec int                `json:"timeoutSec"`
	Headers    *map[string]string `json:"headers"`
}

// SaveIntegration creates (id 0) or updates an outside system.
func (s *Service) SaveIntegration(ctx context.Context, actorID, id uint, in IntegrationInput) error {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkiniz yok."); err != nil {
		return err
	}
	in.Method = strings.ToUpper(strings.TrimSpace(in.Method))
	switch in.Method {
	case "POST", "PUT", "PATCH":
	default:
		in.Method = "GET"
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errs.Invalid("Adres http:// ya da https:// ile başlayan tam bir adres olmalı.", nil)
	}
	if strings.TrimSpace(in.Name) == "" {
		return errs.Invalid("Bir ad verin.", nil)
	}
	fields := map[string]any{"name": strings.TrimSpace(in.Name), "method": in.Method, "url": strings.TrimSpace(in.URL), "body": in.Body, "timeout_sec": in.TimeoutSec, "updated_at": time.Now()}
	if in.Headers != nil {
		enc, err := s.seal(jsonString(*in.Headers))
		if err != nil {
			return errs.Internal(err)
		}
		fields["headers_enc"] = enc
	}
	if id == 0 {
		r := models.WAIntegration{Name: fields["name"].(string), Method: in.Method, URL: fields["url"].(string), Body: in.Body, TimeoutSec: in.TimeoutSec, UpdatedAt: time.Now()}
		if v, ok := fields["headers_enc"].(string); ok {
			r.HeadersEnc = v
		}
		return s.db.WithContext(ctx).Create(&r).Error
	}
	return s.db.WithContext(ctx).Model(&models.WAIntegration{}).Where("id = ?", id).Updates(fields).Error
}

// DeleteIntegration removes an outside system.
func (s *Service) DeleteIntegration(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkiniz yok."); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&models.WAIntegration{}, id).Error
}

// TestIntegration calls an outside system with sample values.
func (s *Service) TestIntegration(ctx context.Context, actorID, id uint, vars map[string]string) (map[string]any, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkiniz yok."); err != nil {
		return nil, err
	}
	res, err := s.callIntegration(ctx, id, vars)
	if err != nil {
		return nil, errs.Invalid("Sorgu başarısız: "+err.Error(), err)
	}
	return res, nil
}

// ---------------------------------------------------------------- callbacks

// CallbackView is a "call me back" request.
type CallbackView struct {
	ID             uint       `json:"id"`
	ChannelName    string     `json:"channelName"`
	ConversationID uint       `json:"conversationId"`
	Customer       string     `json:"customer"`
	Phone          string     `json:"phone"`
	Note           string     `json:"note"`
	Status         string     `json:"status"`
	DoneBy         string     `json:"doneBy,omitempty"`
	DoneAt         *time.Time `json:"doneAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

func (s *Service) createCallback(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, t *models.WATicket, note string) {
	c, err := s.contact(ctx, conv.ContactID)
	if err != nil {
		return
	}
	cb := &models.WACallback{ChannelID: uintPtr(ch.ID), ContactID: uintPtr(c.ID), TicketID: uintPtr(t.ID), Phone: "+" + c.WAID, Note: strings.TrimSpace(note), Status: "open", CreatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(cb).Error; err != nil {
		return
	}
	s.event(ctx, nil, conv, t.ID, 0, "Müşteri geri aranmak istedi. Talep geri arama listesine eklendi.")
	viewers, _ := s.loadViewers(ctx)
	var ids []uint
	for id, v := range viewers {
		if v.can(enums.WACallbacks) {
			ids = append(ids, id)
		}
	}
	s.push.Push(ids, Event{Type: "wa.callback", ConversationID: conv.ID, Text: contactView(c).Display + " geri aranmak istiyor."})
}

// Callbacks lists the requests, open ones first.
func (s *Service) Callbacks(ctx context.Context, actorID uint, all bool) ([]CallbackView, error) {
	if _, err := s.require(ctx, actorID, enums.WACallbacks, "Geri arama taleplerini görme yetkiniz yok."); err != nil {
		return nil, err
	}
	q := `SELECT b.id, COALESCE(ch.name, '') AS channel_name, COALESCE(t.conversation_id, 0) AS conversation_id,
		COALESCE(NULLIF(c.name, ''), NULLIF(c.profile_name, ''), b.phone) AS customer, b.phone, b.note, b.status,
		COALESCE(u.name, '') AS done_by, b.done_at, b.created_at
		FROM wa_callbacks b LEFT JOIN wa_channels ch ON ch.id = b.channel_id LEFT JOIN wa_contacts c ON c.id = b.contact_id
		LEFT JOIN wa_tickets t ON t.id = b.ticket_id LEFT JOIN users u ON u.id = b.done_by`
	if !all {
		q += " WHERE b.status = 'open' OR b.done_at > now() - interval '1 day'"
	}
	q += " ORDER BY (b.status = 'open') DESC, b.id DESC LIMIT 300"
	var out []CallbackView
	if err := s.db.WithContext(ctx).Raw(q).Scan(&out).Error; err != nil {
		return nil, errs.Internal(err)
	}
	if out == nil {
		out = []CallbackView{}
	}
	return out, nil
}

// DoneCallback closes a request.
func (s *Service) DoneCallback(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WACallbacks, "Geri arama taleplerini kapatma yetkiniz yok."); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Exec("UPDATE wa_callbacks SET status = 'done', done_by = ?, done_at = now() WHERE id = ?", actorID, id).Error
}
