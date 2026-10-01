package store

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/postgresql"
)

// LoadTicket reads a ticket and returns the database error as it is.
func (r *Repository) LoadTicket(ctx context.Context, id uint) (*models.WATicket, error) {
	var t models.WATicket
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// TicketsByIDs reads the tickets with the given ids, in no particular order.
func (r *Repository) TicketsByIDs(ctx context.Context, ids []uint) ([]models.WATicket, error) {
	var list []models.WATicket
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&list).Error
	return list, err
}

// ParticipantsInJoinOrder reads who takes part in the given tickets, in the
// order they joined.
func (r *Repository) ParticipantsInJoinOrder(ctx context.Context, ticketIDs []uint) ([]models.WAParticipant, error) {
	var ps []models.WAParticipant
	if err := r.db.WithContext(ctx).Where("ticket_id IN ?", ticketIDs).Order("joined_at").Find(&ps).Error; err != nil {
		return nil, err
	}
	return ps, nil
}

// ParticipantsOfTickets reads who takes part in the given tickets, in no
// particular order.
func (r *Repository) ParticipantsOfTickets(ctx context.Context, ticketIDs []uint) ([]models.WAParticipant, error) {
	var ps []models.WAParticipant
	err := r.db.WithContext(ctx).Where("ticket_id IN ?", ticketIDs).Find(&ps).Error
	return ps, err
}

// LockTicketRow reads a ticket and holds its row until the transaction
// ends. Unlike LockTicket, a missing ticket reads as an empty one.
func LockTicketRow(tx *gorm.DB, id uint) (*models.WATicket, error) {
	var t models.WATicket
	if err := tx.Raw("SELECT * FROM wa_tickets WHERE id = ? FOR UPDATE", id).Scan(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTicket changes the given columns of a ticket.
func (r *Repository) UpdateTicket(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WATicket{}).Where("id = ?", id).Updates(fields).Error
}

// EditTicket changes the given columns of a ticket from the panel. With
// unlessResolved it leaves a ticket someone closed in the meantime alone.
// It returns how many tickets changed.
func (r *Repository) EditTicket(ctx context.Context, id uint, fields map[string]any, unlessResolved bool) (int64, error) {
	q := r.db.WithContext(ctx).Model(&models.WATicket{}).Where("id = ?", id)
	if unlessResolved {
		q = q.Where("status <> 'resolved'")
	}
	res := q.Updates(fields)
	return res.RowsAffected, res.Error
}

// ---------------------------------------------------------------- chatbot hand over

// SetTicketBot marks a ticket as held by a chatbot.
func (r *Repository) SetTicketBot(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET status = 'bot', updated_at = now() WHERE id = ?", id).Error
}

// UpdateBotTicket changes the given columns of a ticket only while a
// chatbot still holds it.
func (r *Repository) UpdateBotTicket(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WATicket{}).Where("id = ? AND status = 'bot'", id).Updates(fields).Error
}

// ReleaseBotTicket opens a conversation's ticket again if a chatbot holds it.
func (r *Repository) ReleaseBotTicket(ctx context.Context, conversationID uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET status = 'open' WHERE status = 'bot' AND conversation_id = ?", conversationID).Error
}

// OpenBotTicket opens a ticket again if a chatbot holds it, and returns
// how many tickets changed.
func (r *Repository) OpenBotTicket(ctx context.Context, id uint) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET status = 'open' WHERE id = ? AND status = 'bot'", id)
	return res.RowsAffected, res.Error
}

// StartAwaiting starts a ticket's waiting clock from now and takes it off
// the waiting list.
func (r *Repository) StartAwaiting(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET awaiting_since = now(), waiting_listed_at = NULL WHERE id = ?", id).Error
}

// ---------------------------------------------------------------- distribution

// EligibleAgents lists the members of a device who are active, on shift
// and available, optionally only those in a team. The one who got a
// ticket longest ago comes first.
func (r *Repository) EligibleAgents(ctx context.Context, channelID uint, teamID *uint) ([]uint, error) {
	q := `SELECT m.user_id FROM wa_channel_members m
		JOIN users u ON u.id = m.user_id AND u.active
		JOIN shifts sh ON sh.user_id = m.user_id AND sh.ended_at IS NULL
		LEFT JOIN agent_presence ap ON ap.user_id = m.user_id
		WHERE m.channel_id = ? AND COALESCE(ap.state, 'available') = 'available'`
	args := []any{channelID}
	if teamID != nil {
		q += " AND m.user_id IN (SELECT user_id FROM wa_team_members WHERE team_id = ?)"
		args = append(args, *teamID)
	}
	q += " GROUP BY m.user_id, m.last_assigned_at ORDER BY m.last_assigned_at NULLS FIRST, m.user_id"
	var ids []uint
	err := r.db.WithContext(ctx).Raw(q, args...).Scan(&ids).Error
	return ids, err
}

// OpenTicketCount counts the open and pending tickets a person owns.
func (r *Repository) OpenTicketCount(ctx context.Context, userID uint) (int64, error) {
	var open int64
	err := r.db.WithContext(ctx).Raw("SELECT count(*) FROM wa_tickets WHERE owner_id = ? AND status IN ('open','pending')", userID).Scan(&open).Error
	return open, err
}

// AutoAssign gives an unowned ticket to a person in one transaction: the
// person becomes its owner, their turn on the device moves to now and the
// hand over is logged. It reports false when someone else got the ticket
// first or it was closed or taken by a chatbot.
func (r *Repository) AutoAssign(ctx context.Context, ticketID, channelID, userID uint, teamID *uint) (bool, error) {
	ok := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("UPDATE wa_tickets SET owner_id = ?, updated_at = now() WHERE id = ? AND owner_id IS NULL AND status NOT IN ('resolved','bot')", userID, ticketID)
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		ok = true
		if err := tx.Exec("INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, 'owner') ON CONFLICT (ticket_id, user_id) DO UPDATE SET role = 'owner'", ticketID, userID).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE wa_channel_members SET last_assigned_at = now() WHERE channel_id = ? AND user_id = ?", channelID, userID).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, to_user, team_id) VALUES (?, 'auto', ?, ?)", ticketID, userID, teamID).Error
	})
	return ok, err
}

// ---------------------------------------------------------------- owners and helpers

// ClaimTicket makes a person the owner of an unowned ticket, opens it if a
// chatbot held it and logs the claim.
func ClaimTicket(tx *gorm.DB, ticketID, userID uint) error {
	if err := tx.Exec("UPDATE wa_tickets SET owner_id = ?, status = CASE WHEN status = 'bot' THEN 'open' ELSE status END, updated_at = now() WHERE id = ?", userID, ticketID).Error; err != nil {
		return err
	}
	return RecordClaim(tx, ticketID, userID)
}

// JoinTicket adds a person to a ticket in a role unless they already take
// part; joined is true when they were added.
func JoinTicket(tx *gorm.DB, ticketID, userID uint, role string) (bool, error) {
	res := tx.Exec(`INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, ?)
			ON CONFLICT (ticket_id, user_id) DO NOTHING`, ticketID, userID, role)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// RecordJoin logs that a person joined a ticket as a helper.
func RecordJoin(tx *gorm.DB, ticketID, userID uint) error {
	return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, to_user, by_user) VALUES (?, 'join', ?, ?)", ticketID, userID, userID).Error
}

// DemoteOwner turns a ticket's owner into a helper.
func DemoteOwner(tx *gorm.DB, ticketID, userID uint) error {
	return tx.Exec("UPDATE wa_ticket_participants SET role = 'helper' WHERE ticket_id = ? AND user_id = ?", ticketID, userID).Error
}

// SetTicketOwner gives a ticket a new owner.
func SetTicketOwner(tx *gorm.DB, ticketID, userID uint) error {
	return tx.Exec("UPDATE wa_tickets SET owner_id = ?, updated_at = now() WHERE id = ?", userID, ticketID).Error
}

// MakeOwner adds a person to a ticket as its owner, or raises them to
// owner if they already take part.
func MakeOwner(tx *gorm.DB, ticketID, userID uint) error {
	return tx.Exec(`INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, 'owner')
			ON CONFLICT (ticket_id, user_id) DO UPDATE SET role = 'owner'`, ticketID, userID).Error
}

// RecordTake logs that a person took a ticket over from its owner.
func RecordTake(tx *gorm.DB, ticketID uint, from *uint, userID uint) error {
	return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, from_user, to_user, by_user) VALUES (?, 'take', ?, ?, ?)", ticketID, from, userID, userID).Error
}

// MoveTicket sets a ticket's owner and team and opens it if a chatbot held it.
func MoveTicket(tx *gorm.DB, ticketID uint, owner, team *uint) error {
	return tx.Exec("UPDATE wa_tickets SET owner_id = ?, team_id = ?, status = CASE WHEN status = 'bot' THEN 'open' ELSE status END, updated_at = now() WHERE id = ?", owner, team, ticketID).Error
}

// RecordTransfer logs that a ticket was handed to a person or a team, with
// the note left for them.
func RecordTransfer(tx *gorm.DB, ticketID uint, from, to, team *uint, by uint, note string) error {
	return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, from_user, to_user, team_id, by_user, note) VALUES (?, 'transfer', ?, ?, ?, ?, ?)",
		ticketID, from, to, team, by, note).Error
}

// Greeted reports whether a person already sent their greeting on a ticket.
func (r *Repository) Greeted(ctx context.Context, ticketID, userID uint) (bool, error) {
	var greeted bool
	err := r.db.WithContext(ctx).Raw("SELECT greeted FROM wa_ticket_participants WHERE ticket_id = ? AND user_id = ?", ticketID, userID).Scan(&greeted).Error
	return greeted, err
}

// MarkGreeted records that a person sent their greeting on a ticket.
func (r *Repository) MarkGreeted(ctx context.Context, ticketID, userID uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_ticket_participants SET greeted = true WHERE ticket_id = ? AND user_id = ?", ticketID, userID).Error
}

// ---------------------------------------------------------------- answering

// TicketConversationID reads which conversation a ticket belongs to inside
// a transaction; it is zero when the ticket does not exist.
func TicketConversationID(tx *gorm.DB, id uint) (uint, error) {
	var conv uint
	err := tx.Raw("SELECT conversation_id FROM wa_tickets WHERE id = ?", id).Scan(&conv).Error
	return conv, err
}

// SetLongestWait records the longest time a ticket's customer waited.
func SetLongestWait(tx *gorm.DB, id uint, seconds int) error {
	return tx.Exec("UPDATE wa_tickets SET longest_wait_sec = ? WHERE id = ?", seconds, id).Error
}

// MarkTicketAnswered ends a ticket's wait, takes it off the waiting list,
// sets its status and owner and records the first response time if it has
// none. With reopen, it also counts a reopening and clears the closing time.
func MarkTicketAnswered(tx *gorm.DB, id uint, status string, ownerID uint, reopen bool) error {
	extra := ""
	if reopen {
		extra = ", reopen_count = reopen_count + 1, resolved_at = NULL"
	}
	return tx.Exec(`UPDATE wa_tickets SET awaiting_since = NULL, waiting_listed_at = NULL, status = ?, owner_id = ?,
		first_response_at = COALESCE(first_response_at, now()), updated_at = now()`+extra+` WHERE id = ?`, status, ownerID, id).Error
}

// RecordFirstReply adds a person to a ticket in a role, or keeps their
// role, and records when they first answered.
func RecordFirstReply(tx *gorm.DB, ticketID, userID uint, role string) error {
	return tx.Exec(`INSERT INTO wa_ticket_participants (ticket_id, user_id, role, first_reply_at) VALUES (?, ?, ?, now())
		ON CONFLICT (ticket_id, user_id) DO UPDATE SET first_reply_at = COALESCE(wa_ticket_participants.first_reply_at, now())`, ticketID, userID, role).Error
}

// RecordClaim logs that a person took an unowned ticket.
func RecordClaim(tx *gorm.DB, ticketID, userID uint) error {
	return tx.Exec("INSERT INTO wa_assignments (ticket_id, kind, to_user, by_user) VALUES (?, 'claim', ?, ?)", ticketID, userID, userID).Error
}

// ---------------------------------------------------------------- surveys and ratings

// RecentSurveys counts a customer's tickets that got a survey in the last hours.
func (r *Repository) RecentSurveys(ctx context.Context, contactID uint, hours int) (int64, error) {
	var recent int64
	err := r.db.WithContext(ctx).Raw("SELECT count(*) FROM wa_tickets WHERE contact_id = ? AND survey_sent_at > now() - make_interval(hours => ?)",
		contactID, hours).Scan(&recent).Error
	return recent, err
}

// MarkSurveySent records that a ticket's survey went out now.
func (r *Repository) MarkSurveySent(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET survey_sent_at = now() WHERE id = ?", id).Error
}

// SetRating stores a ticket's score, comment and per question answers as JSON.
func (r *Repository) SetRating(ctx context.Context, id uint, score int, comment, answers string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET rating = ?, rating_comment = ?, rating_answers = ?, rated_at = now() WHERE id = ?", score, comment, answers, id).Error
}

// SetRatingTexts stores the written answers of a ticket's survey form as JSON.
func (r *Repository) SetRatingTexts(ctx context.Context, id uint, texts string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET rating_texts = ? WHERE id = ?", texts, id).Error
}

// ---------------------------------------------------------------- closing and opening

// ResolveTicket closes a ticket unless it is closed already and returns how
// many tickets changed, so only the first of two people closing goes on.
func (r *Repository) ResolveTicket(ctx context.Context, id uint, by *uint) (int64, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE wa_tickets SET status = 'resolved', resolved_at = now(), resolved_by = ?,
		awaiting_since = NULL, waiting_listed_at = NULL, updated_at = now() WHERE id = ? AND status <> 'resolved'`, by, id)
	return res.RowsAffected, res.Error
}

// ReopenTicket opens a closed ticket again by hand and returns how many
// tickets changed.
func (r *Repository) ReopenTicket(ctx context.Context, id uint) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET status = 'open', reopen_count = reopen_count + 1, resolved_at = NULL, updated_at = now() WHERE id = ? AND status = 'resolved'", id)
	return res.RowsAffected, res.Error
}

// ---------------------------------------------------------------- timed work

// TicketsDueForWaitList lists the open and pending tickets whose customer
// is waiting for an answer and that are not on the waiting list yet.
func (r *Repository) TicketsDueForWaitList(ctx context.Context) ([]models.WATicket, error) {
	var list []models.WATicket
	if err := r.db.WithContext(ctx).Where("awaiting_since IS NOT NULL AND waiting_listed_at IS NULL AND status IN ('open','pending')").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// PutOnWaitList puts a ticket on the waiting list unless it is there
// already or nobody waits any more, and returns how many tickets changed.
func (r *Repository) PutOnWaitList(ctx context.Context, id uint) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET waiting_listed_at = now(), waiting_count = waiting_count + 1 WHERE id = ? AND waiting_listed_at IS NULL AND awaiting_since IS NOT NULL", id)
	return res.RowsAffected, res.Error
}

// PooledTickets lists up to 200 open or pending tickets nobody owns, oldest first.
func (r *Repository) PooledTickets(ctx context.Context) ([]models.WATicket, error) {
	var list []models.WATicket
	if err := r.db.WithContext(ctx).Where("owner_id IS NULL AND status IN ('open','pending')").Order("id").Limit(200).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// housekeepingQueries delete the rows nobody reads any more.
var housekeepingQueries = []string{
	"DELETE FROM wa_webhook_events WHERE status = 'done' AND received_at < now() - interval '30 days'",
	"DELETE FROM wa_pending_statuses WHERE created_at < now() - interval '2 days'",
	"DELETE FROM wa_bot_events WHERE created_at < now() - interval '180 days'",
}

// HousekeepingFailure is a clean up query that failed and why.
type HousekeepingFailure struct {
	Query string
	Err   error
}

// Housekeep deletes old processed webhook events, stale pending statuses
// and old chatbot steps. Every query runs even when an earlier one fails;
// the failures are returned in order. The first clean up of a big table
// may take minutes, longer than a request's query may.
func (r *Repository) Housekeep(ctx context.Context) []HousekeepingFailure {
	var failed []HousekeepingFailure
	for _, q := range housekeepingQueries {
		err := postgresql.Long(ctx, r.db, 10*time.Minute, func(tx *gorm.DB) error { return tx.Exec(q).Error })
		if err != nil {
			failed = append(failed, HousekeepingFailure{Query: q, Err: err})
		}
	}
	return failed
}

// ---------------------------------------------------------------- rule actions

// SetTicketTeam moves a ticket to a team.
func (r *Repository) SetTicketTeam(ctx context.Context, id, teamID uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET team_id = ? WHERE id = ?", teamID, id).Error
}

// SetOwnerIfNone gives a ticket an owner only if it has none.
func (r *Repository) SetOwnerIfNone(ctx context.Context, id, userID uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET owner_id = ? WHERE id = ? AND owner_id IS NULL", userID, id).Error
}

// AddOwner adds a person to a ticket as its owner inside a transaction,
// leaving them as they are if they already take part.
func AddOwner(tx *gorm.DB, ticketID, userID uint) error {
	return tx.Exec("INSERT INTO wa_ticket_participants (ticket_id, user_id, role) VALUES (?, ?, 'owner') ON CONFLICT DO NOTHING", ticketID, userID).Error
}

// AddOwner adds a person to a ticket as its owner, leaving them as they
// are if they already take part.
func (r *Repository) AddOwner(ctx context.Context, ticketID, userID uint) error {
	return AddOwner(r.db.WithContext(ctx), ticketID, userID)
}

// SetTicketTags replaces a ticket's tags with a JSON list.
func (r *Repository) SetTicketTags(ctx context.Context, id uint, tags string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET tags = ? WHERE id = ?", tags, id).Error
}

// SetTicketPriority sets a ticket's priority when it is one of low,
// normal, high and urgent; any other value changes nothing.
func (r *Repository) SetTicketPriority(ctx context.Context, id uint, priority string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET priority = ? WHERE id = ? AND ? IN ('low','normal','high','urgent')", priority, id, priority).Error
}

// SetTicketCategory sets a ticket's category.
func (r *Repository) SetTicketCategory(ctx context.Context, id uint, category string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_tickets SET category = ? WHERE id = ?", category, id).Error
}

// AwaitingTickets lists a device's open and pending tickets whose customer
// is waiting for an answer.
func (r *Repository) AwaitingTickets(ctx context.Context, channelID uint) ([]models.WATicket, error) {
	var list []models.WATicket
	err := r.db.WithContext(ctx).Where("channel_id = ? AND awaiting_since IS NOT NULL AND status IN ('open','pending')", channelID).Find(&list).Error
	return list, err
}
