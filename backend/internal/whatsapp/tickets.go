package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/device"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/store"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// event writes a line into the conversation's history ("Toprak sohbeti
// üstlendi"), so it is always clear who did what.
func (s *Service) event(ctx context.Context, tx *gorm.DB, conv *models.WAConversation, ticketID uint, userID uint, text string) {
	m := &models.WAMessage{ChannelID: conv.ChannelID, ConversationID: conv.ID, TicketID: uintPtr(ticketID), Direction: "event", Kind: "event", SenderKind: "system", Body: text, Status: "received", CreatedAt: time.Now()}
	if userID > 0 {
		m.SenderUserID = uintPtr(userID)
	}
	var err error
	if tx == nil {
		err = s.repo.CreateMessage(ctx, m)
	} else {
		err = store.CreateMessage(tx, m)
	}
	if err != nil {
		slog.WarnContext(ctx, "whatsapp event line could not be written", "error", err)
	}
}

func firstName(name string) string {
	if i := strings.IndexByte(name, ' '); i > 0 {
		return name[:i]
	}
	return name
}

// ---------------------------------------------------------------- distribution

// eligible lists who may take a new ticket on a device now: active,
// on shift, available, with the panel open and room under their limit.
// The one who got a ticket longest ago comes first.
func (s *Service) eligible(ctx context.Context, ch *models.WAChannel, teamID *uint) []uint {
	set := device.Parse(ch.Settings)
	ids, err := s.repo.EligibleAgents(ctx, ch.ID, teamID)
	if err != nil || len(ids) == 0 {
		return nil
	}
	viewers, _ := s.loadViewers(ctx)
	online := s.push.OnlineUsers(ids)
	var out []uint
	for _, id := range ids {
		v := viewers[id]
		if v == nil || !v.can(enums.WAReply) || !online[id] {
			continue
		}
		if set.Distribution.MaxOpen > 0 {
			open, err := s.repo.OpenTicketCount(ctx, id)
			warnDB(ctx, err)
			if open >= int64(set.Distribution.MaxOpen) {
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

// distribute hands an unowned ticket to the next available person when
// the device distributes automatically; otherwise it waits in the pool.
func (s *Service) distribute(ctx context.Context, ch *models.WAChannel, ticketID uint) bool {
	set := device.Parse(ch.Settings)
	if !set.Distribution.Enabled {
		return false
	}
	t, err := s.repo.LoadTicket(ctx, ticketID)
	if err != nil || t.OwnerID != nil || t.Status == "resolved" || t.Status == "bot" {
		return false
	}
	// The fairest agent gets it; eligible puts them first.
	candidates := s.eligible(ctx, ch, t.TeamID)
	if len(candidates) > 0 {
		uid := candidates[0]
		before := s.audience(ctx, t)
		ok, err := s.repo.AutoAssign(ctx, t.ID, ch.ID, uid, t.TeamID)
		if err != nil {
			slog.ErrorContext(ctx, "whatsapp ticket could not be handed out", "ticket", t.ID, "error", err)
			return false
		}
		if !ok {
			return false
		}
		conv, _, _ := s.repo.Conversation(ctx, t.ConversationID)
		if conv != nil {
			s.event(ctx, nil, conv, t.ID, 0, s.repo.UserName(ctx, uid)+" sohbete otomatik olarak atandı.")
			s.publish(ctx, conv.ID, nil, before)
		}
		s.push.Push([]uint{uid}, Event{Type: "wa.assigned", ConversationID: t.ConversationID, Text: "Sana yeni bir WhatsApp sohbeti atandı."})
		if conv != nil {
			s.runAutomations(ctx, ch, "ticket_assigned", conv, s.repo.Ticket(ctx, t.ID), nil)
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------- actions

// titleOf is how an agent is introduced: their profile headline, or their
// role.
func (s *Service) titleOf(ctx context.Context, u *models.User) string {
	headline, err := s.repo.UserHeadline(ctx, u.ID)
	warnDB(ctx, err)
	if h := strings.TrimSpace(headline); h != "" {
		if i := strings.IndexAny(h, "·|,"); i > 0 {
			h = strings.TrimSpace(h[:i])
		}
		return h
	}
	for _, r := range u.Roles {
		if r.DisplayName != "" && !u.IsInvisibleAdmin() {
			return r.DisplayName
		}
	}
	return "müşteri temsilciniz"
}

// takenMeanwhile explains a refusal to someone who picked a conversation
// from the pool a moment after a colleague did: it left their pool, which
// is not the same as never having been theirs to see.
func (s *Service) takenMeanwhile(ctx context.Context, actorID, conversationID uint, err error) error {
	var e *errs.Error
	if !errors.As(err, &e) || e.Status != fiber.StatusForbidden {
		return err
	}
	v, verr := s.viewerOf(ctx, actorID)
	if verr != nil || !v.can(enums.WAPool) {
		return err
	}
	_, ticket, terr := s.repo.Conversation(ctx, conversationID)
	if terr != nil || ticket == nil || !v.seesChannel(ticket.ChannelID) || ticket.OwnerID == nil || *ticket.OwnerID == actorID {
		return err
	}
	return errs.Conflict("Bu sohbeti az önce başka bir temsilci aldı.", nil)
}

// Greet is "Karşıla": the person takes the chat if nobody owns it, joins
// the owner otherwise, and the greeting goes out when it is switched on.
func (s *Service) Greet(ctx context.Context, actorID, conversationID uint) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return s.takenMeanwhile(ctx, actorID, conversationID, err)
	}
	if !v.can(enums.WAReply) {
		return errs.Forbidden("Müşteriye yazma yetkin yok.")
	}
	if ticket.Status == "resolved" {
		return errs.Invalid("Bu sohbet çözülmüş. Müşteri yazınca yeniden açılır.", nil)
	}
	ch, err := s.repo.Channel(ctx, conv.ChannelID)
	if err != nil {
		return err
	}
	before := s.audience(ctx, ticket)
	role := "helper"
	var joined bool
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		cur, err := store.LockTicketRow(tx, ticket.ID)
		if err != nil {
			return err
		}
		if cur.OwnerID == nil {
			if !v.can(enums.WAPool) && !v.can(enums.WAViewAll) && cur.WaitingListedAt == nil {
				return errs.Forbidden("Havuzdan sohbet alma yetkin yok.")
			}
			role = "owner"
			if err := store.ClaimTicket(tx, cur.ID, actorID); err != nil {
				return err
			}
		} else if *cur.OwnerID == actorID {
			role = "owner"
		} else if cur.WaitingListedAt != nil && !v.can(enums.WAWaiting) && !v.can(enums.WAViewAll) {
			return errs.Forbidden("Cevap Bekleyenler'e katılma yetkin yok.")
		}
		added, err := store.JoinTicket(tx, cur.ID, actorID, role)
		if err != nil {
			return err
		}
		joined = added
		if role == "helper" && joined {
			return store.RecordJoin(tx, cur.ID, actorID)
		}
		return nil
	})
	if err != nil {
		var e *errs.Error
		if errors.As(err, &e) {
			return e
		}
		return errs.Internal(err)
	}
	name := s.repo.UserName(ctx, actorID)
	switch {
	case role == "owner" && joined:
		s.event(ctx, nil, conv, ticket.ID, actorID, name+" sohbeti üstlendi.")
	case joined:
		s.event(ctx, nil, conv, ticket.ID, actorID, name+" yardıma katıldı.")
	}
	if ticket.Status == "bot" {
		s.endBot(ctx, conv.ID, "handoff")
	}
	s.sendGreeting(ctx, v.user, ch, conv, s.repo.Ticket(ctx, ticket.ID), role)
	s.publish(ctx, conv.ID, nil, before)
	return nil
}

// sendGreeting sends the device's greeting once per person and ticket.
func (s *Service) sendGreeting(ctx context.Context, u *models.User, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, role string) {
	if ticket == nil {
		return
	}
	set := device.Parse(ch.Settings)
	g := set.Greeting
	if !g.Enabled || (role == "helper" && !g.ForHelpers) {
		return
	}
	greeted, err := s.repo.Greeted(ctx, ticket.ID, u.ID)
	warnDB(ctx, err)
	if greeted {
		return
	}
	contact, err := s.repo.Contact(ctx, conv.ContactID)
	if err != nil {
		return
	}
	title := s.titleOf(ctx, u)
	customer := contact.Name
	if customer == "" {
		customer = contact.ProfileName
	}
	fill := strings.NewReplacer("{ad}", firstName(u.Name), "{adsoyad}", u.Name, "{unvan}", title, "{musteri}", firstName(customer))
	text := strings.TrimSpace(fill.Replace(g.Text))
	text = strings.Join(strings.Fields(strings.ReplaceAll(text, "  ", " ")), " ")
	in := SendInput{Kind: "text", Body: text, ClientID: fmt.Sprintf("greet-%d-%d", ticket.ID, u.ID)}
	if !windowOpen(conv) {
		if g.Template == "" {
			return
		}
		tpl, err := s.repo.ApprovedTemplate(ctx, ch.WABAID, g.Template, g.TemplateLang)
		if err != nil {
			return
		}
		in = SendInput{Kind: "template", TemplateID: tpl.ID, ClientID: in.ClientID, Params: &TemplateParams{Body: []string{firstName(u.Name), title, firstName(customer)}}}
	}
	if _, err := s.Send(ctx, u.ID, conv.ID, in); err != nil {
		slog.WarnContext(ctx, "whatsapp greeting could not be sent", "ticket", ticket.ID, "error", err)
		return
	}
	warnDB(ctx, s.repo.MarkGreeted(ctx, ticket.ID, u.ID))
}

// Take makes the person the owner; the previous owner stays as a helper.
func (s *Service) Take(ctx context.Context, actorID, conversationID uint) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WATake) {
		return errs.Forbidden("Sohbeti devralma yetkin yok.")
	}
	before := s.audience(ctx, ticket)
	var previous *uint
	taken := false
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		fresh, err := store.LockTicket(tx, ticket.ID)
		if err != nil {
			return err
		}
		if fresh.OwnerID != nil && *fresh.OwnerID == actorID {
			return nil
		}
		previous, taken = fresh.OwnerID, true
		if previous != nil {
			if err := store.DemoteOwner(tx, ticket.ID, *previous); err != nil {
				return err
			}
		}
		if err := store.SetTicketOwner(tx, ticket.ID, actorID); err != nil {
			return err
		}
		if err := store.MakeOwner(tx, ticket.ID, actorID); err != nil {
			return err
		}
		return store.RecordTake(tx, ticket.ID, previous, actorID)
	})
	if err != nil {
		return errs.Internal(err)
	}
	if !taken {
		return nil
	}
	s.event(ctx, nil, conv, ticket.ID, actorID, s.repo.UserName(ctx, actorID)+" sohbeti devraldı.")
	if previous != nil {
		s.push.Push([]uint{*previous}, Event{Type: "wa.alert", ConversationID: conv.ID, Text: s.repo.UserName(ctx, actorID) + " bir sohbetini devraldı. Sen yardımcı olarak kaldın.", Level: "info"})
	}
	s.publish(ctx, conv.ID, nil, before)
	return nil
}

// AssignInput moves a ticket to a person or a team.
type AssignInput struct {
	UserID uint   `json:"userId"`
	TeamID uint   `json:"teamId"`
	Note   string `json:"note"`
}

// Assign hands a ticket over with an optional note.
func (s *Service) Assign(ctx context.Context, actorID, conversationID uint, in AssignInput) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAAssign) {
		return errs.Forbidden("Sohbet aktarma yetkin yok.")
	}
	if in.UserID == 0 && in.TeamID == 0 {
		return errs.Invalid("Kime aktarılacağını seç.", nil)
	}
	ch, err := s.repo.Channel(ctx, conv.ChannelID)
	if err != nil {
		return err
	}
	if in.UserID > 0 {
		viewers, _ := s.loadViewers(ctx)
		target := viewers[in.UserID]
		if target == nil || !target.seesChannel(ch.ID) || !target.can(enums.WAReply) {
			return errs.Invalid("Seçilen kişi bu cihazda çalışmıyor ya da yazma yetkisi yok.", nil)
		}
	}
	before := s.audience(ctx, ticket)
	var teamName, userName string
	wasBot := false
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		// Work from the row as it is now: someone may have taken or moved
		// the ticket since it was read.
		fresh, err := store.LockTicket(tx, ticket.ID)
		if err != nil {
			return err
		}
		wasBot = fresh.Status == "bot"
		var team *uint
		if in.TeamID > 0 {
			team = uintPtr(in.TeamID)
			if teamName, err = store.TeamName(tx, in.TeamID); err != nil {
				return err
			}
			if teamName == "" {
				return errs.NotFound("Ekip bulunamadı.")
			}
		} else {
			team = fresh.TeamID
		}
		var owner *uint
		if in.UserID > 0 {
			owner = uintPtr(in.UserID)
		}
		if fresh.OwnerID != nil && (owner == nil || *fresh.OwnerID != *owner) {
			if err := store.DemoteOwner(tx, ticket.ID, *fresh.OwnerID); err != nil {
				return err
			}
		}
		if err := store.MoveTicket(tx, ticket.ID, owner, team); err != nil {
			return err
		}
		if owner != nil {
			if err := store.MakeOwner(tx, ticket.ID, *owner); err != nil {
				return err
			}
		}
		return store.RecordTransfer(tx, ticket.ID, fresh.OwnerID, owner, team, actorID, strings.TrimSpace(in.Note))
	})
	if err != nil {
		var e *errs.Error
		if errors.As(err, &e) {
			return e
		}
		return errs.Internal(err)
	}
	who := s.repo.UserName(ctx, actorID)
	switch {
	case in.UserID > 0:
		userName = s.repo.UserName(ctx, in.UserID)
		line := who + " sohbeti " + userName + " kişisine aktardı."
		if n := strings.TrimSpace(in.Note); n != "" {
			line += " Not: " + n
		}
		s.event(ctx, nil, conv, ticket.ID, actorID, line)
		s.push.Push([]uint{in.UserID}, Event{Type: "wa.assigned", ConversationID: conv.ID, Text: who + " sana bir WhatsApp sohbeti aktardı."})
	default:
		line := who + " sohbeti " + teamName + " ekibine aktardı."
		if n := strings.TrimSpace(in.Note); n != "" {
			line += " Not: " + n
		}
		s.event(ctx, nil, conv, ticket.ID, actorID, line)
		s.distribute(ctx, ch, ticket.ID)
	}
	if wasBot {
		s.endBot(ctx, conv.ID, "handoff")
	}
	s.publish(ctx, conv.ID, nil, before)
	return nil
}

// Resolve closes a ticket; the survey goes out if the device asks for one.
func (s *Service) Resolve(ctx context.Context, actorID, conversationID uint) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAResolve) {
		return errs.Forbidden("Sohbeti çözüldü olarak kapatma yetkin yok.")
	}
	if ticket.Status == "resolved" {
		return nil
	}
	if err := s.resolve(ctx, conv, ticket, actorID); err != nil {
		return err
	}
	return nil
}

func (s *Service) resolve(ctx context.Context, conv *models.WAConversation, ticket *models.WATicket, actorID uint) error {
	before := s.audience(ctx, ticket)
	var by *uint
	if actorID > 0 {
		by = uintPtr(actorID)
	}
	// Only the first of two people closing at once goes on, so the survey
	// and the closing rules run once.
	changed, err := s.repo.ResolveTicket(ctx, ticket.ID, by)
	if err != nil {
		return errs.Internal(err)
	}
	if changed == 0 {
		return nil
	}
	s.endBot(ctx, conv.ID, "end")
	if actorID > 0 {
		s.event(ctx, nil, conv, ticket.ID, actorID, s.repo.UserName(ctx, actorID)+" sohbeti çözüldü olarak kapattı.")
	}
	fresh := s.repo.Ticket(ctx, ticket.ID)
	if ch, err := s.repo.Channel(ctx, conv.ChannelID); err == nil && fresh != nil {
		s.runAutomations(ctx, ch, "ticket_resolved", conv, fresh, nil)
		if actorID > 0 {
			s.sendSurvey(ctx, ch, conv, fresh, actorID)
		}
	}
	s.publish(ctx, conv.ID, nil, before)
	return nil
}

// Reopen opens a resolved ticket again by hand.
func (s *Service) Reopen(ctx context.Context, actorID, conversationID uint) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAResolve) {
		return errs.Forbidden("Sohbeti yeniden açma yetkin yok.")
	}
	if ticket.Status != "resolved" {
		return nil
	}
	before := s.audience(ctx, ticket)
	changed, err := s.repo.ReopenTicket(ctx, ticket.ID)
	if err != nil {
		return errs.Internal(err)
	}
	if changed == 0 {
		return nil
	}
	s.event(ctx, nil, conv, ticket.ID, actorID, s.repo.UserName(ctx, actorID)+" sohbeti yeniden açtı.")
	s.publish(ctx, conv.ID, nil, before)
	return nil
}

// TicketInput changes a ticket's details.
type TicketInput struct {
	Status   *string   `json:"status"` // open | pending
	Priority *string   `json:"priority"`
	Category *string   `json:"category"`
	Tags     *[]string `json:"tags"`
}

// UpdateTicket changes status (open or waiting on the customer),
// priority, category or tags.
func (s *Service) UpdateTicket(ctx context.Context, actorID, conversationID uint, in TicketInput) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAReply) {
		return errs.Forbidden("Sohbeti düzenleme yetkin yok.")
	}
	fields := map[string]any{"updated_at": time.Now()}
	var lines []string
	name := s.repo.UserName(ctx, actorID)
	if in.Status != nil {
		switch *in.Status {
		case "open", "pending":
			if ticket.Status != "resolved" && ticket.Status != *in.Status {
				fields["status"] = *in.Status
				if *in.Status == "pending" {
					lines = append(lines, name+" sohbeti müşteriden cevap bekliyor olarak işaretledi.")
				} else {
					lines = append(lines, name+" sohbeti yeniden açık olarak işaretledi.")
				}
			}
		default:
			return errs.Invalid("Durum tanınmadı.", nil)
		}
	}
	if in.Priority != nil {
		switch *in.Priority {
		case "low", "normal", "high", "urgent":
			fields["priority"] = *in.Priority
		default:
			return errs.Invalid("Öncelik tanınmadı.", nil)
		}
	}
	if in.Category != nil {
		fields["category"] = strings.TrimSpace(*in.Category)
	}
	if in.Tags != nil {
		fields["tags"] = jsonString(cleanTags(*in.Tags))
	}
	// A status change must not bring back a ticket someone closed in the
	// meantime.
	_, changesStatus := fields["status"]
	changed, err := s.repo.EditTicket(ctx, ticket.ID, fields, changesStatus)
	if err != nil {
		return errs.Internal(err)
	}
	if changed == 0 {
		return errs.Conflict("Sohbet bu arada kapatıldı. Değiştirmek için önce yeniden aç.", nil)
	}
	for _, l := range lines {
		s.event(ctx, nil, conv, ticket.ID, actorID, l)
	}
	s.publish(ctx, conv.ID, nil, nil)
	return nil
}

func cleanTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || len([]rune(t)) > 40 || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

// notifyReopen tells the owner that their customer is back.
func (s *Service) notifyReopen(ctx context.Context, t *models.WATicket) {
	if t.OwnerID == nil {
		return
	}
	s.push.Push([]uint{*t.OwnerID}, Event{Type: "wa.alert", ConversationID: t.ConversationID, Text: fmt.Sprintf("Müşteri yeniden yazdı, #%d numaralı sohbet tekrar açıldı.", t.Number), Level: "info"})
}

// ---------------------------------------------------------------- the clock

// clock runs the timed work: the waiting list, handing out pooled
// tickets, chatbot timeouts, timed rules and housekeeping.
// housekeeping deletes old rows nobody reads any more.
func (s *Service) housekeeping(ctx context.Context) {
	for _, f := range s.repo.Housekeep(ctx) {
		slog.WarnContext(ctx, "whatsapp housekeeping failed", "query", f.Query, "error", f.Err)
	}
}

func (s *Service) clock(ctx context.Context) {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	last := map[string]time.Time{}
	every := func(name string, d time.Duration) bool {
		if time.Since(last[name]) < d {
			return false
		}
		last[name] = time.Now()
		return true
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		// Each job runs on its own, so a panic in one does not stop the rest.
		safe.Run(ctx, "whatsapp waiting sweep", func() { s.sweepWaiting(ctx) })
		if every("pool", 30*time.Second) {
			safe.Run(ctx, "whatsapp pool sweep", func() { s.sweepPool(ctx) })
		}
		if every("bots", time.Minute) {
			safe.Run(ctx, "whatsapp chatbot sweep", func() { s.sweepBots(ctx) })
		}
		if every("rules", time.Minute) {
			safe.Run(ctx, "whatsapp timed rules", func() { s.sweepTimedRules(ctx) })
		}
		if every("callsurveys", 30*time.Second) {
			safe.Run(ctx, "whatsapp call surveys", func() { s.sendDueCallSurveys(ctx) })
		}
		if every("templates", 6*time.Hour) {
			safe.Run(ctx, "whatsapp template sync", func() { s.syncAllTemplates(ctx) })
		}
		if every("housekeeping", 6*time.Hour) {
			safe.Run(ctx, "whatsapp housekeeping", func() { s.housekeeping(ctx) })
		}
	}
}

// sweepWaiting puts tickets whose customer waited too long into
// "Cevap Bekleyenler", counting only working hours.
func (s *Service) sweepWaiting(ctx context.Context) {
	list, err := s.repo.TicketsDueForWaitList(ctx)
	if err != nil {
		return
	}
	settings := map[uint]device.Settings{}
	for i := range list {
		t := &list[i]
		set, ok := settings[t.ChannelID]
		if !ok {
			ch, err := s.repo.Channel(ctx, t.ChannelID)
			if err != nil {
				continue
			}
			set = device.Parse(ch.Settings)
			settings[t.ChannelID] = set
		}
		if set.WaitingMinutes <= 0 {
			continue
		}
		if set.Hours.Elapsed(*t.AwaitingSince, time.Now()) < time.Duration(set.WaitingMinutes)*time.Minute {
			continue
		}
		before := s.audience(ctx, t)
		if changed, err := s.repo.PutOnWaitList(ctx, t.ID); err != nil || changed == 0 {
			continue
		}
		s.publish(ctx, t.ConversationID, nil, before)
		// A short signal for those who can help, so the list is not missed.
		fresh := s.repo.Ticket(ctx, t.ID)
		viewers, _ := s.loadViewers(ctx)
		parts := s.repo.Participants(ctx, t.ID)
		var ids []uint
		for id, v := range viewers {
			if v.can(enums.WAWaiting) && v.seesTicket(fresh, parts) {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			s.push.Push(ids, Event{Type: "wa.waiting", ConversationID: t.ConversationID, Text: fmt.Sprintf("Bir müşteri %d dakikadır cevap bekliyor.", set.WaitingMinutes)})
		}
	}
}

// sweepPool offers pooled tickets to people who became available.
func (s *Service) sweepPool(ctx context.Context) {
	list, err := s.repo.PooledTickets(ctx)
	if err != nil {
		return
	}
	chans := map[uint]*models.WAChannel{}
	for i := range list {
		t := &list[i]
		ch, ok := chans[t.ChannelID]
		if !ok {
			c, err := s.repo.Channel(ctx, t.ChannelID)
			if err != nil {
				continue
			}
			ch = c
			chans[t.ChannelID] = ch
		}
		if !device.Parse(ch.Settings).Distribution.Enabled {
			continue
		}
		if !s.distribute(ctx, ch, t.ID) {
			// Nobody is free on this device; the rest can wait too.
			chans[t.ChannelID] = &models.WAChannel{Settings: `{"distribution":{"enabled":false}}`}
		}
	}
}
