package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// event writes a line into the conversation's history ("Toprak sohbeti
// üstlendi"), so it is always clear who did what.
func (s *Service) event(ctx context.Context, tx *gorm.DB, conv *models.WAConversation, ticketID uint, userID uint, text string) {
	if tx == nil {
		tx = s.db.WithContext(ctx)
	}
	m := &models.WAMessage{ChannelID: conv.ChannelID, ConversationID: conv.ID, TicketID: uintPtr(ticketID), Direction: "event", Kind: "event", SenderKind: "system", Body: text, Status: "received", CreatedAt: time.Now()}
	if userID > 0 {
		m.SenderUserID = uintPtr(userID)
	}
	if err := tx.Create(m).Error; err != nil {
		slog.WarnContext(ctx, "whatsapp event line could not be written", "error", err)
	}
}

func (s *Service) userName(ctx context.Context, id uint) string {
	var name string
	warnDB(ctx, s.db.WithContext(ctx).Raw("SELECT name FROM users WHERE id = ?", id).Scan(&name).Error)
	if name == "" {
		return "Biri"
	}
	return name
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
	set := parseSettings(ch.Settings)
	q := `SELECT m.user_id FROM wa_channel_members m
		JOIN users u ON u.id = m.user_id AND u.active
		JOIN shifts sh ON sh.user_id = m.user_id AND sh.ended_at IS NULL
		LEFT JOIN agent_presence ap ON ap.user_id = m.user_id
		WHERE m.channel_id = ? AND COALESCE(ap.state, 'available') = 'available'`
	args := []any{ch.ID}
	if teamID != nil {
		q += " AND m.user_id IN (SELECT user_id FROM wa_team_members WHERE team_id = ?)"
		args = append(args, *teamID)
	}
	q += " GROUP BY m.user_id, m.last_assigned_at ORDER BY m.last_assigned_at NULLS FIRST, m.user_id"
	var ids []uint
	if err := s.db.WithContext(ctx).Raw(q, args...).Scan(&ids).Error; err != nil || len(ids) == 0 {
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
			var open int64
			warnDB(ctx, s.db.WithContext(ctx).Raw("SELECT count(*) FROM wa_tickets WHERE owner_id = ? AND status IN ('open','pending')", id).Scan(&open).Error)
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
	set := parseSettings(ch.Settings)
	if !set.Distribution.Enabled {
		return false
	}
	var t models.WATicket
	if err := s.db.WithContext(ctx).First(&t, ticketID).Error; err != nil || t.OwnerID != nil || t.Status == "resolved" || t.Status == "bot" {
		return false
	}
	for _, uid := range s.eligible(ctx, ch, t.TeamID) {
		before := s.audience(ctx, &t)
		ok := false
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Exec("UPDATE wa_tickets SET owner_id = ?, updated_at = now() WHERE id = ? AND owner_id IS NULL AND status NOT IN ('resolved','bot')", uid, t.ID)
			if res.Error != nil || res.RowsAffected == 0 {
				return res.Error
			}
			ok = true
			if err := tx.Exec("INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, 'owner') ON CONFLICT (ticket_id, user_id) DO UPDATE SET role = 'owner'", t.ID, uid).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE wa_channel_members SET last_assigned_at = now() WHERE channel_id = ? AND user_id = ?", ch.ID, uid).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, to_user, team_id) VALUES (?, 'auto', ?, ?)", t.ID, uid, t.TeamID).Error
		})
		if err != nil || !ok {
			return false
		}
		conv, _, _ := s.loadConv(ctx, t.ConversationID)
		if conv != nil {
			s.event(ctx, nil, conv, t.ID, 0, s.userName(ctx, uid)+" sohbete otomatik olarak atandı.")
			s.publish(ctx, conv.ID, nil, before)
		}
		s.push.Push([]uint{uid}, Event{Type: "wa.assigned", ConversationID: t.ConversationID, Text: "Size yeni bir WhatsApp sohbeti atandı."})
		if conv != nil {
			s.runAutomations(ctx, ch, "ticket_assigned", conv, s.ticketFresh(ctx, t.ID), nil)
		}
		return true
	}
	return false
}

func (s *Service) ticketFresh(ctx context.Context, id uint) *models.WATicket {
	var t models.WATicket
	if s.db.WithContext(ctx).First(&t, id).Error != nil {
		return nil
	}
	return &t
}

// ---------------------------------------------------------------- actions

// titleOf is how an agent is introduced: their profile headline, or their
// role.
func (s *Service) titleOf(ctx context.Context, u *models.User) string {
	var headline string
	warnDB(ctx, s.db.WithContext(ctx).Raw("SELECT headline FROM users WHERE id = ?", u.ID).Scan(&headline).Error)
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

// Greet is "Karşıla": the person takes the chat if nobody owns it, joins
// the owner otherwise, and the greeting goes out when it is switched on.
func (s *Service) Greet(ctx context.Context, actorID, conversationID uint) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAReply) {
		return errs.Forbidden("Müşteriye yazma yetkiniz yok.")
	}
	if ticket.Status == "resolved" {
		return errs.Invalid("Bu sohbet çözülmüş. Müşteri yazınca yeniden açılır.", nil)
	}
	ch, err := s.channel(ctx, conv.ChannelID)
	if err != nil {
		return err
	}
	before := s.audience(ctx, ticket)
	role := "helper"
	var joined bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cur models.WATicket
		if err := tx.Raw("SELECT * FROM wa_tickets WHERE id = ? FOR UPDATE", ticket.ID).Scan(&cur).Error; err != nil {
			return err
		}
		if cur.OwnerID == nil {
			if !v.can(enums.WAPool) && !v.can(enums.WAViewAll) && cur.WaitingListedAt == nil {
				return errs.Forbidden("Havuzdan sohbet alma yetkiniz yok.")
			}
			role = "owner"
			if err := tx.Exec("UPDATE wa_tickets SET owner_id = ?, status = CASE WHEN status = 'bot' THEN 'open' ELSE status END, updated_at = now() WHERE id = ?", actorID, cur.ID).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, to_user, by_user) VALUES (?, 'claim', ?, ?)", cur.ID, actorID, actorID).Error; err != nil {
				return err
			}
		} else if *cur.OwnerID == actorID {
			role = "owner"
		} else if cur.WaitingListedAt != nil && !v.can(enums.WAWaiting) && !v.can(enums.WAViewAll) {
			return errs.Forbidden("Cevap Bekleyenler'e katılma yetkiniz yok.")
		}
		res := tx.Exec(`INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, ?)
			ON CONFLICT (ticket_id, user_id) DO NOTHING`, cur.ID, actorID, role)
		if res.Error != nil {
			return res.Error
		}
		joined = res.RowsAffected > 0
		if role == "helper" && joined {
			return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, to_user, by_user) VALUES (?, 'join', ?, ?)", cur.ID, actorID, actorID).Error
		}
		return nil
	})
	if err != nil {
		if e, ok := err.(*errs.Error); ok {
			return e
		}
		return errs.Internal(err)
	}
	name := s.userName(ctx, actorID)
	switch {
	case role == "owner" && joined:
		s.event(ctx, nil, conv, ticket.ID, actorID, name+" sohbeti üstlendi.")
	case joined:
		s.event(ctx, nil, conv, ticket.ID, actorID, name+" yardıma katıldı.")
	}
	if ticket.Status == "bot" {
		s.endBot(ctx, conv.ID, "handoff")
	}
	s.sendGreeting(ctx, v.user, ch, conv, s.ticketFresh(ctx, ticket.ID), role)
	s.publish(ctx, conv.ID, nil, before)
	return nil
}

// sendGreeting sends the device's greeting once per person and ticket.
func (s *Service) sendGreeting(ctx context.Context, u *models.User, ch *models.WAChannel, conv *models.WAConversation, ticket *models.WATicket, role string) {
	if ticket == nil {
		return
	}
	set := parseSettings(ch.Settings)
	g := set.Greeting
	if !g.Enabled || (role == "helper" && !g.ForHelpers) {
		return
	}
	var greeted bool
	warnDB(ctx, s.db.WithContext(ctx).Raw("SELECT greeted FROM wa_ticket_participants WHERE ticket_id = ? AND user_id = ?", ticket.ID, u.ID).Scan(&greeted).Error)
	if greeted {
		return
	}
	contact, err := s.contact(ctx, conv.ContactID)
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
		var tpl models.WATemplate
		if s.db.WithContext(ctx).Where("waba_id = ? AND name = ? AND status = 'APPROVED'", ch.WABAID, g.Template).
			Where("language = ? OR ? = ''", g.TemplateLang, g.TemplateLang).First(&tpl).Error != nil {
			return
		}
		in = SendInput{Kind: "template", TemplateID: tpl.ID, ClientID: in.ClientID, Params: &TemplateParams{Body: []string{firstName(u.Name), title, firstName(customer)}}}
	}
	if _, err := s.Send(ctx, u.ID, conv.ID, in); err != nil {
		slog.WarnContext(ctx, "whatsapp greeting could not be sent", "ticket", ticket.ID, "error", err)
		return
	}
	warnDB(ctx, s.db.WithContext(ctx).Exec("UPDATE wa_ticket_participants SET greeted = true WHERE ticket_id = ? AND user_id = ?", ticket.ID, u.ID).Error)
}

// Take makes the person the owner; the previous owner stays as a helper.
func (s *Service) Take(ctx context.Context, actorID, conversationID uint) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WATake) {
		return errs.Forbidden("Sohbeti devralma yetkiniz yok.")
	}
	before := s.audience(ctx, ticket)
	var previous *uint
	taken := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fresh, err := lockTicket(tx, ticket.ID)
		if err != nil {
			return err
		}
		if fresh.OwnerID != nil && *fresh.OwnerID == actorID {
			return nil
		}
		previous, taken = fresh.OwnerID, true
		if previous != nil {
			if err := tx.Exec("UPDATE wa_ticket_participants SET role = 'helper' WHERE ticket_id = ? AND user_id = ?", ticket.ID, *previous).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE wa_tickets SET owner_id = ?, updated_at = now() WHERE id = ?", actorID, ticket.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, 'owner')
			ON CONFLICT (ticket_id, user_id) DO UPDATE SET role = 'owner'`, ticket.ID, actorID).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, from_user, to_user, by_user) VALUES (?, 'take', ?, ?, ?)", ticket.ID, previous, actorID, actorID).Error
	})
	if err != nil {
		return errs.Internal(err)
	}
	if !taken {
		return nil
	}
	s.event(ctx, nil, conv, ticket.ID, actorID, s.userName(ctx, actorID)+" sohbeti devraldı.")
	if previous != nil {
		s.push.Push([]uint{*previous}, Event{Type: "wa.alert", ConversationID: conv.ID, Text: s.userName(ctx, actorID) + " bir sohbetinizi devraldı. Siz yardımcı olarak kaldınız.", Level: "info"})
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
		return errs.Forbidden("Sohbet aktarma yetkiniz yok.")
	}
	if in.UserID == 0 && in.TeamID == 0 {
		return errs.Invalid("Kime aktarılacağını seçin.", nil)
	}
	ch, err := s.channel(ctx, conv.ChannelID)
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
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Work from the row as it is now: someone may have taken or moved
		// the ticket since it was read.
		fresh, err := lockTicket(tx, ticket.ID)
		if err != nil {
			return err
		}
		wasBot = fresh.Status == "bot"
		var team *uint
		if in.TeamID > 0 {
			team = uintPtr(in.TeamID)
			if err := tx.Raw("SELECT name FROM wa_teams WHERE id = ?", in.TeamID).Scan(&teamName).Error; err != nil {
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
			if err := tx.Exec("UPDATE wa_ticket_participants SET role = 'helper' WHERE ticket_id = ? AND user_id = ?", ticket.ID, *fresh.OwnerID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE wa_tickets SET owner_id = ?, team_id = ?, status = CASE WHEN status = 'bot' THEN 'open' ELSE status END, updated_at = now() WHERE id = ?", owner, team, ticket.ID).Error; err != nil {
			return err
		}
		if owner != nil {
			if err := tx.Exec(`INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, 'owner')
				ON CONFLICT (ticket_id, user_id) DO UPDATE SET role = 'owner'`, ticket.ID, *owner).Error; err != nil {
				return err
			}
		}
		return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, from_user, to_user, team_id, by_user, note) VALUES (?, 'transfer', ?, ?, ?, ?, ?)",
			ticket.ID, fresh.OwnerID, owner, team, actorID, strings.TrimSpace(in.Note)).Error
	})
	if err != nil {
		var e *errs.Error
		if errors.As(err, &e) {
			return e
		}
		return errs.Internal(err)
	}
	who := s.userName(ctx, actorID)
	switch {
	case in.UserID > 0:
		userName = s.userName(ctx, in.UserID)
		line := who + " sohbeti " + userName + " kişisine aktardı."
		if n := strings.TrimSpace(in.Note); n != "" {
			line += " Not: " + n
		}
		s.event(ctx, nil, conv, ticket.ID, actorID, line)
		s.push.Push([]uint{in.UserID}, Event{Type: "wa.assigned", ConversationID: conv.ID, Text: who + " size bir WhatsApp sohbeti aktardı."})
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

// lockTicket reads a ticket's current state and holds its row until the
// transaction ends, so two people changing it at once take turns instead
// of overwriting each other.
func lockTicket(tx *gorm.DB, id uint) (*models.WATicket, error) {
	var t models.WATicket
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&t, id).Error; err != nil {
		return nil, fmt.Errorf("ticket %d could not be locked: %w", id, err)
	}
	return &t, nil
}

// Resolve closes a ticket; the survey goes out if the device asks for one.
func (s *Service) Resolve(ctx context.Context, actorID, conversationID uint) error {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if !v.can(enums.WAResolve) {
		return errs.Forbidden("Sohbeti çözüldü olarak kapatma yetkiniz yok.")
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
	res := s.db.WithContext(ctx).Exec(`UPDATE wa_tickets SET status = 'resolved', resolved_at = now(), resolved_by = ?,
		awaiting_since = NULL, waiting_listed_at = NULL, updated_at = now() WHERE id = ? AND status <> 'resolved'`, by, ticket.ID)
	if res.Error != nil {
		return errs.Internal(res.Error)
	}
	if res.RowsAffected == 0 {
		return nil
	}
	s.endBot(ctx, conv.ID, "end")
	if actorID > 0 {
		s.event(ctx, nil, conv, ticket.ID, actorID, s.userName(ctx, actorID)+" sohbeti çözüldü olarak kapattı.")
	}
	fresh := s.ticketFresh(ctx, ticket.ID)
	if ch, err := s.channel(ctx, conv.ChannelID); err == nil && fresh != nil {
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
		return errs.Forbidden("Sohbeti yeniden açma yetkiniz yok.")
	}
	if ticket.Status != "resolved" {
		return nil
	}
	before := s.audience(ctx, ticket)
	res := s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET status = 'open', reopen_count = reopen_count + 1, resolved_at = NULL, updated_at = now() WHERE id = ? AND status = 'resolved'", ticket.ID)
	if res.Error != nil {
		return errs.Internal(res.Error)
	}
	if res.RowsAffected == 0 {
		return nil
	}
	s.event(ctx, nil, conv, ticket.ID, actorID, s.userName(ctx, actorID)+" sohbeti yeniden açtı.")
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
		return errs.Forbidden("Sohbeti düzenleme yetkiniz yok.")
	}
	fields := map[string]any{"updated_at": time.Now()}
	var lines []string
	name := s.userName(ctx, actorID)
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
	q := s.db.WithContext(ctx).Model(&models.WATicket{}).Where("id = ?", ticket.ID)
	if _, changesStatus := fields["status"]; changesStatus {
		// A status change must not bring back a ticket someone closed
		// in the meantime.
		q = q.Where("status <> 'resolved'")
	}
	res := q.Updates(fields)
	if res.Error != nil {
		return errs.Internal(res.Error)
	}
	if res.RowsAffected == 0 {
		return errs.Conflict("Sohbet bu arada kapatıldı. Değiştirmek için önce yeniden açın.", nil)
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
	for _, q := range []string{
		"DELETE FROM wa_webhook_events WHERE status = 'done' AND received_at < now() - interval '30 days'",
		"DELETE FROM wa_pending_statuses WHERE created_at < now() - interval '2 days'",
		"DELETE FROM wa_bot_events WHERE created_at < now() - interval '180 days'",
	} {
		if err := s.db.WithContext(ctx).Exec(q).Error; err != nil {
			slog.WarnContext(ctx, "whatsapp housekeeping failed", "query", q, "error", err)
		}
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
		if every("inbound", 30*time.Second) {
			safe.Run(ctx, "whatsapp follow-up sweep", func() { s.sweepInboundJobs(ctx) })
		}
		if every("media", 5*time.Minute) {
			safe.Run(ctx, "whatsapp media sweep", func() { s.sweepMedia(ctx) })
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
	var list []models.WATicket
	if err := s.db.WithContext(ctx).Where("awaiting_since IS NOT NULL AND waiting_listed_at IS NULL AND status IN ('open','pending')").Find(&list).Error; err != nil {
		return
	}
	settings := map[uint]ChannelSettings{}
	for i := range list {
		t := &list[i]
		set, ok := settings[t.ChannelID]
		if !ok {
			ch, err := s.channel(ctx, t.ChannelID)
			if err != nil {
				continue
			}
			set = parseSettings(ch.Settings)
			settings[t.ChannelID] = set
		}
		if set.WaitingMinutes <= 0 {
			continue
		}
		if set.Hours.Elapsed(*t.AwaitingSince, time.Now()) < time.Duration(set.WaitingMinutes)*time.Minute {
			continue
		}
		before := s.audience(ctx, t)
		res := s.db.WithContext(ctx).Exec("UPDATE wa_tickets SET waiting_listed_at = now(), waiting_count = waiting_count + 1 WHERE id = ? AND waiting_listed_at IS NULL AND awaiting_since IS NOT NULL", t.ID)
		if res.Error != nil || res.RowsAffected == 0 {
			continue
		}
		s.publish(ctx, t.ConversationID, nil, before)
		// A short signal for those who can help, so the list is not missed.
		fresh := s.ticketFresh(ctx, t.ID)
		viewers, _ := s.loadViewers(ctx)
		parts := s.participantSet(ctx, t.ID)
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
	var list []models.WATicket
	if err := s.db.WithContext(ctx).Where("owner_id IS NULL AND status IN ('open','pending')").Order("id").Limit(200).Find(&list).Error; err != nil {
		return
	}
	chans := map[uint]*models.WAChannel{}
	for i := range list {
		t := &list[i]
		ch, ok := chans[t.ChannelID]
		if !ok {
			c, err := s.channel(ctx, t.ChannelID)
			if err != nil {
				continue
			}
			ch = c
			chans[t.ChannelID] = ch
		}
		if !parseSettings(ch.Settings).Distribution.Enabled {
			continue
		}
		if !s.distribute(ctx, ch, t.ID) {
			// Nobody is free on this device; the rest can wait too.
			chans[t.ChannelID] = &models.WAChannel{Settings: `{"distribution":{"enabled":false}}`}
		}
	}
}
