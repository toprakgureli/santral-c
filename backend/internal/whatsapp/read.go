package whatsapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// MarkRead records that a person read a conversation up to a message.
// The unread badge is one for the whole team: when anyone reads, it drops
// for everyone at once. Who read what and when is kept separately. The
// customer sees blue ticks only if the device sends read receipts.
func (s *Service) MarkRead(ctx context.Context, actorID, conversationID, messageID uint) error {
	_, conv, _, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	if messageID == 0 && conv.LastMessageID != nil {
		messageID = *conv.LastMessageID
	}
	if messageID == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO wa_reads (conversation_id, user_id, message_id, read_at) VALUES (?, ?, ?, now())
		ON CONFLICT (conversation_id, user_id) DO UPDATE SET message_id = GREATEST(wa_reads.message_id, EXCLUDED.message_id), read_at = now()`,
		conversationID, actorID, messageID).Error; err != nil {
		return errs.Internal(err)
	}
	res := s.db.WithContext(ctx).Exec(`UPDATE wa_conversations SET team_read_id = ?,
		unread = (SELECT count(*) FROM wa_messages m WHERE m.conversation_id = wa_conversations.id AND m.direction = 'in' AND m.kind <> 'reaction' AND m.id > ?)
		WHERE id = ? AND team_read_id < ?`, messageID, messageID, conversationID, messageID)
	if res.Error != nil {
		return errs.Internal(res.Error)
	}
	if res.RowsAffected > 0 {
		s.publish(ctx, conversationID, nil, nil)
	}
	s.sendReadReceipt(ctx, conv, messageID)
	return nil
}

// sendReadReceipt gives the customer blue ticks for their latest message
// up to the one read, once.
func (s *Service) sendReadReceipt(ctx context.Context, conv *models.WAConversation, upTo uint) {
	ch, err := s.channel(ctx, conv.ChannelID)
	if err != nil || !parseSettings(ch.Settings).ReadReceipts {
		return
	}
	var last models.WAMessage
	if err := s.db.WithContext(ctx).Where("conversation_id = ? AND direction = 'in' AND id <= ? AND id > ? AND wamid IS NOT NULL", conv.ID, upTo, conv.MetaReadID).
		Order("id DESC").Limit(1).Find(&last).Error; err != nil || last.ID == 0 || last.WAMID == nil {
		return
	}
	// Meta only accepts it for messages from the last 30 days.
	if time.Since(last.CreatedAt) > 29*24*time.Hour {
		return
	}
	res := s.db.WithContext(ctx).Exec("UPDATE wa_conversations SET meta_read_id = ? WHERE id = ? AND meta_read_id < ?", last.ID, conv.ID, last.ID)
	if res.Error != nil || res.RowsAffected == 0 {
		return
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return
	}
	wamid := *last.WAMID
	go func() {
		c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := cl.MarkRead(c, wamid, false); err != nil {
			slog.Warn("whatsapp read receipt failed", "conversation", conv.ID, "error", err)
		}
	}()
}

// MarkUnread puts the badge back for the team.
func (s *Service) MarkUnread(ctx context.Context, actorID, conversationID uint) error {
	_, conv, _, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	var lastIn uint
	_ = s.db.WithContext(ctx).Raw("SELECT COALESCE(max(id), 0) FROM wa_messages WHERE conversation_id = ? AND direction = 'in' AND kind <> 'reaction'", conv.ID).Scan(&lastIn).Error
	if lastIn == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Exec("UPDATE wa_conversations SET team_read_id = ?, unread = GREATEST(unread, 1) WHERE id = ?", lastIn-1, conv.ID).Error; err != nil {
		return errs.Internal(err)
	}
	s.publish(ctx, conv.ID, nil, nil)
	return nil
}

// Typing tells the others looking at a chat that this person is writing,
// so two people do not answer the customer at once.
func (s *Service) Typing(ctx context.Context, actorID, conversationID uint) error {
	v, _, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	ids := s.audience(ctx, ticket)
	out := ids[:0]
	for _, id := range ids {
		if id != actorID {
			out = append(out, id)
		}
	}
	if len(out) > 0 {
		s.push.Push(out, Event{Type: "wa.typing", ConversationID: conversationID, UserID: actorID, Name: firstName(v.user.Name)})
	}
	return nil
}

// ReadInfo is who on our side read a conversation and when.
type ReadInfo struct {
	User      PersonView `json:"user"`
	MessageID uint       `json:"messageId"`
	ReadAt    time.Time  `json:"readAt"`
}

// Reads lists who read a conversation.
func (s *Service) Reads(ctx context.Context, actorID, conversationID uint) ([]ReadInfo, error) {
	if _, _, _, err := s.reachable(ctx, actorID, conversationID); err != nil {
		return nil, err
	}
	var rows []struct {
		UserID    uint
		MessageID uint
		ReadAt    time.Time
	}
	if err := s.db.WithContext(ctx).Raw("SELECT user_id, message_id, read_at FROM wa_reads WHERE conversation_id = ? ORDER BY read_at DESC", conversationID).Scan(&rows).Error; err != nil {
		return nil, errs.Internal(err)
	}
	var ids []uint
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	people := s.people(ctx, ids)
	out := make([]ReadInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReadInfo{User: people[r.UserID], MessageID: r.MessageID, ReadAt: r.ReadAt})
	}
	return out, nil
}

// Note writes an internal note only our side sees.
func (s *Service) Note(ctx context.Context, actorID, conversationID uint, body string) (*MessageView, error) {
	v, conv, ticket, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return nil, err
	}
	if !v.can(enums.WANote) {
		return nil, errs.Forbidden("İç not yazma yetkiniz yok.")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errs.Invalid("Not boş olamaz.", nil)
	}
	m := &models.WAMessage{ChannelID: conv.ChannelID, ConversationID: conv.ID, TicketID: uintPtr(ticket.ID), Direction: "note", Kind: "text",
		SenderKind: "agent", SenderUserID: uintPtr(actorID), Body: body, Status: "received", CreatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(m).Error; err != nil {
		return nil, errs.Internal(err)
	}
	s.publish(ctx, conv.ID, m, nil)
	views, err := s.messageViews(ctx, []models.WAMessage{*m})
	if err != nil || len(views) == 0 {
		return nil, errs.Internal(err)
	}
	return &views[0], nil
}
