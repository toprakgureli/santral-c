package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/outside"

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
	list, err := s.repo.TeamsByName(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]TeamView, 0, len(list))
	for _, t := range list {
		v := TeamView{ID: t.ID, Name: t.Name, Color: t.Color, MemberIDs: []uint{}}
		members, err := s.repo.TeamMemberIDs(ctx, t.ID)
		if err != nil {
			return nil, errs.Internal(err)
		}
		v.MemberIDs = members
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
	if _, err := s.require(ctx, actorID, enums.WATeamManage, "Ekip düzenleme yetkin yok."); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return errs.Invalid("Ekibe bir ad ver.", nil)
	}
	if err := s.repo.SaveTeam(ctx, id, name, in.Color, in.MemberIDs); err != nil {
		return errs.Internal(err)
	}
	s.forget()
	return nil
}

// DeleteTeam removes a team; its tickets keep going without it.
func (s *Service) DeleteTeam(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WATeamManage, "Ekip düzenleme yetkin yok."); err != nil {
		return err
	}
	if err := s.repo.DeleteTeam(ctx, id); err != nil {
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
	avail, err := s.repo.AvailableAgents(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
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
	if channelID == 0 && !v.can(enums.WAQuickReply) {
		return nil, errs.Forbidden("Hazır yanıtları düzenleme yetkin yok.")
	}
	list, err := s.repo.QuickReplies(ctx, channelID)
	if err != nil {
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
	if _, err := s.require(ctx, actorID, enums.WAQuickReply, "Hazır yanıtları düzenleme yetkin yok."); err != nil {
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
		return s.repo.CreateQuickReply(ctx, &r)
	}
	return s.repo.UpdateQuickReply(ctx, id, map[string]any{
		"shortcut": r.Shortcut, "title": r.Title, "body": r.Body, "channel_ids": r.ChannelIDs, "updated_at": time.Now()})
}

// DeleteQuickReply removes a ready answer.
func (s *Service) DeleteQuickReply(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WAQuickReply, "Hazır yanıtları düzenleme yetkin yok."); err != nil {
		return err
	}
	return s.repo.DeleteQuickReply(ctx, id)
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
		if _, err := s.require(ctx, actorID, enums.WAQuickReply, "Hazır yanıtları düzenleme yetkin yok."); err != nil {
			return 0, err
		}
		// The same answer is simply switched on for the target number too; a
		// shortcut the target already has is left alone.
		shared, err := s.repo.ShareQuickReplies(ctx, from, to)
		if err != nil {
			return 0, errs.Internal(err)
		}
		n = int(shared)
	case "rules":
		if _, err := s.require(ctx, actorID, enums.WAAutomation, "Otomatik mesajları düzenleme yetkin yok."); err != nil {
			return 0, err
		}
		list, err := s.repo.RulesOnChannel(ctx, from)
		if err != nil {
			return 0, errs.Internal(err)
		}
		for _, r := range list {
			c := models.WAAutomation{Name: r.Name, Active: false, ChannelIDs: jsonString([]uint{to}), Trigger: r.Trigger, Conditions: r.Conditions, Actions: r.Actions,
				CooldownMin: r.CooldownMin, Position: r.Position, CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
			if s.repo.CreateRule(ctx, &c) == nil {
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
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !v.can(enums.WAContactManage) {
		return nil, errs.Forbidden("Müşteri bilgilerini düzenleme yetkin yok.")
	}
	visible, err := s.visibleTickets(ctx, v, id)
	if err != nil {
		return nil, err
	}
	if len(visible) == 0 {
		return nil, errs.NotFound("Müşteri bulunamadı.")
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
	if err := s.repo.UpdateContact(ctx, id, fields); err != nil {
		return nil, errs.Internal(err)
	}
	c, err := s.repo.Contact(ctx, id)
	if err != nil {
		return nil, err
	}
	convs, err := s.repo.ConversationIDsOfContact(ctx, id)
	warnDB(ctx, err)
	for _, cid := range convs {
		s.publish(ctx, cid, nil, nil)
	}
	view := contactView(c)
	return &view, nil
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

// visibleTickets returns the ids of a customer's recent tickets the viewer
// may see, by the same rules as the conversation list.
func (s *Service) visibleTickets(ctx context.Context, v *viewer, contactID uint) (map[uint]bool, error) {
	tickets, err := s.repo.RecentTicketsOfContact(ctx, contactID, historyLimit)
	if err != nil {
		return nil, errs.Internal(fmt.Errorf("customer tickets could not be listed: %w", err))
	}
	if len(tickets) == 0 {
		return map[uint]bool{}, nil
	}
	ids := make([]uint, len(tickets))
	for i := range tickets {
		ids[i] = tickets[i].ID
	}
	joined, err := s.repo.JoinedTickets(ctx, v.user.ID, ids)
	if err != nil {
		return nil, errs.Internal(fmt.Errorf("ticket participants could not be listed: %w", err))
	}
	in := make(map[uint]bool, len(joined))
	for _, id := range joined {
		in[id] = true
	}
	out := make(map[uint]bool, len(tickets))
	for i := range tickets {
		t := &tickets[i]
		if v.seesTicket(t, map[uint]bool{v.user.ID: in[t.ID]}) {
			out[t.ID] = true
		}
	}
	return out, nil
}

// historyLimit is how many of a customer's newest tickets the history
// shows.
const historyLimit = 50

// ContactHistory lists the customer's tickets the person may see, newest
// first. Ratings are shown only to those who may read ratings.
func (s *Service) ContactHistory(ctx context.Context, actorID, contactID uint) ([]HistoryItem, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	visible, err := s.visibleTickets(ctx, v, contactID)
	if err != nil {
		return nil, err
	}
	if len(visible) == 0 {
		return []HistoryItem{}, nil
	}
	rows, err := s.repo.ContactTickets(ctx, contactID, historyLimit)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ratings := v.can(enums.WARatings)
	out := []HistoryItem{}
	for _, r := range rows {
		if !visible[r.TicketID] {
			continue
		}
		item := HistoryItem{ConversationID: r.ConversationID, TicketID: r.TicketID, Number: r.Number, ChannelName: r.ChannelName, Status: r.Status,
			Owner: r.Owner, CreatedAt: r.CreatedAt, ResolvedAt: r.ResolvedAt, Messages: r.Messages}
		if ratings {
			item.Rating = r.Rating
		}
		out = append(out, item)
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
	convs, err := s.repo.ConversationsOfNumber(ctx, v.reach(), key)
	if err != nil {
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
		return nil, errs.Forbidden("Bu cihazdan yeni sohbet başlatma yetkin yok.")
	}
	e164, err := phone.Normalize(number)
	if err != nil {
		return nil, errs.Invalid("Numara anlaşılamadı. Örnek: 0555 123 45 67", err)
	}
	waID := strings.TrimPrefix(e164, "+")
	convID, err := s.repo.StartConversation(ctx, channelID, waID, strings.TrimSpace(name), actorID)
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
	list, err := s.repo.Integrations(ctx)
	if err != nil {
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
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkin yok."); err != nil {
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
	if err := outside.CheckURLTemplate(strings.TrimSpace(in.URL)); err != nil {
		return errs.Invalid("Değişkenler ({ad} gibi) adresin sunucu kısmında kullanılamaz; yalnızca yolda ve sorguda olabilir.", err)
	}
	if strings.TrimSpace(in.Name) == "" {
		return errs.Invalid("Bir ad ver.", nil)
	}
	fields := map[string]any{"name": strings.TrimSpace(in.Name), "method": in.Method, "url": strings.TrimSpace(in.URL), "body": in.Body, "timeout_sec": in.TimeoutSec, "updated_at": time.Now()}
	if in.Headers != nil {
		enc, err := s.seal(jsonString(*in.Headers))
		if err != nil {
			return errs.Internal(err)
		}
		fields["headers_enc"] = enc
	} else if id != 0 {
		// The stored headers (an API key, a token) belong to the address
		// they were entered for; moving to another server drops them, so
		// they can never be sent somewhere new without being typed again.
		old, err := s.repo.IntegrationURL(ctx, id)
		if err != nil {
			return errs.NotFound("Entegrasyon bulunamadı.")
		}
		if was, err := url.Parse(old); err != nil || was.Scheme != u.Scheme || was.Host != u.Host {
			fields["headers_enc"] = ""
		}
	}
	if id == 0 {
		r := models.WAIntegration{Name: fields["name"].(string), Method: in.Method, URL: fields["url"].(string), Body: in.Body, TimeoutSec: in.TimeoutSec, UpdatedAt: time.Now()}
		if v, ok := fields["headers_enc"].(string); ok {
			r.HeadersEnc = v
		}
		return s.repo.CreateIntegration(ctx, &r)
	}
	return s.repo.UpdateIntegration(ctx, id, fields)
}

// DeleteIntegration removes an outside system.
func (s *Service) DeleteIntegration(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkin yok."); err != nil {
		return err
	}
	return s.repo.DeleteIntegration(ctx, id)
}

// TestIntegration calls an outside system with sample values.
func (s *Service) TestIntegration(ctx context.Context, actorID, id uint, vars map[string]string) (map[string]any, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkin yok."); err != nil {
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
	c, err := s.repo.Contact(ctx, conv.ContactID)
	if err != nil {
		return
	}
	cb := &models.WACallback{ChannelID: uintPtr(ch.ID), ContactID: uintPtr(c.ID), TicketID: uintPtr(t.ID), Phone: "+" + c.WAID, Note: strings.TrimSpace(note), Status: "open", CreatedAt: time.Now()}
	if err := s.repo.CreateCallback(ctx, cb); err != nil {
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
	if _, err := s.require(ctx, actorID, enums.WACallbacks, "Geri arama taleplerini görme yetkin yok."); err != nil {
		return nil, err
	}
	rows, err := s.repo.Callbacks(ctx, all)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]CallbackView, 0, len(rows))
	for _, r := range rows {
		out = append(out, CallbackView(r))
	}
	return out, nil
}

// DoneCallback closes a request.
func (s *Service) DoneCallback(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WACallbacks, "Geri arama taleplerini kapatma yetkin yok."); err != nil {
		return err
	}
	return s.repo.DoneCallback(ctx, id, actorID)
}
