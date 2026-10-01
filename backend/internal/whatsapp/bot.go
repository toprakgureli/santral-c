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

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/device"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/flow"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/hours"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/outside"

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

func (o *liveIO) SendText(text string) {
	o.s.queueObject(o.ctx, o.ch, o.conv.ID, o.ticket.ID, "bot", o.bot.Name, "text", text, map[string]any{"type": "text", "text": map[string]any{"body": text}})
}

func (o *liveIO) SendMedia(kind, url string, fileID uint, fileName, caption string) {
	if fileID > 0 {
		metaID, f, err := o.s.metaMediaFor(o.ctx, o.ch, fileID)
		if err != nil || f == nil {
			slog.WarnContext(o.ctx, "whatsapp chatbot file could not be sent", "bot", o.bot.ID, "file", fileID, "error", err)
			if t := strings.TrimSpace(caption); t != "" {
				o.SendText(t)
			}
			return
		}
		k := storedKind(f.Mime)
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

func (o *liveIO) SendMenu(style, text, button string, options []flow.Option) {
	lines := []string{text}
	for i, opt := range options {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, opt.Label))
	}
	o.s.queueObject(o.ctx, o.ch, o.conv.ID, o.ticket.ID, "bot", o.bot.Name, "interactive", strings.Join(lines, "\n"), flow.MenuMessage(style, text, button, options))
}

func (o *liveIO) Handoff(teamID uint, note string) {
	o.handed, o.team, o.note = true, teamID, note
}

func (o *liveIO) Finish(resolve bool) {
	o.ended, o.resolve = true, resolve
}

func (o *liveIO) Tag(tags []string, priority, category string) {
	t := o.s.repo.Ticket(o.ctx, o.ticket.ID)
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
	_ = o.s.repo.UpdateTicket(o.ctx, o.ticket.ID, fields)
}

func (o *liveIO) Callback(note string) {
	o.s.createCallback(o.ctx, o.ch, o.conv, o.ticket, note)
}

func (o *liveIO) Survey() {
	o.s.sendNativeSurvey(o.ctx, o.ch, o.conv, o.ticket, "")
}

func (o *liveIO) CallAPI(id uint, vars map[string]string) (map[string]any, error) {
	return o.s.callIntegration(o.ctx, id, vars)
}

func (o *liveIO) Now() time.Time { return time.Now() }

func (o *liveIO) HoursOpen() bool {
	h := device.Parse(o.ch.Settings).Hours
	return !h.Enabled || h.Open(time.Now())
}

func (o *liveIO) Mark(nodeID, kind string) {
	_ = o.s.repo.AddBotEvent(o.ctx, o.bot.ID, o.version, o.conv.ID, nodeID, kind)
}

func (s *Service) botGraph(ctx context.Context, botID uint, version int) (*flow.Graph, error) {
	raw, err := s.repo.BotGraph(ctx, botID, version)
	if err != nil || raw == "" {
		return nil, errors.New("chatbot sürümü bulunamadı")
	}
	var g flow.Graph
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// pickBot chooses the flow that greets a customer on a device: the
// after-hours flow when the device is closed, a keyword flow when the
// message matches, else the entry flow.
func (s *Service) pickBot(ctx context.Context, ch *models.WAChannel, text string) *models.WABot {
	bots, err := s.repo.ActiveBotsOnChannel(ctx, ch.ID)
	if err != nil {
		return nil
	}
	h := device.Parse(ch.Settings).Hours
	now := time.Now()
	closed := h.Enabled && !h.Open(now)
	var entry, timed, after, keyword *models.WABot
	low := strings.ToLower(text)
	for i := range bots {
		b := &bots[i]
		sc := hours.ParseSchedule(b.Schedule)
		if b.Trigger != "after_hours" && !sc.Fits(h, now) {
			continue
		}
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
			if sc.Mode != "always" && timed == nil {
				timed = b
			} else if sc.Mode == "always" && entry == nil {
				entry = b
			}
		}
	}
	switch {
	case closed && after != nil:
		return after
	case keyword != nil:
		return keyword
	case timed != nil:
		return timed
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
	contact, _ := s.repo.Contact(ctx, conv.ContactID)
	st := &flow.State{Vars: map[string]string{}}
	if contact != nil {
		v := contactView(contact)
		st.Vars["musteri"] = firstName(v.Display)
		st.Vars["numara"] = "+" + contact.WAID
	}
	if err := s.repo.SetTicketBot(ctx, ticket.ID); err != nil {
		return false
	}
	ticket.Status = "bot"
	if err := s.repo.StartBotSession(ctx, conv.ID, bot.ID, bot.PublishedVersion); err != nil {
		return false
	}
	s.runStep(ctx, ch, conv, ticket, bot, bot.PublishedVersion, g, st, nil)
	return true
}

// continueBot feeds the customer's answer to their flow.
func (s *Service) continueBot(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, msg *models.WAMessage) bool {
	sess, err := s.repo.BotSession(ctx, conv.ID)
	if err != nil {
		// The flow is gone (ended or timed out): a person takes over.
		s.botToHuman(ctx, ch, conv, ticket, 0, "")
		return false
	}
	bot, err := s.repo.LoadBot(ctx, sess.BotID)
	if err != nil {
		s.botToHuman(ctx, ch, conv, ticket, 0, "")
		return false
	}
	set := device.Parse(ch.Settings)
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
	st := &flow.State{NodeID: sess.NodeID, Tries: sess.Tries, Vars: map[string]string{}}
	_ = json.Unmarshal([]byte(sess.Vars), &st.Vars)
	in := &flow.Input{Text: msg.Body}
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
	s.runStep(ctx, ch, conv, ticket, bot, sess.Version, g, st, in)
	return true
}

// runStep walks the flow and saves or ends the session.
func (s *Service) runStep(ctx context.Context, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, bot *models.WABot, version int, g *flow.Graph, st *flow.State, in *flow.Input) {
	io := &liveIO{s: s, ctx: ctx, ch: ch, conv: conv, ticket: ticket, bot: bot, version: version}
	flow.Step(g, st, in, io)
	if !st.Done {
		warnDB(ctx, s.repo.SaveBotSession(ctx, conv.ID, st.NodeID, jsonString(st.Vars), st.Tries))
		return
	}
	warnDB(ctx, s.repo.DeleteBotSession(ctx, conv.ID))
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
	warnDB(ctx, s.repo.UpdateBotTicket(ctx, ticket.ID, fields))
	line := "Chatbot sohbeti bir temsilciye aktardı."
	if teamID > 0 {
		name, err := s.repo.TeamName(ctx, teamID)
		warnDB(ctx, err)
		if name != "" {
			line = "Chatbot sohbeti " + name + " ekibine aktardı."
		}
	}
	if n := strings.TrimSpace(note); n != "" {
		line += " " + n
	}
	s.event(ctx, nil, conv, ticket.ID, 0, line)
	// The waiting clock starts only now that a person is needed.
	warnDB(ctx, s.repo.StartAwaiting(ctx, ticket.ID))
	s.distribute(ctx, ch, ticket.ID)
	s.publish(ctx, conv.ID, nil, nil)
}

// endBot stops a conversation's flow, if any.
func (s *Service) endBot(ctx context.Context, conversationID uint, kind string) {
	sess, err := s.repo.BotSession(ctx, conversationID)
	if err != nil {
		return
	}
	warnDB(ctx, s.repo.DeleteBotSession(ctx, conversationID))
	warnDB(ctx, s.repo.AddBotEvent(ctx, sess.BotID, sess.Version, conversationID, sess.NodeID, kind))
	warnDB(ctx, s.repo.ReleaseBotTicket(ctx, conversationID))
}

// sweepBots ends flows whose customer went quiet; the ticket is closed
// without a survey and opens again when they write.
func (s *Service) sweepBots(ctx context.Context) {
	rows, err := s.repo.IdleBotSessions(ctx)
	warnDB(ctx, err)
	for _, r := range rows {
		ch, err := s.repo.Channel(ctx, r.ChannelID)
		if err != nil {
			continue
		}
		if time.Since(r.UpdatedAt) < time.Duration(device.Parse(ch.Settings).BotTimeoutMinutes)*time.Minute {
			continue
		}
		conv, ticket, err := s.repo.Conversation(ctx, r.ConversationID)
		if err != nil || ticket == nil {
			continue
		}
		// A ticket a person already has is never closed by the chatbot's
		// clock; only the leftover session goes.
		opened, err := s.repo.OpenBotTicket(ctx, ticket.ID)
		if err != nil {
			warnDB(ctx, err)
			continue
		}
		if opened == 0 {
			s.endBot(ctx, conv.ID, "handoff")
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
	Schedule         hours.Schedule  `json:"schedule"`
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
		Keywords: parseTags(b.Keywords), Schedule: hours.ParseSchedule(b.Schedule), Draft: json.RawMessage(b.Draft), PublishedVersion: b.PublishedVersion, PublishedAt: b.PublishedAt, UpdatedAt: b.UpdatedAt}
	if b.PublishedVersion == 0 {
		v.DraftChanged = true
	} else {
		pub, err := s.repo.BotVersionText(ctx, b.ID, b.PublishedVersion)
		warnDB(ctx, err)
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
		return nil, errs.Forbidden("Chatbot'ları görme yetkin yok.")
	}
	return u, nil
}

// Bots lists the chatbots.
func (s *Service) Bots(ctx context.Context, actorID uint) ([]BotView, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	list, err := s.repo.Bots(ctx)
	if err != nil {
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
	b, err := s.repo.LoadBot(ctx, id)
	if err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	v := s.botView(ctx, b)
	return &v, nil
}

// BotInput is the chatbot's card: name, when it runs and on which devices.
type BotInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Trigger     string          `json:"trigger"`
	Keywords    []string        `json:"keywords"`
	Schedule    *hours.Schedule `json:"schedule"`
	ChannelIDs  []uint          `json:"channelIds"`
	Active      *bool           `json:"active"`
}

// schedule reads the hours of the form; after-hours chatbots always follow
// the device's working hours, so they carry none of their own.
func (in BotInput) schedule(trigger string) (hours.Schedule, error) {
	sc := hours.Schedule{Mode: "always", Spans: []hours.Span{}}
	if in.Schedule != nil && trigger != "after_hours" {
		sc = hours.ParseSchedule(jsonString(in.Schedule))
		if sc.Mode != "custom" {
			sc.Spans = []hours.Span{}
		}
	}
	if err := sc.Check(); err != nil {
		return sc, errs.Invalid("Çalışma saatleri: "+err.Error()+".", nil)
	}
	return sc, nil
}

func starterGraph() string {
	g := flow.Graph{
		Nodes: []flow.Node{
			{ID: "start", Type: "start", X: 80, Y: 200},
			{ID: "welcome", Type: "message", X: 320, Y: 180, Data: flow.Data{Text: "Merhaba {musteri}, size nasıl yardımcı olabiliriz?"}},
			{ID: "menu", Type: "menu", X: 600, Y: 160, Data: flow.Data{Text: "Lütfen bir konu seçin.", Style: "buttons", Options: []flow.Option{{ID: "o1", Label: "Destek"}, {ID: "o2", Label: "Satış"}, {ID: "o3", Label: "Temsilci"}}}},
			{ID: "h", Type: "handoff", X: 900, Y: 200, Data: flow.Data{Text: "Sizi bir temsilcimize aktarıyorum, en kısa sürede dönüş yapacağız."}},
		},
		Edges: []flow.Edge{
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

// checkBotConflict keeps one always-on entry flow and one after-hours flow
// per device, so it is never unclear which one answers. Entry flows with
// their own hours may sit beside them; in their hours they come first.
func (s *Service) checkBotConflict(ctx context.Context, id uint, trigger string, sc hours.Schedule, channels []uint, active bool) error {
	if !active || trigger == "keyword" || (trigger == "entry" && sc.Mode != "always") {
		return nil
	}
	others, err := s.repo.ConflictingBots(ctx, trigger, id)
	if err != nil {
		return errs.Internal(err)
	}
	for _, o := range others {
		for _, a := range parseIDs(o.ChannelIDs) {
			for _, b := range channels {
				if a == b {
					name, err := s.repo.ChannelName(ctx, a)
					if err != nil {
						return errs.Internal(err)
					}
					return errs.Conflict(fmt.Sprintf("%s cihazında zaten açık bir %s var: %s. Önce onu kapat ya da cihazdan çıkar.", name, triggerWord(trigger), o.Name), nil)
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
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot oluşturma yetkin yok."); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errs.Invalid("Chatbot'a bir ad ver.", nil)
	}
	trigger := normTrigger(in.Trigger)
	sc, err := in.schedule(trigger)
	if err != nil {
		return nil, err
	}
	b := &models.WABot{Name: name, Description: strings.TrimSpace(in.Description), Trigger: trigger, Keywords: jsonString(cleanTags(in.Keywords)),
		Schedule: jsonString(sc), ChannelIDs: "[]", Draft: starterGraph(), CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	if err := s.repo.CreateBot(ctx, b); err != nil {
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
	b, err := s.repo.LoadBot(ctx, id)
	if err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	fields := map[string]any{"updated_at": time.Now()}
	if n := strings.TrimSpace(in.Name); n != "" {
		fields["name"] = n
	}
	fields["description"] = strings.TrimSpace(in.Description)
	trigger := normTrigger(in.Trigger)
	fields["trigger"] = trigger
	sc := hours.ParseSchedule(b.Schedule)
	if in.Schedule != nil || trigger == "after_hours" {
		if sc, err = in.schedule(trigger); err != nil {
			return nil, err
		}
	}
	fields["schedule"] = jsonString(sc)
	fields["keywords"] = jsonString(cleanTags(in.Keywords))
	channels := in.ChannelIDs
	if channels == nil {
		channels = []uint{}
	}
	active := b.Active
	if in.Active != nil {
		active = *in.Active
	}
	liveChange := active != b.Active || jsonString(channels) != jsonString(parseIDs(b.ChannelIDs)) || trigger != b.Trigger || jsonString(sc) != jsonString(hours.ParseSchedule(b.Schedule))
	if liveChange && !u.Can(enums.WABotPublish) {
		return nil, errs.Forbidden("Chatbot'u açıp kapatma ya da cihaz seçme yetkin yok.")
	}
	if active && b.PublishedVersion == 0 {
		return nil, errs.Invalid("Chatbot'u açmadan önce yayına al.", nil)
	}
	if err := s.checkBotConflict(ctx, id, trigger, sc, channels, active); err != nil {
		return nil, err
	}
	fields["channel_ids"] = jsonString(channels)
	fields["active"] = active
	if !u.Can(enums.WABotManage) {
		delete(fields, "name")
		delete(fields, "description")
		delete(fields, "keywords")
	}
	if err := s.repo.UpdateBot(ctx, id, fields); err != nil {
		return nil, errs.Internal(err)
	}
	return s.Bot(ctx, actorID, id)
}

// SaveDraft stores the drawing without touching what customers meet.
func (s *Service) SaveDraft(ctx context.Context, actorID, id uint, g flow.Graph) (*BotView, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkin yok."); err != nil {
		return nil, err
	}
	if len(g.Nodes) > 300 {
		return nil, errs.Invalid("Bir akış en fazla 300 kutu olabilir.", nil)
	}
	if err := s.repo.UpdateBot(ctx, id, map[string]any{"draft": jsonString(g), "updated_at": time.Now()}); err != nil {
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
	if _, err := s.require(ctx, actorID, enums.WABotPublish, "Chatbot yayınlama yetkin yok."); err != nil {
		return nil, err
	}
	b, err := s.repo.LoadBot(ctx, id)
	if err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	var g flow.Graph
	if err := json.Unmarshal([]byte(b.Draft), &g); err != nil {
		return nil, errs.Invalid("Akış okunamadı.", err)
	}
	if p := g.Validate(); len(p) > 0 {
		return &PublishResult{Problems: p}, nil
	}
	next := b.PublishedVersion + 1
	if err := s.repo.PublishBot(ctx, id, next, b.Draft, actorID); err != nil {
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
	rows, err := s.repo.BotVersions(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]BotVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, BotVersion(r))
	}
	return out, nil
}

// RestoreVersion puts an older version back into the draft.
func (s *Service) RestoreVersion(ctx context.Context, actorID, id uint, version int) (*BotView, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot düzenleme yetkin yok."); err != nil {
		return nil, err
	}
	raw, err := s.repo.BotVersionText(ctx, id, version)
	if err != nil || raw == "" {
		return nil, errs.NotFound("Sürüm bulunamadı.")
	}
	if err := s.repo.SetBotDraft(ctx, id, raw); err != nil {
		return nil, errs.Internal(err)
	}
	return s.Bot(ctx, actorID, id)
}

// CopyBot makes an independent copy, for another device or a variant.
func (s *Service) CopyBot(ctx context.Context, actorID, id uint, name string, channelIDs []uint) (*BotView, error) {
	if _, err := s.require(ctx, actorID, enums.WABotManage, "Chatbot kopyalama yetkin yok."); err != nil {
		return nil, err
	}
	src, err := s.repo.LoadBot(ctx, id)
	if err != nil {
		return nil, errs.NotFound("Chatbot bulunamadı.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = src.Name + " (kopya)"
	}
	if channelIDs == nil {
		channelIDs = []uint{}
	}
	b := &models.WABot{Name: name, Description: src.Description, Trigger: src.Trigger, Keywords: src.Keywords, Schedule: src.Schedule, ChannelIDs: jsonString(channelIDs),
		Draft: src.Draft, CreatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	if err := s.repo.CreateBot(ctx, b); err != nil {
		return nil, errs.Internal(err)
	}
	v := s.botView(ctx, b)
	return &v, nil
}

// SimInput is one turn in the test screen.
type SimInput struct {
	Graph     flow.Graph        `json:"graph"`
	NodeID    string            `json:"nodeId"`
	Vars      map[string]string `json:"vars"`
	Tries     int               `json:"tries"`
	Text      string            `json:"text"`
	ChoiceID  string            `json:"choiceId"`
	Start     bool              `json:"start"`
	HoursOpen bool              `json:"hoursOpen"` // used only when no device is given
	// the chatbot being tested and the device whose working hours count
	BotID     uint `json:"botId"`
	ChannelID uint `json:"channelId"`
	// the moment the test pretends it is; empty means now
	Clock string `json:"clock"` // "14:30"
	Day   *int   `json:"day"`   // 0 is Monday
}

// simNow is the moment a test run pretends it is, in Turkey time.
func (in SimInput) simNow() time.Time {
	now := time.Now().In(hours.Zone)
	if in.Day != nil && *in.Day >= 0 && *in.Day <= 6 {
		today := (int(now.Weekday()) + 6) % 7
		now = now.AddDate(0, 0, *in.Day-today)
	}
	if hours.ValidClock(in.Clock) {
		m := hours.MinuteOf(in.Clock)
		now = time.Date(now.Year(), now.Month(), now.Day(), m/60, m%60, 0, 0, hours.Zone)
	}
	return now
}

// SimResult is what the flow did and where it stands.
type SimResult struct {
	Outputs []flow.SimOutput  `json:"outputs"`
	NodeID  string            `json:"nodeId"`
	Vars    map[string]string `json:"vars"`
	Tries   int               `json:"tries"`
	Done    bool              `json:"done"`
	// what the test assumed: in working hours or not, by which device
	HoursOpen bool   `json:"hoursOpen"`
	Channel   string `json:"channel,omitempty"`
}

// Simulate runs a turn of a flow without sending anything to anyone.
// Outside systems are really asked, so their answers can be checked.
func (s *Service) Simulate(ctx context.Context, actorID uint, in SimInput) (*SimResult, error) {
	if _, err := s.botManager(ctx, actorID); err != nil {
		return nil, err
	}
	st := &flow.State{NodeID: in.NodeID, Vars: in.Vars, Tries: in.Tries}
	if st.Vars == nil {
		st.Vars = map[string]string{"musteri": "Ayşe", "numara": "+905xxxxxxxxx"}
	}
	at := in.simNow()
	// Working hours come from the device at the pretended moment, so a
	// Sunday test on a device closed on Sundays is outside working hours.
	var h hours.Week
	open, chName := in.HoursOpen, ""
	chID := in.ChannelID
	var bot *models.WABot
	if in.BotID > 0 {
		if b, err := s.repo.LoadBot(ctx, in.BotID); err == nil {
			bot = b
			if ids := parseIDs(b.ChannelIDs); chID == 0 && len(ids) > 0 {
				chID = ids[0]
			}
		}
	}
	if chID > 0 {
		if ch, err := s.repo.Channel(ctx, chID); err == nil {
			h = device.Parse(ch.Settings).Hours
			open, chName = !h.Enabled || h.Open(at), ch.Name
		}
	}
	io := flow.NewSimIO(open, at, func(id uint, vars map[string]string) (map[string]any, error) {
		return s.callIntegration(ctx, id, vars)
	})
	var input *flow.Input
	if in.Start {
		st.NodeID = ""
		// Would this chatbot greet the customer at all at that moment?
		if bot != nil && chName != "" {
			skip, note := simGate(bot, h, open, at, chName)
			if skip != "" {
				return &SimResult{Outputs: []flow.SimOutput{{Kind: "skip", Text: skip}}, Vars: st.Vars, Done: true, HoursOpen: open, Channel: chName}, nil
			}
			if note != "" {
				io.Out = append(io.Out, flow.SimOutput{Kind: "note", Text: note})
			}
		}
	} else {
		input = &flow.Input{Text: in.Text, ChoiceID: in.ChoiceID}
	}
	flow.Step(&in.Graph, st, input, io)
	if io.Out == nil {
		io.Out = []flow.SimOutput{}
	}
	return &SimResult{Outputs: io.Out, NodeID: st.NodeID, Vars: st.Vars, Tries: st.Tries, Done: st.Done, HoursOpen: open, Channel: chName}, nil
}

// simGate says whether a chatbot would greet a customer writing at t on a
// device with these working hours: a reason when it would not, or a note
// when it would although the device is closed.
func simGate(bot *models.WABot, h hours.Week, open bool, t time.Time, chName string) (skip, note string) {
	sc := hours.ParseSchedule(bot.Schedule)
	switch {
	case bot.Trigger == "after_hours" && open:
		return "Bu saatte " + chName + " mesai içinde. Bu chatbot sadece mesai dışında çalıştığı için müşteriyi karşılamaz; sohbet doğrudan temsilcilere düşer.", ""
	case bot.Trigger != "after_hours" && !sc.Fits(h, t):
		return "Bu saatte bu chatbot çalışmaz (ayarlarındaki \"Hangi saatlerde çalışsın\" seçimine göre). Müşteri bu saatte yazarsa, o saatte çalışan başka bir chatbot varsa o karşılar; yoksa sohbet doğrudan temsilcilere düşer.", ""
	case bot.Trigger != "after_hours" && sc.Mode == "always" && !open:
		return "", "Bu saatte " + chName + " için mesai dışı. Bu chatbot \"Her zaman\" çalışacak şekilde ayarlı olduğu için yine karşılar. Mesai dışında karşılamasın istiyorsan chatbot ayarlarından \"Mesai saatlerinde\" seç ya da akışa Koşul > Mesai dışındaysa ekle."
	}
	return "", ""
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
	rows, err := s.repo.BotEventCounts(ctx, id, since)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := &BotStats{Nodes: map[string]int64{}, Fails: map[string]int64{}, Drops: map[string]int64{}}
	var g flow.Graph
	draft, err := s.repo.BotDraft(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	_ = json.Unmarshal([]byte(draft), &g)
	start := ""
	if n := g.Start(); n != nil {
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
	in, err := s.repo.LoadIntegration(ctx, id)
	if err != nil {
		return nil, errors.New("dış sorgu bulunamadı")
	}
	target, err := outside.FillURL(in.URL, vars)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if in.Method != "GET" && strings.TrimSpace(in.Body) != "" {
		body = bytes.NewReader([]byte(outside.FillJSON(in.Body, vars)))
	}
	timeout := time.Duration(in.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 8 * time.Second
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(c, in.Method, target, body)
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
				req.Header.Set(k, outside.FillHeader(v, vars))
			}
		}
	}
	resp, err := outside.Client.Do(req)
	if err != nil {
		return nil, outside.Explain(err)
	}
	defer func() { _ = resp.Body.Close() }()
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
