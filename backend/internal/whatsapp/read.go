package whatsapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/device"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
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
	if err := s.repo.RecordRead(ctx, conversationID, actorID, messageID); err != nil {
		return errs.Internal(err)
	}
	changed, err := s.repo.MarkTeamRead(ctx, conversationID, messageID)
	if err != nil {
		return errs.Internal(err)
	}
	if changed > 0 {
		s.publish(ctx, conversationID, nil, nil)
	}
	s.sendReadReceipt(ctx, conv, messageID)
	return nil
}

// sendReadReceipt gives the customer blue ticks for their latest message
// up to the one read, once.
func (s *Service) sendReadReceipt(ctx context.Context, conv *models.WAConversation, upTo uint) {
	ch, err := s.repo.Channel(ctx, conv.ChannelID)
	if err != nil || !device.Parse(ch.Settings).ReadReceipts {
		return
	}
	last, err := s.repo.LastUnreceiptedInbound(ctx, conv.ID, upTo, conv.MetaReadID)
	if err != nil || last.ID == 0 || last.WAMID == nil {
		return
	}
	// Meta only accepts it for messages from the last 30 days.
	if time.Since(last.CreatedAt) > 29*24*time.Hour {
		return
	}
	if changed, err := s.repo.MarkMetaRead(ctx, conv.ID, last.ID); err != nil || changed == 0 {
		return
	}
	cl, err := s.cloudFor(ch)
	if err != nil {
		return
	}
	wamid := *last.WAMID
	safe.Go(ctx, "whatsapp read receipt", func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if err := cl.MarkRead(c, wamid, false); err != nil {
			slog.WarnContext(c, "whatsapp read receipt failed", "conversation", conv.ID, "error", err)
		}
	})
}

// MarkUnread puts the badge back for the team.
func (s *Service) MarkUnread(ctx context.Context, actorID, conversationID uint) error {
	_, conv, _, err := s.reachable(ctx, actorID, conversationID)
	if err != nil {
		return err
	}
	lastIn, err := s.repo.LastInboundID(ctx, conv.ID)
	if err != nil {
		return errs.Internal(err)
	}
	if lastIn == 0 {
		return nil
	}
	if err := s.repo.MarkTeamUnread(ctx, conv.ID, lastIn-1); err != nil {
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
	rows, err := s.repo.Reads(ctx, conversationID)
	if err != nil {
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
		return nil, errs.Forbidden("İç not yazma yetkin yok.")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errs.Invalid("Not boş olamaz.", nil)
	}
	m := &models.WAMessage{ChannelID: conv.ChannelID, ConversationID: conv.ID, TicketID: uintPtr(ticket.ID), Direction: "note", Kind: "text",
		SenderKind: "agent", SenderUserID: uintPtr(actorID), Body: body, Status: "received", CreatedAt: time.Now()}
	if err := s.repo.CreateMessage(ctx, m); err != nil {
		return nil, errs.Internal(err)
	}
	s.publish(ctx, conv.ID, m, nil)
	views, err := s.messageViews(ctx, []models.WAMessage{*m})
	if err != nil || len(views) == 0 {
		return nil, errs.Internal(err)
	}
	return &views[0], nil
}
