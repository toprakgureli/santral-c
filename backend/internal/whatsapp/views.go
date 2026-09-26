package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// PersonView is a panel user as the inbox shows them.
type PersonView struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	HasAvatar     bool   `json:"hasAvatar"`
	AvatarVersion int64  `json:"avatarVersion,omitempty"`
	Role          string `json:"role,omitempty"`
}

// ContactView is a customer.
type ContactView struct {
	ID          uint            `json:"id"`
	WAID        string          `json:"waId"`
	Name        string          `json:"name"`
	ProfileName string          `json:"profileName"`
	Display     string          `json:"display"`
	Tags        []string        `json:"tags"`
	Note        string          `json:"note"`
	OptedOut    bool            `json:"optedOut"`
	Blocked     bool            `json:"blocked"`
	Source      json.RawMessage `json:"source,omitempty"`
}

// TicketView is a ticket's card.
type TicketView struct {
	ID              uint         `json:"id"`
	Number          int64        `json:"number"`
	Status          string       `json:"status"`
	Priority        string       `json:"priority"`
	Category        string       `json:"category"`
	Tags            []string     `json:"tags"`
	Owner           *PersonView  `json:"owner,omitempty"`
	TeamID          *uint        `json:"teamId,omitempty"`
	TeamName        string       `json:"teamName,omitempty"`
	AwaitingSince   *time.Time   `json:"awaitingSince,omitempty"`
	WaitingListedAt *time.Time   `json:"waitingListedAt,omitempty"`
	WaitingCount    int          `json:"waitingCount"`
	ReopenCount     int          `json:"reopenCount"`
	FirstResponseAt *time.Time   `json:"firstResponseAt,omitempty"`
	ResolvedAt      *time.Time   `json:"resolvedAt,omitempty"`
	ResolvedBy      *PersonView  `json:"resolvedBy,omitempty"`
	Rating          *int         `json:"rating,omitempty"`
	RatingComment   string       `json:"ratingComment,omitempty"`
	CreatedAt       time.Time    `json:"createdAt"`
	Participants    []PersonView `json:"participants"`
}

// LastView is the list's one-line preview.
type LastView struct {
	ID         uint      `json:"id"`
	Direction  string    `json:"direction"`
	Kind       string    `json:"kind"`
	Preview    string    `json:"preview"`
	At         time.Time `json:"at"`
	SenderName string    `json:"senderName,omitempty"`
	Status     string    `json:"status"`
}

// ConversationView is one row of the inbox list.
type ConversationView struct {
	ID            uint        `json:"id"`
	ChannelID     uint        `json:"channelId"`
	ChannelName   string      `json:"channelName"`
	Contact       ContactView `json:"contact"`
	Ticket        *TicketView `json:"ticket,omitempty"`
	Last          *LastView   `json:"last,omitempty"`
	Unread        int         `json:"unread"`
	TeamReadID    uint        `json:"teamReadId"`
	LastInboundAt *time.Time  `json:"lastInboundAt,omitempty"`
	WindowEndsAt  *time.Time  `json:"windowEndsAt,omitempty"`
	Version       int64       `json:"version"`
}

func (s *Service) people(ctx context.Context, ids []uint) map[uint]PersonView {
	out := map[uint]PersonView{}
	if len(ids) == 0 {
		return out
	}
	var rows []struct {
		ID        uint
		Name      string
		HasAvatar bool
		Version   int64
	}
	_ = s.db.WithContext(ctx).Raw("SELECT id, name, avatar <> '' AS has_avatar, extract(epoch from updated_at)::bigint AS version FROM users WHERE id IN ?", ids).Scan(&rows).Error
	for _, r := range rows {
		p := PersonView{ID: r.ID, Name: r.Name, HasAvatar: r.HasAvatar}
		if r.HasAvatar {
			p.AvatarVersion = r.Version
		}
		out[r.ID] = p
	}
	return out
}

func parseTags(raw string) []string {
	var t []string
	_ = json.Unmarshal([]byte(raw), &t)
	if t == nil {
		t = []string{}
	}
	return t
}

func contactView(c *models.WAContact) ContactView {
	v := ContactView{ID: c.ID, WAID: c.WAID, Name: c.Name, ProfileName: c.ProfileName, Tags: parseTags(c.Tags), Note: c.Note, OptedOut: c.OptedOut, Blocked: c.Blocked}
	v.Display = c.Name
	if v.Display == "" {
		v.Display = c.ProfileName
	}
	if v.Display == "" {
		v.Display = "+" + c.WAID
	}
	if c.Source != nil {
		v.Source = json.RawMessage(*c.Source)
	}
	return v
}

// preview is a message's one-line description.
func preview(m *models.WAMessage) string {
	body := strings.TrimSpace(m.Body)
	label := map[string]string{"image": "Görsel", "video": "Video", "audio": "Ses kaydı", "document": "Belge", "sticker": "Çıkartma", "location": "Konum", "contacts": "Kişi kartı"}[m.Kind]
	if m.Kind == "audio" && m.Media != nil && strings.Contains(*m.Media, `"voice":true`) {
		label = "Sesli mesaj"
	}
	switch {
	case label != "" && body != "":
		return label + ": " + body
	case label != "":
		return label
	case m.Kind == "template" && body == "":
		return "Şablon: " + m.SenderLabel
	}
	if len([]rune(body)) > 140 {
		body = string([]rune(body)[:140]) + "…"
	}
	return body
}

// summaries builds list rows for conversations.
func (s *Service) summaries(ctx context.Context, convs []models.WAConversation) ([]ConversationView, error) {
	if len(convs) == 0 {
		return []ConversationView{}, nil
	}
	var contactIDs, ticketIDs, lastIDs, channelIDs []uint
	for _, c := range convs {
		contactIDs = append(contactIDs, c.ContactID)
		channelIDs = append(channelIDs, c.ChannelID)
		if c.TicketID != nil {
			ticketIDs = append(ticketIDs, *c.TicketID)
		}
		if c.LastMessageID != nil {
			lastIDs = append(lastIDs, *c.LastMessageID)
		}
	}
	var contacts []models.WAContact
	if err := s.db.WithContext(ctx).Where("id IN ?", contactIDs).Find(&contacts).Error; err != nil {
		return nil, err
	}
	cm := map[uint]*models.WAContact{}
	for i := range contacts {
		cm[contacts[i].ID] = &contacts[i]
	}
	var chans []models.WAChannel
	_ = s.db.WithContext(ctx).Select("id, name").Where("id IN ?", channelIDs).Find(&chans).Error
	chName := map[uint]string{}
	for _, c := range chans {
		chName[c.ID] = c.Name
	}
	tickets := map[uint]*models.WATicket{}
	parts := map[uint][]models.WAParticipant{}
	if len(ticketIDs) > 0 {
		var list []models.WATicket
		_ = s.db.WithContext(ctx).Where("id IN ?", ticketIDs).Find(&list).Error
		for i := range list {
			tickets[list[i].ID] = &list[i]
		}
		var ps []models.WAParticipant
		_ = s.db.WithContext(ctx).Where("ticket_id IN ?", ticketIDs).Order("joined_at").Find(&ps).Error
		for _, p := range ps {
			parts[p.TicketID] = append(parts[p.TicketID], p)
		}
	}
	lasts := map[uint]*models.WAMessage{}
	if len(lastIDs) > 0 {
		var list []models.WAMessage
		_ = s.db.WithContext(ctx).Where("id IN ?", lastIDs).Find(&list).Error
		for i := range list {
			lasts[list[i].ID] = &list[i]
		}
	}
	var userIDs []uint
	for _, t := range tickets {
		if t.OwnerID != nil {
			userIDs = append(userIDs, *t.OwnerID)
		}
		if t.ResolvedBy != nil {
			userIDs = append(userIDs, *t.ResolvedBy)
		}
	}
	for _, ps := range parts {
		for _, p := range ps {
			userIDs = append(userIDs, p.UserID)
		}
	}
	for _, m := range lasts {
		if m.SenderUserID != nil {
			userIDs = append(userIDs, *m.SenderUserID)
		}
	}
	people := s.people(ctx, userIDs)
	teamNames := map[uint]string{}
	var teams []models.WATeam
	_ = s.db.WithContext(ctx).Find(&teams).Error
	for _, t := range teams {
		teamNames[t.ID] = t.Name
	}

	out := make([]ConversationView, 0, len(convs))
	for _, c := range convs {
		ct := cm[c.ContactID]
		if ct == nil {
			continue
		}
		v := ConversationView{ID: c.ID, ChannelID: c.ChannelID, ChannelName: chName[c.ChannelID], Contact: contactView(ct), Unread: c.Unread, TeamReadID: c.TeamReadID, LastInboundAt: c.LastInboundAt, Version: c.Version}
		if c.LastInboundAt != nil {
			end := c.LastInboundAt.Add(24 * time.Hour)
			v.WindowEndsAt = &end
		}
		if c.TicketID != nil {
			if t := tickets[*c.TicketID]; t != nil {
				tv := &TicketView{ID: t.ID, Number: t.Number, Status: t.Status, Priority: t.Priority, Category: t.Category, Tags: parseTags(t.Tags),
					TeamID: t.TeamID, AwaitingSince: t.AwaitingSince, WaitingListedAt: t.WaitingListedAt, WaitingCount: t.WaitingCount,
					ReopenCount: t.ReopenCount, FirstResponseAt: t.FirstResponseAt, ResolvedAt: t.ResolvedAt, Rating: t.Rating,
					RatingComment: t.RatingComment, CreatedAt: t.CreatedAt, Participants: []PersonView{}}
				if t.OwnerID != nil {
					if p, ok := people[*t.OwnerID]; ok {
						tv.Owner = &p
					}
				}
				if t.ResolvedBy != nil {
					if p, ok := people[*t.ResolvedBy]; ok {
						tv.ResolvedBy = &p
					}
				}
				if t.TeamID != nil {
					tv.TeamName = teamNames[*t.TeamID]
				}
				for _, p := range parts[t.ID] {
					if pv, ok := people[p.UserID]; ok {
						pv.Role = p.Role
						tv.Participants = append(tv.Participants, pv)
					}
				}
				v.Ticket = tv
			}
		}
		if c.LastMessageID != nil {
			if m := lasts[*c.LastMessageID]; m != nil {
				lv := &LastView{ID: m.ID, Direction: m.Direction, Kind: m.Kind, Preview: preview(m), At: m.CreatedAt, Status: m.Status}
				if m.SenderUserID != nil {
					lv.SenderName = firstName(people[*m.SenderUserID].Name)
				} else if m.SenderKind == "bot" {
					lv.SenderName = "Chatbot"
				} else if m.SenderKind == "automation" {
					lv.SenderName = "Otomatik"
				}
				v.Last = lv
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// ---------------------------------------------------------------- messages

// MediaView is a file on a message.
type MediaView struct {
	URL      string `json:"url"`
	Mime     string `json:"mime"`
	Name     string `json:"name,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Voice    bool   `json:"voice,omitempty"`
	Animated bool   `json:"animated,omitempty"`
	Failed   string `json:"failed,omitempty"`
}

// SenderView says who wrote a message.
type SenderView struct {
	Kind          string `json:"kind"` // customer | agent | bot | automation | system
	UserID        uint   `json:"userId,omitempty"`
	Name          string `json:"name,omitempty"`
	HasAvatar     bool   `json:"hasAvatar,omitempty"`
	AvatarVersion int64  `json:"avatarVersion,omitempty"`
	Label         string `json:"label,omitempty"`
}

// ReplyView is the quoted message above a reply.
type ReplyView struct {
	ID     uint   `json:"id"`
	Kind   string `json:"kind"`
	Body   string `json:"body"`
	Sender string `json:"sender"`
}

// ReactionView is one side's reaction to a message.
type ReactionView struct {
	Emoji string `json:"emoji"`
	Ours  bool   `json:"ours"`
	By    string `json:"by,omitempty"`
}

// MessageView is one bubble, note or history line.
type MessageView struct {
	ID             uint            `json:"id"`
	ConversationID uint            `json:"conversationId"`
	TicketID       *uint           `json:"ticketId,omitempty"`
	ClientID       string          `json:"clientId,omitempty"`
	Direction      string          `json:"direction"`
	Kind           string          `json:"kind"`
	Body           string          `json:"body"`
	Media          *MediaView      `json:"media,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	ReplyTo        *ReplyView      `json:"replyTo,omitempty"`
	Status         string          `json:"status"`
	ErrorText      string          `json:"errorText,omitempty"`
	Sender         SenderView      `json:"sender"`
	Reactions      []ReactionView  `json:"reactions,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	SentAt         *time.Time      `json:"sentAt,omitempty"`
	DeliveredAt    *time.Time      `json:"deliveredAt,omitempty"`
	ReadAt         *time.Time      `json:"readAt,omitempty"`
}

func reactionTarget(m *models.WAMessage) (string, string) {
	if m.Payload == nil {
		return "", ""
	}
	var in struct {
		MessageID string `json:"messageId"`
		Emoji     string `json:"emoji"`
		Reaction  struct {
			MessageID string `json:"message_id"`
			Emoji     string `json:"emoji"`
		} `json:"reaction"`
	}
	_ = json.Unmarshal([]byte(*m.Payload), &in)
	if in.MessageID != "" {
		return in.MessageID, in.Emoji
	}
	return in.Reaction.MessageID, in.Reaction.Emoji
}

func (s *Service) messageViews(ctx context.Context, list []models.WAMessage) ([]MessageView, error) {
	if len(list) == 0 {
		return []MessageView{}, nil
	}
	var userIDs []uint
	var wamids []string
	var replyTo []string
	convs := map[uint]bool{}
	for _, m := range list {
		if m.SenderUserID != nil {
			userIDs = append(userIDs, *m.SenderUserID)
		}
		if m.WAMID != nil {
			wamids = append(wamids, *m.WAMID)
		}
		if m.ReplyToWAMID != nil {
			replyTo = append(replyTo, *m.ReplyToWAMID)
		}
		convs[m.ConversationID] = true
	}
	// Reactions: the latest one from each side counts; an empty one
	// takes it back.
	reacts := map[string]map[bool]ReactionView{}
	if len(wamids) > 0 {
		var convIDs []uint
		for id := range convs {
			convIDs = append(convIDs, id)
		}
		var rs []models.WAMessage
		_ = s.db.WithContext(ctx).Where("conversation_id IN ? AND kind = 'reaction'", convIDs).Order("id").Find(&rs).Error
		want := map[string]bool{}
		for _, w := range wamids {
			want[w] = true
		}
		for i := range rs {
			target, emoji := reactionTarget(&rs[i])
			if !want[target] {
				continue
			}
			ours := rs[i].Direction == "out"
			if reacts[target] == nil {
				reacts[target] = map[bool]ReactionView{}
			}
			if emoji == "" {
				delete(reacts[target], ours)
				continue
			}
			rv := ReactionView{Emoji: emoji, Ours: ours}
			if rs[i].SenderUserID != nil {
				userIDs = append(userIDs, *rs[i].SenderUserID)
				rv.By = fmt.Sprint(*rs[i].SenderUserID)
			}
			reacts[target][ours] = rv
		}
	}
	quoted := map[string]*models.WAMessage{}
	if len(replyTo) > 0 {
		var qs []models.WAMessage
		_ = s.db.WithContext(ctx).Where("wamid IN ?", replyTo).Find(&qs).Error
		for i := range qs {
			quoted[*qs[i].WAMID] = &qs[i]
			if qs[i].SenderUserID != nil {
				userIDs = append(userIDs, *qs[i].SenderUserID)
			}
		}
	}
	people := s.people(ctx, userIDs)
	senderName := func(m *models.WAMessage) string {
		switch {
		case m.SenderUserID != nil:
			return people[*m.SenderUserID].Name
		case m.Direction == "in":
			return "Müşteri"
		case m.SenderKind == "bot":
			return "Chatbot"
		default:
			return "Otomatik mesaj"
		}
	}
	out := make([]MessageView, 0, len(list))
	for i := range list {
		m := &list[i]
		v := MessageView{ID: m.ID, ConversationID: m.ConversationID, TicketID: m.TicketID, Direction: m.Direction, Kind: m.Kind, Body: m.Body,
			Status: m.Status, ErrorText: m.ErrorText, CreatedAt: m.CreatedAt, SentAt: m.SentAt, DeliveredAt: m.DeliveredAt, ReadAt: m.ReadAt}
		if m.ClientID != nil {
			v.ClientID = *m.ClientID
		}
		if m.Payload != nil && m.Kind != "text" {
			v.Payload = json.RawMessage(*m.Payload)
		}
		if m.Media != nil {
			var ref MediaRef
			if json.Unmarshal([]byte(*m.Media), &ref) == nil {
				v.Media = &MediaView{URL: fmt.Sprintf("/api/v1/wa/media/%d", m.ID), Mime: ref.Mime, Name: ref.Name, Size: ref.Size, Voice: ref.Voice, Animated: ref.Animated, Failed: ref.Failed}
			}
		}
		v.Sender = SenderView{Kind: m.SenderKind, Label: m.SenderLabel}
		if m.SenderUserID != nil {
			p := people[*m.SenderUserID]
			v.Sender.UserID, v.Sender.Name, v.Sender.HasAvatar, v.Sender.AvatarVersion = p.ID, p.Name, p.HasAvatar, p.AvatarVersion
		}
		if m.ReplyToWAMID != nil {
			if q := quoted[*m.ReplyToWAMID]; q != nil {
				v.ReplyTo = &ReplyView{ID: q.ID, Kind: q.Kind, Body: preview(q), Sender: senderName(q)}
			} else {
				v.ReplyTo = &ReplyView{Body: "Eski bir mesaj", Sender: ""}
			}
		}
		if m.WAMID != nil {
			for _, side := range []bool{false, true} {
				if r, ok := reacts[*m.WAMID][side]; ok {
					if r.By != "" {
						var id uint
						fmt.Sscan(r.By, &id)
						r.By = people[id].Name
					}
					v.Reactions = append(v.Reactions, r)
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// ---------------------------------------------------------------- listing

// ListResult is the inbox list, with the rows the person no longer sees.
type ListResult struct {
	Items   []ConversationView `json:"items"`
	Hidden  []uint             `json:"hidden"`
	Version int64              `json:"version"`
	Me      uint               `json:"me"`
}

// Conversations lists what a person sees. With since, only what changed
// after that version (for catching up after a lost connection).
func (s *Service) Conversations(ctx context.Context, actorID uint, since int64) (*ListResult, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Model(&models.WAConversation{}).Where("ticket_id IS NOT NULL")
	if !v.can(enums.WAViewAll) {
		var chans []uint
		for id := range v.channels {
			chans = append(chans, id)
		}
		if len(chans) == 0 {
			return &ListResult{Items: []ConversationView{}, Hidden: []uint{}, Me: actorID}, nil
		}
		q = q.Where("channel_id IN ?", chans)
	}
	if since > 0 {
		q = q.Where("version > ?", since).Order("version").Limit(1000)
	} else {
		q = q.Where(`ticket_id IN (SELECT id FROM wa_tickets WHERE status <> 'resolved' OR resolved_at > now() - interval '3 days')`).
			Order("last_message_at DESC NULLS LAST").Limit(1500)
	}
	var convs []models.WAConversation
	if err := q.Find(&convs).Error; err != nil {
		return nil, errs.Internal(err)
	}
	visible, hidden := s.filterVisible(ctx, v, convs)
	items, err := s.summaries(ctx, visible)
	if err != nil {
		return nil, errs.Internal(err)
	}
	var top int64
	_ = s.db.WithContext(ctx).Raw("SELECT COALESCE(max(version), 0) FROM wa_conversations").Scan(&top).Error
	return &ListResult{Items: items, Hidden: hidden, Version: top, Me: actorID}, nil
}

func (s *Service) filterVisible(ctx context.Context, v *viewer, convs []models.WAConversation) ([]models.WAConversation, []uint) {
	var ticketIDs []uint
	for _, c := range convs {
		if c.TicketID != nil {
			ticketIDs = append(ticketIDs, *c.TicketID)
		}
	}
	tickets := map[uint]*models.WATicket{}
	parts := map[uint]map[uint]bool{}
	if len(ticketIDs) > 0 {
		var list []models.WATicket
		_ = s.db.WithContext(ctx).Where("id IN ?", ticketIDs).Find(&list).Error
		for i := range list {
			tickets[list[i].ID] = &list[i]
		}
		var ps []models.WAParticipant
		_ = s.db.WithContext(ctx).Where("ticket_id IN ?", ticketIDs).Find(&ps).Error
		for _, p := range ps {
			if parts[p.TicketID] == nil {
				parts[p.TicketID] = map[uint]bool{}
			}
			parts[p.TicketID][p.UserID] = true
		}
	}
	var visible []models.WAConversation
	hidden := []uint{}
	for _, c := range convs {
		if c.TicketID != nil && v.seesTicket(tickets[*c.TicketID], parts[*c.TicketID]) {
			visible = append(visible, c)
		} else {
			hidden = append(hidden, c.ID)
		}
	}
	return visible, hidden
}

// ResolvedPage lists older resolved conversations, newest first, with an
// optional search on the customer's name or number.
func (s *Service) ResolvedPage(ctx context.Context, actorID uint, before time.Time, search string) ([]ConversationView, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Model(&models.WAConversation{}).
		Where("ticket_id IN (SELECT id FROM wa_tickets WHERE status = 'resolved')").
		Where("last_message_at < ?", before)
	if !v.can(enums.WAViewAll) {
		var chans []uint
		for id := range v.channels {
			chans = append(chans, id)
		}
		if len(chans) == 0 {
			return []ConversationView{}, nil
		}
		q = q.Where("channel_id IN ?", chans)
	}
	if t := strings.TrimSpace(search); t != "" {
		like := "%" + strings.ToLower(t) + "%"
		q = q.Where("contact_id IN (SELECT id FROM wa_contacts WHERE lower(name) LIKE ? OR lower(profile_name) LIKE ? OR wa_id LIKE ?)", like, like, "%"+digitsOnly(t)+"%")
	}
	var convs []models.WAConversation
	if err := q.Order("last_message_at DESC").Limit(60).Find(&convs).Error; err != nil {
		return nil, errs.Internal(err)
	}
	visible, _ := s.filterVisible(ctx, v, convs)
	return s.summaries(ctx, visible)
}

// Conversation returns one row, for opening a chat from a link.
func (s *Service) Conversation(ctx context.Context, actorID, id uint) (*ConversationView, error) {
	_, conv, _, err := s.reachable(ctx, actorID, id)
	if err != nil {
		return nil, err
	}
	views, err := s.summaries(ctx, []models.WAConversation{*conv})
	if err != nil || len(views) == 0 {
		return nil, errs.Internal(err)
	}
	return &views[0], nil
}

// Messages returns a page of a conversation: the latest by default, or
// before/after a message, or around one (for jumping to a search hit).
func (s *Service) Messages(ctx context.Context, actorID, conversationID, before, after, around uint) ([]MessageView, error) {
	if _, _, _, err := s.reachable(ctx, actorID, conversationID); err != nil {
		return nil, err
	}
	const page = 60
	q := s.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction'", conversationID)
	var list []models.WAMessage
	switch {
	case around > 0:
		var older, newer []models.WAMessage
		_ = s.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction' AND id <= ?", conversationID, around).Order("id DESC").Limit(page / 2).Find(&older).Error
		_ = s.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction' AND id > ?", conversationID, around).Order("id").Limit(page / 2).Find(&newer).Error
		for i := len(older) - 1; i >= 0; i-- {
			list = append(list, older[i])
		}
		list = append(list, newer...)
	case after > 0:
		if err := q.Where("id > ?", after).Order("id").Limit(page).Find(&list).Error; err != nil {
			return nil, errs.Internal(err)
		}
	default:
		if before > 0 {
			q = q.Where("id < ?", before)
		}
		var desc []models.WAMessage
		if err := q.Order("id DESC").Limit(page).Find(&desc).Error; err != nil {
			return nil, errs.Internal(err)
		}
		for i := len(desc) - 1; i >= 0; i-- {
			list = append(list, desc[i])
		}
	}
	return s.messageViews(ctx, list)
}

// SearchHit is a message found by the inbox search.
type SearchHit struct {
	ConversationID uint      `json:"conversationId"`
	MessageID      uint      `json:"messageId"`
	Contact        string    `json:"contact"`
	Snippet        string    `json:"snippet"`
	At             time.Time `json:"at"`
}

// Search finds messages containing a word in conversations the person sees.
func (s *Service) Search(ctx context.Context, actorID uint, text string, conversationID uint) ([]SearchHit, error) {
	v, err := s.viewerOf(ctx, actorID)
	if err != nil {
		return nil, err
	}
	t := strings.TrimSpace(text)
	if len([]rune(t)) < 2 {
		return []SearchHit{}, nil
	}
	q := s.db.WithContext(ctx).Model(&models.WAMessage{}).Where("body ILIKE ? AND direction IN ('in','out','note')", "%"+t+"%")
	if conversationID > 0 {
		if _, _, _, err := s.reachable(ctx, actorID, conversationID); err != nil {
			return nil, err
		}
		q = q.Where("conversation_id = ?", conversationID)
	}
	var list []models.WAMessage
	if err := q.Order("id DESC").Limit(200).Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	convIDs := map[uint]bool{}
	for _, m := range list {
		convIDs[m.ConversationID] = true
	}
	var ids []uint
	for id := range convIDs {
		ids = append(ids, id)
	}
	var convs []models.WAConversation
	_ = s.db.WithContext(ctx).Where("id IN ?", ids).Find(&convs).Error
	visible, _ := s.filterVisible(ctx, v, convs)
	ok := map[uint]models.WAConversation{}
	var contactIDs []uint
	for _, c := range visible {
		ok[c.ID] = c
		contactIDs = append(contactIDs, c.ContactID)
	}
	var contacts []models.WAContact
	_ = s.db.WithContext(ctx).Where("id IN ?", contactIDs).Find(&contacts).Error
	names := map[uint]string{}
	for i := range contacts {
		names[contacts[i].ID] = contactView(&contacts[i]).Display
	}
	out := []SearchHit{}
	for _, m := range list {
		c, seen := ok[m.ConversationID]
		if !seen {
			continue
		}
		out = append(out, SearchHit{ConversationID: m.ConversationID, MessageID: m.ID, Contact: names[c.ContactID], Snippet: snippet(m.Body, t), At: m.CreatedAt})
		if len(out) >= 80 {
			break
		}
	}
	return out, nil
}

func snippet(body, word string) string {
	r := []rune(body)
	i := strings.Index(strings.ToLower(body), strings.ToLower(word))
	if i < 0 || len(r) <= 120 {
		if len(r) > 120 {
			return string(r[:120]) + "…"
		}
		return body
	}
	start := len([]rune(body[:i])) - 40
	if start < 0 {
		start = 0
	}
	end := start + 120
	if end > len(r) {
		end = len(r)
	}
	out := string(r[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(r) {
		out += "…"
	}
	return out
}
