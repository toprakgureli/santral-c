package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// ---------------------------------------------------------------- running

// liveIO carries a flow's actions out for a real customer.
type liveIO struct {
	s       *Service
	ctx     context.Context
	ch      *models.WAChannel
	conv    *models.WAConversation
	ticket  *models.WATicket
	bot     *models.WABot
	version int

	handed  bool
	team    uint
	note    string
	ended   bool
	resolve bool
}

func (o *liveIO) sendText(text string) {
	o.s.queueObject(o.ctx, o.ch, o.conv.ID, o.ticket.ID, "bot", o.bot.Name, "text", text, map[string]any{"type": "text", "text": map[string]any{"body": text}})
}

func (o *liveIO) sendMedia(kind, url string, fileID uint, fileName, caption string) {
	if fileID > 0 {
		metaID, f, err := o.s.metaMediaFor(o.ctx, o.ch, fileID)
		if err != nil || f == nil {
			slog.WarnContext(o.ctx, "whatsapp chatbot file could not be sent", "bot", o.bot.ID, "file", fileID, "error", err)
			if t := strings.TrimSpace(caption); t != "" {
				o.sendText(t)
			}
			return
		}
		k, _ := mediaKind(f.Mime, 0)
		ref := MediaRef{MetaID: metaID, StoreID: f.StorageID, Mime: f.Mime, Name: f.Name, Size: f.Size}
		msg := &models.WAMessage{ChannelID: o.ch.ID, ConversationID: o.conv.ID, TicketID: uintPtr(o.ticket.ID), Direction: "out", Kind: k,
			SenderKind: "bot", SenderLabel: o.bot.Name, Body: strings.TrimSpace(caption), Media: strPtr(jsonString(ref)), Status: "queued", CreatedAt: time.Now()}
		if _, err := o.s.enqueue(o.ctx, o.ch, o.conv, o.ticket, msg, mediaObject(k, metaID, caption, f.Name), 0); err != nil {
			slog.WarnContext(o.ctx, "whatsapp chatbot file could not be queued", "bot", o.bot.ID, "error", err)
		}
		return
	}
	switch kind {
	case "video", "document":
	default:
		kind = "image"
	}
	obj := map[string]any{"link": url}
	if caption != "" {
		obj["caption"] = caption
	}
	o.s.queueObject(o.ctx, o.ch, o.conv.ID, o.ticket.ID, "bot", o.bot.Name, "text", strings.TrimSpace(caption+" "+url), map[string]any{"type": kind, kind: obj})
}

func (o *liveIO) sendMenu(style, text, button string, options []BotOption) {
	lines := []string{text}
	for i, opt := range options {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, opt.Label))
	}
	o.s.queueObject(o.ctx, o.ch, o.conv.ID, o.ticket.ID, "bot", o.bot.Name, "interactive", strings.Join(lines, "\n"), menuMessage(style, text, button, options))
}

func (o *liveIO) handoff(teamID uint, note string) {
	o.handed, o.team, o.note = true, teamID, note
}

func (o *liveIO) finish(resolve bool) {
	o.ended, o.resolve = true, resolve
}

func (o *liveIO) tag(tags []string, priority, category string) {
	t := o.s.ticketFresh(o.ctx, o.ticket.ID)
	if t == nil {
		return
	}
	merged := cleanTags(append(parseTags(t.Tags), tags...))
	fields := map[string]any{"tags": jsonString(merged)}
	switch priority {
	case "low", "normal", "high", "urgent":
		fields["priority"] = priority
	}
	if c := strings.TrimSpace(category); c != "" {
		fields["category"] = c
	}
	_ = o.s.db.WithContext(o.ctx).Model(&models.WATicket{}).Where("id = ?", o.ticket.ID).Updates(fields).Error
}

func (o *liveIO) callback(note string) {
	o.s.createCallback(o.ctx, o.ch, o.conv, o.ticket, note)
}

func (o *liveIO) survey() {
	o.s.sendNativeSurvey(o.ctx, o.ch, o.conv, o.ticket, "")
}

func (o *liveIO) callAPI(id uint, vars map[string]string) (map[string]any, error) {
	return o.s.callIntegration(o.ctx, id, vars)
}

func (o *liveIO) hoursOpen() bool {
	h := parseSettings(o.ch.Settings).Hours
	return !h.Enabled || h.Open(time.Now())
}

func (o *liveIO) mark(nodeID, kind string) {
	_ = o.s.db.WithContext(o.ctx).Exec("INSERT INTO wa_bot_events (bot_id, version, conversation_id, node_id, kind) VALUES (?, ?, ?, ?, ?)",
		o.bot.ID, o.version, o.conv.ID, nodeID, kind).Error
}

func (s *Service) botGraph(ctx context.Context, botID uint, version int) (*BotGraph, error) {
	var raw string
	if err := s.db.WithContext(ctx).Raw("SELECT graph FROM wa_bot_versions WHERE bot_id = ? AND version = ?", botID, version).Scan(&raw).Error; err != nil || raw == "" {
		return nil, errors.New("chatbot sürümü bulunamadı")
	}
	var g BotGraph
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// pickBot chooses the flow that greets a customer on a device: the
// after-hours flow when the device is closed, a keyword flow when the
// message matches, else the entry flow.
func (s *Service) pickBot(ctx context.Context, ch *models.WAChannel, text string) *models.WABot {
	var bots []models.WABot
	if err := s.db.WithContext(ctx).Where("active AND published_version > 0 AND channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", ch.ID)).Order("id").Find(&bots).Error; err != nil {
		return nil
	}
	h := parseSettings(ch.Settings).Hours
	closed := h.Enabled && !h.Open(time.Now())
	var entry, after, keyword *models.WABot
	low := strings.ToLower(text)
	for i := range bots {
		b := &bots[i]
		switch b.Trigger {
		case "after_hours":
			if after == nil {
				after = b
			}
		case "keyword":
			for _, k := range parseTags(b.Keywords) {
				if k = strings.ToLower(strings.TrimSpace(k)); k != "" && strings.Contains(low, k) && keyword == nil {
					keyword = b
				}
			}
		default:
			if entry == nil {
				entry = b
			}
		}
	}
	switch {
	case closed && after != nil:
		return after
	case keyword != nil:
		return keyword
	}
	return entry
}

// startBot starts a flow for a new or returning customer. It reports
// whether a flow took the conversation.
func (s *Service) startBot(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, msg *models.WAMessage) bool {
	bot := s.pickBot(ctx, ch, msg.Body)
	if bot == nil {
		return false
	}
	g, err := s.botGraph(ctx, bot.ID, bot.PublishedVersion)
	if err != nil {
		slog.WarnContext(ctx, "whatsapp chatbot could not start", "bot", bot.ID, "error", err)
		return false
	}
	contact, _ := s.contact(ctx, conv.ContactID)
	st := &botState{Vars: map[string]string{}}
	if contact != nil {
		v := contactView(contact)
		st.Vars["musteri"] = firstName(v.Display)
		st.Vars["numara"] = "+" + contact.WAID
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET status = 'bot', updated_at = now() WHERE id = ?", ticket.ID).Error; err != nil {
		return false
	}
	ticket.Status = "bot"
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO wa_bot_sessions (conversation_id, bot_id, version, node_id, vars, tries) VALUES (?, ?, ?, '', '{}', 0)
		ON CONFLICT (conversation_id) DO UPDATE SET bot_id = EXCLUDED.bot_id, version = EXCLUDED.version, node_id = '', vars = '{}', tries = 0, started_at = now(), updated_at = now()`,
		conv.ID, bot.ID, bot.PublishedVersion).Error; err != nil {
		return false
	}
	s.runStep(ctx, ch, conv, ticket, bot, bot.PublishedVersion, g, st, nil)
	return true
}

// continueBot feeds the customer's answer to their flow.
func (s *Service) continueBot(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, msg *models.WAMessage) bool {
	var sess models.WABotSession
	if err := s.db.WithContext(ctx).Where("conversation_id = ?", conv.ID).First(&sess).Error; err != nil {
		// The flow is gone (ended or timed out): a person takes over.
		s.botToHuman(ctx, ch, conv, ticket, 0, "")
		return false
	}
	var bot models.WABot
	if err := s.db.WithContext(ctx).First(&bot, sess.BotID).Error; err != nil {
		s.botToHuman(ctx, ch, conv, ticket, 0, "")
		return false
	}
	set := parseSettings(ch.Settings)
	if msg.Kind == "text" && matchesWord(msg.Body, set.HumanKeywords) {
		s.endBot(ctx, conv.ID, "handoff")
		s.botToHuman(ctx, ch, conv, ticket, 0, "Müşteri temsilciyle görüşmek istedi.")
		return true
	}
	g, err := s.botGraph(ctx, sess.BotID, sess.Version)
	if err != nil {
		s.endBot(ctx, conv.ID, "handoff")
		s.botToHuman(ctx, ch, conv, ticket, 0, "")
		return false
	}
	st := &botState{NodeID: sess.NodeID, Tries: sess.Tries, Vars: map[string]string{}}
	_ = json.Unmarshal([]byte(sess.Vars), &st.Vars)
	in := &botInput{Text: msg.Body}
	if msg.Payload != nil {
		var p struct {
			ID      string `json:"id"`
			Payload string `json:"payload"`
		}
		_ = json.Unmarshal([]byte(*msg.Payload), &p)
		in.ChoiceID = p.ID
		if in.ChoiceID == "" {
			in.ChoiceID = p.Payload
		}
	}
	s.runStep(ctx, ch, conv, ticket, &bot, sess.Version, g, st, in)
	return true
}

// runStep walks the flow and saves or ends the session.
func (s *Service) runStep(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, bot *models.WABot, version int, g *BotGraph, st *botState, in *botInput) {
	io := &liveIO{s: s, ctx: ctx, ch: ch, conv: conv, ticket: ticket, bot: bot, version: version}
	step(g, st, in, io)
	if !st.Done {
		_ = s.db.WithContext(ctx).Exec("UPDATE wa_bot_sessions SET node_id = ?, vars = ?, tries = ?, updated_at = now() WHERE conversation_id = ?",
			st.NodeID, jsonString(st.Vars), st.Tries, conv.ID).Error
		return
	}
	_ = s.db.WithContext(ctx).Exec("DELETE FROM wa_bot_sessions WHERE conversation_id = ?", conv.ID).Error
	summary := botSummary(st.Vars)
	switch {
	case io.handed:
		note := io.note
		if summary != "" {
			note = strings.TrimSpace(note + " " + summary)
		}
		s.botToHuman(ctx, ch, conv, ticket, io.team, note)
	case io.ended && io.resolve:
		s.event(ctx, nil, conv, ticket.ID, 0, bot.Name+" chatbot'u sohbeti tamamladı. "+summary)
		_ = s.resolve(ctx, conv, ticket, 0)
	default:
		s.botToHuman(ctx, ch, conv, ticket, 0, summary)
	}
}

// botSummary is what the flow collected, shown to the agent who takes
// over so the customer is not asked again.
func botSummary(vars map[string]string) string {
	var parts []string
	for k, v := range vars {
		if k == "musteri" || k == "numara" || strings.TrimSpace(v) == "" {
			continue
		}
		parts = append(parts, k+": "+v)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Chatbot'un topladıkları: " + strings.Join(parts, ", ") + "."
}

// botToHuman hands a conversation from the flow to a person.
func (s *Service) botToHuman(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, teamID uint, note string) {
	fields := map[string]any{"status": "open", "updated_at": time.Now()}
	if teamID > 0 {
		fields["team_id"] = teamID
	}
	_ = s.db.WithContext(ctx).Model(&models.WATicket{}).Where("id = ? AND status = 'bot'", ticket.ID).Updates(fields).Error
	line := "Chatbot sohbeti bir temsilciye aktardı."
	if teamID > 0 {
		var name string
		_ = s.db.WithContext(ctx).Raw("SELECT name FROM wa_teams WHERE id = ?", teamID).Scan(&name).Error
		if name != "" {
			line = "Chatbot sohbeti " + name + " ekibine aktardı."
		}
	}
	if n := strings.TrimSpace(note); n != "" {
		line += " " + n
	}
	s.event(ctx, nil, conv, ticket.ID, 0, line)
	// The waiting clock starts only now that a person is needed.
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET awaiting_since = now(), waiting_listed_at = NULL WHERE id = ?", ticket.ID).Error
	s.distribute(ctx, ch, ticket.ID)
	s.publish(ctx, conv.ID, nil, nil)
}

// endBot stops a conversation's flow, if any.
func (s *Service) endBot(ctx context.Context, conversationID uint, kind string) {
	var sess models.WABotSession
	if s.db.WithContext(ctx).Where("conversation_id = ?", conversationID).First(&sess).Error != nil {
		return
	}
	_ = s.db.WithContext(ctx).Exec("DELETE FROM wa_bot_sessions WHERE conversation_id = ?", conversationID).Error
	_ = s.db.WithContext(ctx).Exec("INSERT INTO wa_bot_events (bot_id, version, conversation_id, node_id, kind) VALUES (?, ?, ?, ?, ?)", sess.BotID, sess.Version, conversationID, sess.NodeID, kind).Error
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET status = 'open' WHERE status = 'bot' AND conversation_id = ?", conversationID).Error
}

// sweepBots ends flows whose customer went quiet; the ticket is closed
// without a survey and opens again when they write.
func (s *Service) sweepBots(ctx context.Context) {
	var rows []struct {
		ConversationID uint
		ChannelID      uint
		UpdatedAt      time.Time
	}
	_ = s.db.WithContext(ctx).Raw(`SELECT s.conversation_id, c.channel_id, s.updated_at FROM wa_bot_sessions s JOIN wa_conversations c ON c.id = s.conversation_id
		WHERE s.updated_at < now() - interval '5 minutes'`).Scan(&rows).Error
	for _, r := range rows {
		ch, err := s.channel(ctx, r.ChannelID)
		if err != nil {
			continue
		}
		if time.Since(r.UpdatedAt) < time.Duration(parseSettings(ch.Settings).BotTimeoutMinutes)*time.Minute {
			continue
		}
		conv, ticket, err := s.loadConv(ctx, r.ConversationID)
		if err != nil || ticket == nil {
			continue
		}
		s.endBot(ctx, conv.ID, "timeout")
		s.event(ctx, nil, conv, ticket.ID, 0, "Müşteri chatbot'ta cevap vermeyi bıraktı, sohbet kapatıldı. Tekrar yazarsa yeniden açılır.")
		_ = s.resolve(ctx, conv, ticket, 0)
	}
}

// ---------------------------------------------------------------- managing

// BotView is a chatbot for the panel.
type BotView struct {
	ID               uint            `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Active           bool            `json:"active"`
	ChannelIDs       []uint          `json:"channelIds"`
	Trigger          string          `json:"trigger"`
	Keywords         []string        `json:"keywords"`
	Draft            json.RawMessage `json:"draft"`
	PublishedVersion int             `json:"publishedVersion"`
	PublishedAt      *time.Time      `json:"publishedAt,omitempty"`
	DraftChanged     bool            `json:"draftChanged"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

func parseIDs(raw string) []uint {
	var out []uint
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		out = []uint{}
	}
	return out
}

func (s *Service) botView(ctx context.Context, b *models.WABot) BotView {
	v := BotView{ID: b.ID, Name: b.Name, Description: b.Description, Active: b.Active, ChannelIDs: parseIDs(b.ChannelIDs), Trigger: b.Trigger,
		Keywords: parseTags(b.Keywords), Draft: json.RawMessage(b.Draft), PublishedVersion: b.PublishedVersion, PublishedAt: b.PublishedAt, UpdatedAt: b.UpdatedAt}
	if b.PublishedVersion == 0 {
		v.DraftChanged = true
	} else {
		var pub string
		_ = s.db.WithContext(ctx).Raw("SELECT graph::text FROM wa_bot_versions WHERE bot_id = ? AND version = ?", b.ID, b.PublishedVersion).Scan(&pub).Error
		var a, c any
		_ = json.Unmarshal([]byte(pub), &a)
		_ = json.Unmarshal([]byte(b.Draft), &c)
		v.DraftChanged = jsonString(a) != jsonString(c)
	}
	return v
}

func (s *Service) botManager(ctx context.Context, actorID uint) (*models.User, error) {
	u, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !u.Can(enums.WABotManage) && !u.Can(enums.WABotPublish) {
		return nil, errs.Forbidden("Chatbot'ları görme yetkiniz yok.")
	}
	return u, nil
}

// Bots lists the chatbots.
func (s *Service) Bots(ctx context.Context, actorID uint) ([]BotView, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	var list []models.WABot
	if err := s.db.WithContext(ctx).Order("id").Find(&list).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]BotView, 0, len(list))
	for i := range list {
		out = append(out, s.botView(ctx, &list[i]))
	}
	return out, nil
}

// Bot returns one chatbot.
func (s *Service) Bot(ctx context.Context, actorID, id uint) (*BotView, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	var b models.WABot
	if err := s.db.WithContext(ctx).First(&b, id).Error; err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	v := s.botView(ctx, &b)
	return &v, nil
}

// BotInput is the chatbot's card: name, when it runs and on which devices.
type BotInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Trigger     string   `json:"trigger"`
	Keywords    []string `json:"keywords"`
	ChannelIDs  []uint   `json:"channelIds"`
	Active      *bool    `json:"active"`
}

func starterGraph() string {
	g := BotGraph{
		Nodes: []BotNode{
			{ID: "start", Type: "start", X: 80, Y: 200},
			{ID: "welcome", Type: "message", X: 320, Y: 180, Data: BotData{Text: "Merhaba {musteri}, size nasıl yardımcı olabiliriz?"}},
			{ID: "menu", Type: "menu", X: 600, Y: 160, Data: BotData{Text: "Lütfen bir konu seçin.", Style: "buttons", Options: []BotOption{{ID: "o1", Label: "Destek"}, {ID: "o2", Label: "Satış"}, {ID: "o3", Label: "Temsilci"}}}},
			{ID: "h", Type: "handoff", X: 900, Y: 200, Data: BotData{Text: "Sizi bir temsilcimize aktarıyorum, en kısa sürede dönüş yapacağız."}},
		},
		Edges: []BotEdge{
			{ID: "e1", From: "start", Port: "next", To: "welcome"},
			{ID: "e2", From: "welcome", Port: "next", To: "menu"},
			{ID: "e3", From: "menu", Port: "o1", To: "h"},
			{ID: "e4", From: "menu", Port: "o2", To: "h"},
			{ID: "e5", From: "menu", Port: "o3", To: "h"},
		},
	}
	return jsonString(g)
}

func normTrigger(t string) string {
	switch t {
	case "after_hours", "keyword":
		return t
	}
	return "entry"
}

// checkBotConflict keeps one entry flow and one after-hours flow per
// device, so it is never unclear which one answers.
func (s *Service) checkBotConflict(ctx context.Context, id uint, trigger string, channels []uint, active bool) error {
	if !active || trigger == "keyword" {
		return nil
	}
	var others []models.WABot
	_ = s.db.WithContext(ctx).Where("active AND trigger = ? AND id <> ?", trigger, id).Find(&others).Error
	for _, o := range others {
		for _, a := range parseIDs(o.ChannelIDs) {
			for _, b := range channels {
				if a == b {
					var name string
					_ = s.db.WithContext(ctx).Raw("SELECT name FROM wa_channels WHERE id = ?", a).Scan(&name).Error
					return errs.Conflict(fmt.Sprintf("%s cihazında zaten açık bir %s var: %s. Önce onu kapatın ya da cihazdan çıkarın.", name, triggerWord(trigger), o.Name), nil)
				}
			}
		}
	}
	return nil
}

func triggerWord(t string) string {
	if t == "after_hours" {
		return "mesai dışı chatbot'u"
	}
	return "karşılama chatbot'u"
}

// CreateBot makes a new chatbot with a small starter flow. It is off and
// on no device until someone publishes it and picks devices.
func (s *Service) CreateBot(ctx context.Context, actorID uint, in BotInput) (*BotView, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot oluşturma yetkiniz yok."); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errs.Invalid("Chatbot'a bir ad verin.", nil)
	}
	b := &models.WABot{Name: name, Description: strings.TrimSpace(in.Description), Trigger: normTrigger(in.Trigger), Keywords: jsonString(cleanTags(in.Keywords)),
		ChannelIDs: "[]", Draft: starterGraph(), CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(b).Error; err != nil {
		return nil, errs.Internal(err)
	}
	v := s.botView(ctx, b)
	return &v, nil
}

// UpdateBot changes the card. Turning it on or choosing devices changes
// what customers meet, so that needs the publish permission.
func (s *Service) UpdateBot(ctx context.Context, actorID, id uint, in BotInput) (*BotView, error) {
	u, err := s.botManager(ctx, actorID)
	if err != nil {
		return nil, err
	}
	var b models.WABot
	if err := s.db.WithContext(ctx).First(&b, id).Error; err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	fields := map[string]any{"updated_at": time.Now()}
	if n := strings.TrimSpace(in.Name); n != "" {
		fields["name"] = n
	}
	fields["description"] = strings.TrimSpace(in.Description)
	trigger := normTrigger(in.Trigger)
	fields["trigger"] = trigger
	fields["keywords"] = jsonString(cleanTags(in.Keywords))
	channels := in.ChannelIDs
	if channels == nil {
		channels = []uint{}
	}
	active := b.Active
	if in.Active != nil {
		active = *in.Active
	}
	liveChange := active != b.Active || jsonString(channels) != jsonString(parseIDs(b.ChannelIDs)) || trigger != b.Trigger
	if liveChange && !u.Can(enums.WABotPublish) {
		return nil, errs.Forbidden("Chatbot'u açıp kapatma ya da cihaz seçme yetkiniz yok.")
	}
	if active && b.PublishedVersion == 0 {
		return nil, errs.Invalid("Chatbot'u açmadan önce yayına alın.", nil)
	}
	if err := s.checkBotConflict(ctx, id, trigger, channels, active); err != nil {
		return nil, err
	}
	fields["channel_ids"] = jsonString(channels)
	fields["active"] = active
	if !u.Can(enums.WABotManage) {
		delete(fields, "name")
		delete(fields, "description")
		delete(fields, "keywords")
	}
	if err := s.db.WithContext(ctx).Model(&models.WABot{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return nil, errs.Internal(err)
	}
	return s.Bot(ctx, actorID, id)
}

// SaveDraft stores the drawing without touching what customers meet.
func (s *Service) SaveDraft(ctx context.Context, actorID, id uint, g BotGraph) (*BotView, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkiniz yok."); err != nil {
		return nil, err
	}
	if len(g.Nodes) > 300 {
		return nil, errs.Invalid("Bir akış en fazla 300 kutu olabilir.", nil)
	}
	if err := s.db.WithContext(ctx).Model(&models.WABot{}).Where("id = ?", id).Updates(map[string]any{"draft": jsonString(g), "updated_at": time.Now()}).Error; err != nil {
		return nil, errs.Internal(err)
	}
	return s.Bot(ctx, actorID, id)
}

// PublishResult is a publish attempt; problems are in words.
type PublishResult struct {
	Bot      *BotView `json:"bot,omitempty"`
	Problems []string `json:"problems,omitempty"`
}

// PublishBot makes the draft what customers meet, after checking it.
func (s *Service) PublishBot(ctx context.Context, actorID, id uint) (*PublishResult, error) {
	if _, err := s.require(ctx, actorID, enums.WABotPublish, "Chatbot yayınlama yetkiniz yok."); err != nil {
		return nil, err
	}
	var b models.WABot
	if err := s.db.WithContext(ctx).First(&b, id).Error; err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	var g BotGraph
	if err := json.Unmarshal([]byte(b.Draft), &g); err != nil {
		return nil, errs.Invalid("Akış okunamadı.", err)
	}
	if p := g.Validate(); len(p) > 0 {
		return &PublishResult{Problems: p}, nil
	}
	next := b.PublishedVersion + 1
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO wa_bot_versions (bot_id, version, graph, published_by) VALUES (?, ?, ?, ?)", id, next, b.Draft, actorID).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE wa_bots SET published_version = ?, published_at = now(), updated_at = now() WHERE id = ?", next, id).Error
	})
	if err != nil {
		return nil, errs.Internal(err)
	}
	v, err := s.Bot(ctx, actorID, id)
	if err != nil {
		return nil, err
	}
	return &PublishResult{Bot: v}, nil
}

// BotVersion is one published version.
type BotVersion struct {
	Version     int       `json:"version"`
	PublishedBy string    `json:"publishedBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// BotVersions lists a chatbot's published versions, newest first.
func (s *Service) BotVersions(ctx context.Context, actorID, id uint) ([]BotVersion, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	var out []BotVersion
	if err := s.db.WithContext(ctx).Raw(`SELECT v.version, COALESCE(u.name, '') AS published_by, v.created_at FROM wa_bot_versions v
		LEFT JOIN users u ON u.id = v.published_by WHERE v.bot_id = ? ORDER BY v.version DESC`, id).Scan(&out).Error; err != nil {
		return nil, errs.Internal(err)
	}
	if out == nil {
		out = []BotVersion{}
	}
	return out, nil
}

// RestoreVersion puts an older version back into the draft.
func (s *Service) RestoreVersion(ctx context.Context, actorID, id uint, version int) (*BotView, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkiniz yok."); err != nil {
		return nil, err
	}
	var raw string
	if err := s.db.WithContext(ctx).Raw("SELECT graph::text FROM wa_bot_versions WHERE bot_id = ? AND version = ?", id, version).Scan(&raw).Error; err != nil || raw == "" {
		return nil, errs.NotFound("Sürüm bulunamadı.")
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_bots SET draft = ?, updated_at = now() WHERE id = ?", raw, id).Error; err != nil {
		return nil, errs.Internal(err)
	}
	return s.Bot(ctx, actorID, id)
}

// CopyBot makes an independent copy, for another device or a variant.
func (s *Service) CopyBot(ctx context.Context, actorID, id uint, name string, channelIDs []uint) (*BotView, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot kopyalama yetkiniz yok."); err != nil {
		return nil, err
	}
	var src models.WABot
	if err := s.db.WithContext(ctx).First(&src, id).Error; err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = src.Name + " (kopya)"
	}
	if channelIDs == nil {
		channelIDs = []uint{}
	}
	b := &models.WABot{Name: name, Description: src.Description, Trigger: src.Trigger, Keywords: src.Keywords, ChannelIDs: jsonString(channelIDs),
		Draft: src.Draft, CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(b).Error; err != nil {
		return nil, errs.Internal(err)
	}
	v := s.botView(ctx, b)
	return &v, nil
}

// DeleteBot removes a chatbot; customers inside it go to a person.
func (s *Service) DeleteBot(ctx context.Context, actorID, id uint) error {
	if _, err := s.require(ctx, actorID, enums.WABotPublish, "Chatbot silme yetkiniz yok."); err != nil {
		return err
	}
	var convs []uint
	_ = s.db.WithContext(ctx).Raw("SELECT conversation_id FROM wa_bot_sessions WHERE bot_id = ?", id).Scan(&convs).Error
	if err := s.db.WithContext(ctx).Delete(&models.WABot{}, id).Error; err != nil {
		return errs.Internal(err)
	}
	for _, c := range convs {
		conv, ticket, err := s.loadConv(ctx, c)
		if err != nil || ticket == nil {
			continue
		}
		if ch, err := s.channel(ctx, conv.ChannelID); err == nil {
			s.botToHuman(ctx, ch, conv, ticket, 0, "Chatbot silindi.")
		}
	}
	return nil
}

// SimInput is one turn in the test screen.
type SimInput struct {
	Graph     BotGraph          `json:"graph"`
	NodeID    string            `json:"nodeId"`
	Vars      map[string]string `json:"vars"`
	Tries     int               `json:"tries"`
	Text      string            `json:"text"`
	ChoiceID  string            `json:"choiceId"`
	Start     bool              `json:"start"`
	HoursOpen bool              `json:"hoursOpen"`
}

// SimResult is what the flow did and where it stands.
type SimResult struct {
	Outputs []SimOutput       `json:"outputs"`
	NodeID  string            `json:"nodeId"`
	Vars    map[string]string `json:"vars"`
	Tries   int               `json:"tries"`
	Done    bool              `json:"done"`
}

// Simulate runs a turn of a flow without sending anything to anyone.
// Outside systems are really asked, so their answers can be checked.
func (s *Service) Simulate(ctx context.Context, actorID uint, in SimInput) (*SimResult, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	st := &botState{NodeID: in.NodeID, Vars: in.Vars, Tries: in.Tries}
	if st.Vars == nil {
		st.Vars = map[string]string{"musteri": "Ayşe", "numara": "+905xxxxxxxxx"}
	}
	io := &simIO{hours: in.HoursOpen, api: func(id uint, vars map[string]string) (map[string]any, error) {
		return s.callIntegration(ctx, id, vars)
	}}
	var input *botInput
	if in.Start {
		st.NodeID = ""
	} else {
		input = &botInput{Text: in.Text, ChoiceID: in.ChoiceID}
	}
	step(&in.Graph, st, input, io)
	if io.Out == nil {
		io.Out = []SimOutput{}
	}
	return &SimResult{Outputs: io.Out, NodeID: st.NodeID, Vars: st.Vars, Tries: st.Tries, Done: st.Done}, nil
}

// BotStats is a flow's use: how many entered each box and how it ended.
type BotStats struct {
	Started  int64            `json:"started"`
	Handoffs int64            `json:"handoffs"`
	Ended    int64            `json:"ended"`
	Timeouts int64            `json:"timeouts"`
	Nodes    map[string]int64 `json:"nodes"`
	Fails    map[string]int64 `json:"fails"`
	Drops    map[string]int64 `json:"drops"`
}

// BotReport counts a flow's use over the last days.
func (s *Service) BotReport(ctx context.Context, actorID, id uint, days int) (*BotStats, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	if days <= 0 || days > 365 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	var rows []struct {
		NodeID string
		Kind   string
		N      int64
	}
	if err := s.db.WithContext(ctx).Raw("SELECT node_id, kind, count(*) AS n FROM wa_bot_events WHERE bot_id = ? AND created_at >= ? GROUP BY node_id, kind", id, since).Scan(&rows).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out := &BotStats{Nodes: map[string]int64{}, Fails: map[string]int64{}, Drops: map[string]int64{}}
	var g BotGraph
	var draft string
	_ = s.db.WithContext(ctx).Raw("SELECT draft::text FROM wa_bots WHERE id = ?", id).Scan(&draft).Error
	_ = json.Unmarshal([]byte(draft), &g)
	start := ""
	if n := g.start(); n != nil {
		start = n.ID
	}
	for _, r := range rows {
		switch r.Kind {
		case "enter":
			out.Nodes[r.NodeID] += r.N
			if r.NodeID == start {
				out.Started += r.N
			}
		case "fail":
			out.Fails[r.NodeID] += r.N
		case "handoff":
			out.Handoffs += r.N
		case "end":
			out.Ended += r.N
		case "timeout":
			out.Timeouts += r.N
			out.Drops[r.NodeID] += r.N
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- outside systems

// callIntegration asks an outside system with the flow's variables and
// returns its JSON answer.
func (s *Service) callIntegration(ctx context.Context, id uint, vars map[string]string) (map[string]any, error) {
	var in models.WAIntegration
	if err := s.db.WithContext(ctx).First(&in, id).Error; err != nil {
		return nil, errors.New("dış sorgu bulunamadı")
	}
	url := fillVars(in.URL, vars)
	var body io.Reader
	if in.Method != "GET" && strings.TrimSpace(in.Body) != "" {
		body = bytes.NewReader([]byte(fillVars(in.Body, vars)))
	}
	timeout := time.Duration(in.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 8 * time.Second
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(c, in.Method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if raw := s.open(in.HeadersEnc); raw != "" {
		var hs map[string]string
		if json.Unmarshal([]byte(raw), &hs) == nil {
			for k, v := range hs {
				req.Header.Set(k, fillVars(v, vars))
			}
		}
	}
	resp, err := outsideClient.Do(req)
	if err != nil {
		return nil, explainOutside(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("yanıt %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		var list []any
		if json.Unmarshal(data, &list) == nil {
			return map[string]any{"items": list}, nil
		}
		return nil, errors.New("yanıt JSON değil")
	}
	return out, nil
}
