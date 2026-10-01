package store

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// ConversationsByIDs reads the conversations with the given ids, in no
// particular order.
func (r *Repository) ConversationsByIDs(ctx context.Context, ids []uint) ([]models.WAConversation, error) {
	var convs []models.WAConversation
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&convs).Error; err != nil {
		return nil, err
	}
	return convs, nil
}

// InboxConversations lists the conversations with a ticket for the inbox,
// only on the given devices unless channels is nil. With since above zero
// it returns up to 1000 rows changed after that version, oldest change
// first; otherwise up to 1500 rows whose ticket is open or was closed in
// the last three days, latest message first.
func (r *Repository) InboxConversations(ctx context.Context, channels []uint, since int64) ([]models.WAConversation, error) {
	q := r.db.WithContext(ctx).Model(&models.WAConversation{}).Where("ticket_id IS NOT NULL")
	if channels != nil {
		q = q.Where("channel_id IN ?", channels)
	}
	if since > 0 {
		q = q.Where("version > ?", since).Order("version").Limit(1000)
	} else {
		q = q.Where(`ticket_id IN (SELECT id FROM wa_tickets WHERE status <> 'resolved' OR resolved_at > now() - interval '3 days')`).
			Order("last_message_at DESC NULLS LAST").Limit(1500)
	}
	var convs []models.WAConversation
	if err := q.Find(&convs).Error; err != nil {
		return nil, err
	}
	return convs, nil
}

// MaxConversationVersion is the newest change version of any conversation,
// or zero when there is none.
func (r *Repository) MaxConversationVersion(ctx context.Context) (int64, error) {
	var top int64
	err := r.db.WithContext(ctx).Raw("SELECT COALESCE(max(version), 0) FROM wa_conversations").Scan(&top).Error
	return top, err
}

// ResolvedConversations lists up to 60 conversations whose ticket is
// closed, that the person sees and whose last message came before a
// moment, latest first. When nameLike is not empty, only customers whose
// lower case name or profile name matches nameLike, or whose number
// matches numberLike, count.
func (r *Repository) ResolvedConversations(ctx context.Context, reach Reach, before time.Time, nameLike, numberLike string) ([]models.WAConversation, error) {
	seen, seenArgs := reach.tickets()
	q := r.db.WithContext(ctx).Model(&models.WAConversation{}).
		Where("ticket_id IN (SELECT id FROM wa_tickets WHERE status = 'resolved')").
		Where("ticket_id IN ("+seen+")", seenArgs...).
		Where("last_message_at < ?", before)
	if nameLike != "" {
		q = q.Where("contact_id IN (SELECT id FROM wa_contacts WHERE lower(name) LIKE ? OR lower(profile_name) LIKE ? OR wa_id LIKE ?)", nameLike, nameLike, numberLike)
	}
	var convs []models.WAConversation
	if err := q.Order("last_message_at DESC").Limit(60).Find(&convs).Error; err != nil {
		return nil, err
	}
	return convs, nil
}

// SetLastMessage makes a message the latest of its conversation.
func SetLastMessage(tx *gorm.DB, conversationID, messageID uint) error {
	return tx.Exec("UPDATE wa_conversations SET last_message_id = ?, last_message_at = now() WHERE id = ?", messageID, conversationID).Error
}

// SetLastInbound makes a customer's message the latest of its conversation
// without counting it as unread.
func SetLastInbound(tx *gorm.DB, conversationID uint, inboundAt time.Time, messageID uint, at time.Time) error {
	return tx.Exec("UPDATE wa_conversations SET last_inbound_at = ?, last_message_id = ?, last_message_at = ? WHERE id = ?", inboundAt, messageID, at, conversationID).Error
}

// RecordInbound makes a customer's message the latest of its conversation,
// counts it as unread and sets the conversation's current ticket.
func RecordInbound(tx *gorm.DB, conversationID uint, inboundAt time.Time, messageID uint, at time.Time, ticketID uint) error {
	return tx.Exec(`UPDATE wa_conversations SET last_inbound_at = ?, last_message_id = ?, last_message_at = ?,
			unread = unread + 1, ticket_id = ? WHERE id = ?`, inboundAt, messageID, at, ticketID, conversationID).Error
}

// ---------------------------------------------------------------- reading

// ReadableMessageID is the newest message of a conversation at or before
// a message id, or zero. A read mark is moved only to a message of the
// conversation itself, never past its newest one.
func (r *Repository) ReadableMessageID(ctx context.Context, conversationID, messageID uint) (uint, error) {
	var id uint
	err := r.db.WithContext(ctx).Raw("SELECT COALESCE(max(id), 0) FROM wa_messages WHERE conversation_id = ? AND id <= ?", conversationID, messageID).Scan(&id).Error
	return id, err
}

// RecordRead stores how far a person read a conversation; it never moves
// back, except from a mark past the conversation's newest message.
func (r *Repository) RecordRead(ctx context.Context, conversationID, userID, messageID uint) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO wa_reads (conversation_id, user_id, message_id, read_at) VALUES (?, ?, ?, now())
		ON CONFLICT (conversation_id, user_id) DO UPDATE SET message_id = GREATEST(
			LEAST(wa_reads.message_id, (SELECT COALESCE(max(m.id), 0) FROM wa_messages m WHERE m.conversation_id = wa_reads.conversation_id)),
			EXCLUDED.message_id), read_at = now()`,
		conversationID, userID, messageID).Error
}

// MarkTeamRead moves the team's read mark of a conversation forward to a
// message and counts the customer's messages after it as unread. It
// returns how many conversations changed. A mark that points past the
// conversation's newest message (stored before marks were checked) is
// put right by the next read.
func (r *Repository) MarkTeamRead(ctx context.Context, conversationID, messageID uint) (int64, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE wa_conversations SET team_read_id = ?,
		unread = (SELECT count(*) FROM wa_messages m WHERE m.conversation_id = wa_conversations.id AND m.direction = 'in' AND m.kind <> 'reaction' AND m.id > ?)
		WHERE id = ? AND (team_read_id < ?
			OR team_read_id > (SELECT COALESCE(max(m.id), 0) FROM wa_messages m WHERE m.conversation_id = wa_conversations.id))`,
		messageID, messageID, conversationID, messageID)
	return res.RowsAffected, res.Error
}

// LastUnreceiptedInbound reads the customer's latest message with a
// WhatsApp id up to upTo and after afterID. A missing one reads as an
// empty message with id zero.
func (r *Repository) LastUnreceiptedInbound(ctx context.Context, conversationID, upTo, afterID uint) (*models.WAMessage, error) {
	var last models.WAMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ? AND direction = 'in' AND id <= ? AND id > ? AND wamid IS NOT NULL", conversationID, upTo, afterID).
		Order("id DESC").Limit(1).Find(&last).Error; err != nil {
		return nil, err
	}
	return &last, nil
}

// MarkMetaRead moves the mark of the latest message we sent blue ticks
// for forward, and returns how many conversations changed.
func (r *Repository) MarkMetaRead(ctx context.Context, conversationID, messageID uint) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_conversations SET meta_read_id = ? WHERE id = ? AND meta_read_id < ?", messageID, conversationID, messageID)
	return res.RowsAffected, res.Error
}

// LastInboundID is the id of the customer's latest message in a
// conversation, reactions left out, or zero.
func (r *Repository) LastInboundID(ctx context.Context, conversationID uint) (uint, error) {
	var lastIn uint
	err := r.db.WithContext(ctx).Raw("SELECT COALESCE(max(id), 0) FROM wa_messages WHERE conversation_id = ? AND direction = 'in' AND kind <> 'reaction'", conversationID).Scan(&lastIn).Error
	return lastIn, err
}

// MarkTeamUnread sets the team's read mark of a conversation and shows at
// least one unread message.
func (r *Repository) MarkTeamUnread(ctx context.Context, conversationID, teamReadID uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_conversations SET team_read_id = ?, unread = GREATEST(unread, 1) WHERE id = ?", teamReadID, conversationID).Error
}

// Read is how far one person read a conversation.
type Read struct {
	UserID    uint
	MessageID uint
	ReadAt    time.Time
}

// Reads lists who read a conversation how far, latest first.
func (r *Repository) Reads(ctx context.Context, conversationID uint) ([]Read, error) {
	var rows []Read
	if err := r.db.WithContext(ctx).Raw("SELECT user_id, message_id, read_at FROM wa_reads WHERE conversation_id = ? ORDER BY read_at DESC", conversationID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// BumpConversation moves a conversation's change version forward so
// reconnecting panels see the change.
func (r *Repository) BumpConversation(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_conversations SET version = nextval('wa_version_seq') WHERE id = ?", id).Error
}
