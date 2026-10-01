package store

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// CreateMessage stores a message or a history line inside a transaction
// and fills in its id.
func CreateMessage(tx *gorm.DB, m *models.WAMessage) error {
	return tx.Create(m).Error
}

// CreateMessage stores a message or a history line and fills in its id.
func (r *Repository) CreateMessage(ctx context.Context, m *models.WAMessage) error {
	return CreateMessage(r.db.WithContext(ctx), m)
}

// LoadMessage reads a message.
func (r *Repository) LoadMessage(ctx context.Context, id uint) (*models.WAMessage, error) {
	var msg models.WAMessage
	if err := r.db.WithContext(ctx).First(&msg, id).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

// ReloadMessage reads a message again into msg, by its id.
func (r *Repository) ReloadMessage(ctx context.Context, msg *models.WAMessage) error {
	return r.db.WithContext(ctx).First(msg, msg.ID).Error
}

// MessageInConversation reads a message only if it belongs to the conversation.
func (r *Repository) MessageInConversation(ctx context.Context, id, conversationID uint) (*models.WAMessage, error) {
	var target models.WAMessage
	if err := r.db.WithContext(ctx).Where("id = ? AND conversation_id = ?", id, conversationID).First(&target).Error; err != nil {
		return nil, err
	}
	return &target, nil
}

// MessageByClientID reads the message the panel sent with a client id.
func (r *Repository) MessageByClientID(ctx context.Context, clientID string) (*models.WAMessage, error) {
	var existing models.WAMessage
	if err := r.db.WithContext(ctx).Where("client_id = ?", clientID).First(&existing).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}

// MessageByWAMID reads the message with a WhatsApp message id. A missing
// message reads as an empty one with id zero.
func (r *Repository) MessageByWAMID(ctx context.Context, wamid string) (*models.WAMessage, error) {
	var msg models.WAMessage
	if err := r.db.WithContext(ctx).Where("wamid = ?", wamid).Limit(1).Find(&msg).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

// UpdateMessage changes the given columns of a message.
func (r *Repository) UpdateMessage(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WAMessage{}).Where("id = ?", id).Updates(fields).Error
}

// SetMessageMedia replaces what a message records about its file.
func (r *Repository) SetMessageMedia(ctx context.Context, id uint, media string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_messages SET media = ? WHERE id = ?", media, id).Error
}

// RecentChat reads up to 30 messages of a conversation that went to or
// came from the customer, reactions left out, newest first.
func (r *Repository) RecentChat(ctx context.Context, conversationID uint) ([]models.WAMessage, error) {
	var msgs []models.WAMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ? AND direction IN ('in','out') AND kind <> 'reaction'", conversationID).
		Order("id DESC").Limit(30).Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

// ConversationMessages reads every message of a conversation, reactions
// left out, oldest first.
func (r *Repository) ConversationMessages(ctx context.Context, conversationID uint) ([]models.WAMessage, error) {
	var msgs []models.WAMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction'", conversationID).Order("id").Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

// InboundSince counts the customer's messages in a conversation after a
// moment, leaving out reactions and answers to a rating list.
func (r *Repository) InboundSince(ctx context.Context, conversationID uint, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM wa_messages WHERE conversation_id = ? AND direction = 'in' AND created_at > ?
		AND kind <> 'reaction' AND COALESCE(payload->>'id', '') NOT LIKE '%rate-%'`, conversationID, since).Scan(&n).Error
	return n, err
}

// ---------------------------------------------------------------- inbound

// CreateMessageOnce stores a customer's message inside a transaction unless
// one with the same WhatsApp message id is stored already. It returns how
// many rows it stored.
func CreateMessageOnce(tx *gorm.DB, msg *models.WAMessage) (int64, error) {
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "wamid"}}, DoNothing: true}).Create(msg)
	return res.RowsAffected, res.Error
}

// SetMessageTicket files a message under a ticket.
func SetMessageTicket(tx *gorm.DB, messageID, ticketID uint) error {
	return tx.Exec("UPDATE wa_messages SET ticket_id = ? WHERE id = ?", ticketID, messageID).Error
}

// NewInboundJob is the follow-up work of a customer message just stored.
type NewInboundJob struct {
	MessageID      uint
	ConversationID uint
	Created        bool
	Reopened       bool
	First          bool
	OptedOut       bool
	// ResolvedAt is when the ticket had been resolved before this message
	// reopened it; OwnerID who had it when the message came.
	ResolvedAt *time.Time
	OwnerID    *uint
}

// AddInboundJob records the follow-up work of a stored customer message,
// so a restart cannot lose it. It is ready to be taken at once.
func AddInboundJob(tx *gorm.DB, j NewInboundJob) error {
	return tx.Exec(`INSERT INTO wa_inbound_jobs (message_id, conversation_id, created, reopened, first, opted_out, resolved_at, owner_id, claimed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, '-infinity')`,
		j.MessageID, j.ConversationID, j.Created, j.Reopened, j.First, j.OptedOut, j.ResolvedAt, j.OwnerID).Error
}

// DeleteInboundJob removes a message's follow-up work once it is done.
func (r *Repository) DeleteInboundJob(ctx context.Context, messageID uint) error {
	return r.db.WithContext(ctx).Exec("DELETE FROM wa_inbound_jobs WHERE message_id = ?", messageID).Error
}

// InboundJob is a stored customer message whose follow-up work is not done:
// starting or continuing a chatbot, handing the chat out, automatic rules.
type InboundJob struct {
	MessageID uint
	Created   bool
	Reopened  bool
	First     bool
	OptedOut  bool
	Attempts  int
	// ResolvedAt is when the ticket had been resolved before this message
	// reopened it; OwnerID who had it when the message came.
	ResolvedAt *time.Time
	OwnerID    *uint
	// Steps lists the steps already done, comma separated.
	Steps string
}

// TakeInboundJob takes the oldest follow-up job last taken before a moment
// by moving its clock and counting the try, and returns it (or nothing).
// Jobs another worker holds are skipped, and only the oldest waiting
// message of a conversation can be taken.
func (r *Repository) TakeInboundJob(ctx context.Context, takenBefore time.Time) ([]InboundJob, error) {
	var jobs []InboundJob
	err := r.db.WithContext(ctx).Raw(`WITH due AS (
			SELECT j.message_id FROM wa_inbound_jobs j
			WHERE j.claimed_at < ?
			  AND NOT EXISTS (SELECT 1 FROM wa_inbound_jobs o WHERE o.conversation_id = j.conversation_id AND o.message_id < j.message_id)
			ORDER BY j.message_id LIMIT 1
			FOR UPDATE OF j SKIP LOCKED)
		UPDATE wa_inbound_jobs j SET claimed_at = now(), attempts = j.attempts + 1
		FROM due WHERE j.message_id = due.message_id
		RETURNING j.message_id, j.created, j.reopened, j.first, j.opted_out, j.attempts, j.resolved_at, j.owner_id, j.steps`,
		takenBefore).Scan(&jobs).Error
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// RenewInboundJob moves a running job's clock, so no other worker takes it.
func (r *Repository) RenewInboundJob(ctx context.Context, messageID uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_inbound_jobs SET claimed_at = now() WHERE message_id = ?", messageID).Error
}

// AddInboundStep records that a step of a job is done.
func (r *Repository) AddInboundStep(ctx context.Context, messageID uint, step string) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_inbound_jobs SET steps = steps || ? WHERE message_id = ?", step+",", messageID).Error
}

// UnkeptFile is a customer's message whose file was never kept.
type UnkeptFile struct {
	ID        uint
	ChannelID uint
}

// UnkeptFiles lists up to 10 customer messages written between two moments
// whose file is known to Meta but was neither kept nor marked failed.
func (r *Repository) UnkeptFiles(ctx context.Context, from, to time.Time) ([]UnkeptFile, error) {
	var rows []UnkeptFile
	err := r.db.WithContext(ctx).Raw(`SELECT id, channel_id FROM wa_messages
		WHERE direction = 'in' AND media IS NOT NULL
		  AND COALESCE(media->>'metaId', '') <> '' AND COALESCE(media->>'storeId', '') = ''
		  AND COALESCE(media->>'failed', '') = '' AND COALESCE((media->>'size')::bigint, 0) = 0
		  AND created_at BETWEEN ? AND ?
		ORDER BY id LIMIT 10`, from, to).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ---------------------------------------------------------------- statuses

// AddPendingStatus keeps a delivery status that came before the message's
// id was stored.
func (r *Repository) AddPendingStatus(ctx context.Context, wamid, status, payload string) error {
	return r.db.WithContext(ctx).Exec("INSERT INTO wa_pending_statuses (wamid, status, payload) VALUES (?, ?, ?) ON CONFLICT DO NOTHING", wamid, status, payload).Error
}

// PendingStatus is a delivery status kept until its message was known.
type PendingStatus struct {
	Payload string
}

// TakePendingStatuses removes and returns the statuses kept for a message.
func (r *Repository) TakePendingStatuses(ctx context.Context, wamid string) ([]PendingStatus, error) {
	var rows []PendingStatus
	err := r.db.WithContext(ctx).Raw("DELETE FROM wa_pending_statuses WHERE wamid = ? RETURNING payload", wamid).Scan(&rows).Error
	return rows, err
}

// CatchUpTicks moves the earlier sent messages of a conversation that are
// in one of the lower states to a delivered or read status, keeping the
// times they already have.
func (r *Repository) CatchUpTicks(ctx context.Context, conversationID, beforeID uint, status string, at time.Time, lower []string) error {
	return r.db.WithContext(ctx).Exec(`UPDATE wa_messages SET status = ?,
			delivered_at = COALESCE(delivered_at, ?),
			read_at = CASE WHEN ? = 'read' THEN COALESCE(read_at, ?) ELSE read_at END
			WHERE conversation_id = ? AND direction = 'out' AND id < ? AND wamid IS NOT NULL AND status IN ?`,
		status, at, status, at, conversationID, beforeID, lower).Error
}

// ---------------------------------------------------------------- the queue

// TakeDueMessages marks up to 20 queued messages whose time has come as
// being sent and returns them. Rows another worker holds are skipped, and
// a message waits while an earlier one of its conversation is queued or
// being sent.
func (r *Repository) TakeDueMessages(ctx context.Context) ([]models.WAMessage, error) {
	var list []models.WAMessage
	err := r.db.WithContext(ctx).Raw(`WITH due AS (
			SELECT m.id FROM wa_messages m
			WHERE m.status = 'queued' AND m.next_try_at <= now()
			  AND NOT EXISTS (SELECT 1 FROM wa_messages p WHERE p.conversation_id = m.conversation_id AND p.status IN ('queued','sending') AND p.id < m.id)
			ORDER BY m.id LIMIT 20
			FOR UPDATE OF m SKIP LOCKED)
		UPDATE wa_messages SET status = 'sending', sending_at = now()
		FROM due WHERE wa_messages.id = due.id
		RETURNING wa_messages.*`).Scan(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// RequeueSending puts messages taken for sending back into the queue,
// those still being sent only.
func (r *Repository) RequeueSending(ctx context.Context, ids []uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_messages SET status = 'queued', sending_at = NULL WHERE id IN ? AND status = 'sending'", ids).Error
}

// FailStaleSends marks the messages taken for sending before a moment as
// failed with a note, and returns them.
func (r *Repository) FailStaleSends(ctx context.Context, note string, takenBefore time.Time) ([]models.WAMessage, error) {
	var stale []models.WAMessage
	err := r.db.WithContext(ctx).Raw(`UPDATE wa_messages SET status = 'failed', failed_at = now(), error_text = ?
		WHERE status = 'sending' AND sending_at < ? RETURNING *`,
		note, takenBefore).Scan(&stale).Error
	if err != nil {
		return nil, err
	}
	return stale, nil
}

// The outcomes below change a message only while it is still being sent,
// so they never overwrite a later state. Each returns how many messages
// changed.

// SendBroke marks a message failed because sending it broke unexpectedly.
func (r *Repository) SendBroke(ctx context.Context, id uint, text string) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_messages SET status = 'failed', failed_at = now(), error_text = ?, attempts = attempts + 1 WHERE id = ? AND status = 'sending'",
		text, id)
	return res.RowsAffected, res.Error
}

// SendFailed marks a message failed with Meta's error code and a reason.
func (r *Repository) SendFailed(ctx context.Context, id uint, code int, text string) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_messages SET status = 'failed', failed_at = now(), error_code = ?, error_text = ?, attempts = attempts + 1 WHERE id = ? AND status = 'sending'", code, text, id)
	return res.RowsAffected, res.Error
}

// SendLater queues a message again for a later try.
func (r *Repository) SendLater(ctx context.Context, id uint, attempts int, next time.Time, text string) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_messages SET status = 'queued', attempts = ?, next_try_at = ?, error_text = ? WHERE id = ? AND status = 'sending'", attempts, next, text, id)
	return res.RowsAffected, res.Error
}

// SendDone marks a message sent with the id Meta gave it.
func (r *Repository) SendDone(ctx context.Context, id uint, wamid string) (int64, error) {
	res := r.db.WithContext(ctx).Exec("UPDATE wa_messages SET wamid = ?, status = 'sent', sent_at = now(), attempts = attempts + 1, error_text = '' WHERE id = ? AND status = 'sending'", wamid, id)
	return res.RowsAffected, res.Error
}

// RequeueMessage queues a failed message again from the start.
func (r *Repository) RequeueMessage(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_messages SET status = 'queued', next_try_at = now(), attempts = 0, error_code = NULL, error_text = '', failed_at = NULL WHERE id = ?", id).Error
}

// MessagesByIDs reads the messages with the given ids, in no particular order.
func (r *Repository) MessagesByIDs(ctx context.Context, ids []uint) ([]models.WAMessage, error) {
	var list []models.WAMessage
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// MessagesByWAMIDs reads the messages with the given WhatsApp message ids.
func (r *Repository) MessagesByWAMIDs(ctx context.Context, wamids []string) ([]models.WAMessage, error) {
	var qs []models.WAMessage
	if err := r.db.WithContext(ctx).Where("wamid IN ?", wamids).Find(&qs).Error; err != nil {
		return nil, err
	}
	return qs, nil
}

// Reactions lists every reaction in the given conversations, oldest first.
func (r *Repository) Reactions(ctx context.Context, conversationIDs []uint) ([]models.WAMessage, error) {
	var rs []models.WAMessage
	if err := r.db.WithContext(ctx).Where("conversation_id IN ? AND kind = 'reaction'", conversationIDs).Order("id").Find(&rs).Error; err != nil {
		return nil, err
	}
	return rs, nil
}

// MessagesUpTo reads up to limit messages of a conversation, reactions
// left out, with an id up to and including id, newest first.
func (r *Repository) MessagesUpTo(ctx context.Context, conversationID, id uint, limit int) ([]models.WAMessage, error) {
	var older []models.WAMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction' AND id <= ?", conversationID, id).Order("id DESC").Limit(limit).Find(&older).Error; err != nil {
		return nil, err
	}
	return older, nil
}

// MessagesAfter reads up to limit messages of a conversation, reactions
// left out, with an id above id, oldest first.
func (r *Repository) MessagesAfter(ctx context.Context, conversationID, id uint, limit int) ([]models.WAMessage, error) {
	var newer []models.WAMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction' AND id > ?", conversationID, id).Order("id").Limit(limit).Find(&newer).Error; err != nil {
		return nil, err
	}
	return newer, nil
}

// NewerMessages reads up to limit messages of a conversation, reactions
// left out, with an id above after, oldest first.
func (r *Repository) NewerMessages(ctx context.Context, conversationID, after uint, limit int) ([]models.WAMessage, error) {
	var list []models.WAMessage
	if err := r.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction'", conversationID).
		Where("id > ?", after).Order("id").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// MessagesBefore reads up to limit messages of a conversation, reactions
// left out, newest first: the latest ones when before is zero, otherwise
// those with an id below before.
func (r *Repository) MessagesBefore(ctx context.Context, conversationID, before uint, limit int) ([]models.WAMessage, error) {
	q := r.db.WithContext(ctx).Where("conversation_id = ? AND kind <> 'reaction'", conversationID)
	if before > 0 {
		q = q.Where("id < ?", before)
	}
	var desc []models.WAMessage
	if err := q.Order("id DESC").Limit(limit).Find(&desc).Error; err != nil {
		return nil, err
	}
	return desc, nil
}

// SearchMessages finds up to 200 messages, notes included, whose text
// matches an ILIKE pattern in conversations whose ticket the person sees,
// newest first; only in one conversation when conversationID is not zero.
func (r *Repository) SearchMessages(ctx context.Context, reach Reach, pattern string, conversationID uint) ([]models.WAMessage, error) {
	seen, seenArgs := reach.tickets()
	q := r.db.WithContext(ctx).Model(&models.WAMessage{}).Where("body ILIKE ? AND direction IN ('in','out','note')", pattern).
		Where("conversation_id IN (SELECT c.id FROM wa_conversations c WHERE c.ticket_id IN ("+seen+"))", seenArgs...)
	if conversationID > 0 {
		q = q.Where("conversation_id = ?", conversationID)
	}
	var list []models.WAMessage
	if err := q.Order("id DESC").Limit(200).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
