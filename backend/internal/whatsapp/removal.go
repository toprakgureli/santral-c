package whatsapp

import (
	"context"
	"fmt"
	"strconv"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Removing a device or a chatbot keeps its history. One that has history is
// turned off instead of deleted. The panel has no way to delete that history:
// conversations, messages and reports outlive the device or chatbot.

// RemoveResult says what removing a device or chatbot did.
type RemoveResult struct {
	// Deactivated is true when the item had history and was turned off and
	// kept instead of deleted.
	Deactivated bool `json:"deactivated"`
}

// RemoveChannel deletes a device that never had a conversation, and turns off
// one that did, so its conversations stay readable.
func (s *Service) RemoveChannel(ctx context.Context, actorID, id uint, ip string) (*RemoveResult, error) {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz silme yetkin yok."); err != nil {
		return nil, err
	}
	ch, err := s.repo.Channel(ctx, id)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.ChannelHasConversations(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	res := &RemoveResult{Deactivated: used}
	action := enums.AuditWAChannelDeleted
	if used {
		action = enums.AuditWAChannelDeactivated
		err = s.repo.DeactivateChannel(ctx, id)
	} else {
		err = s.repo.DeleteChannel(ctx, id)
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	s.forget()
	s.forgetHookPaths()
	s.record(ctx, actorID, action, "wa_channel", id, ip, map[string]any{"name": ch.Name, "phone": ch.DisplayPhone})
	return res, nil
}

// RemoveBot deletes a chatbot that was never published or run, and turns off
// one that was, so its versions and reports stay. Customers inside the flow
// go to a person either way.
func (s *Service) RemoveBot(ctx context.Context, actorID, id uint, ip string) (*RemoveResult, error) {
	if _, err := s.require(ctx, actorID, enums.WABotPublish, "Chatbot silme yetkin yok."); err != nil {
		return nil, err
	}
	b, err := s.repo.Bot(ctx, id)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.BotHasHistory(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	// Turn it off first so no new customer enters while the ones inside are
	// handed to a person.
	if err := s.repo.DeactivateBot(ctx, id); err != nil {
		return nil, errs.Internal(err)
	}
	if err := s.endBotSessions(ctx, id, "Chatbot kaldırıldı."); err != nil {
		return nil, err
	}
	action := enums.AuditWABotDeactivated
	if !used {
		action = enums.AuditWABotDeleted
		if err := s.repo.DeleteBot(ctx, id); err != nil {
			return nil, errs.Internal(err)
		}
	}
	s.record(ctx, actorID, action, "wa_bot", id, ip, map[string]any{"name": b.Name})
	return &RemoveResult{Deactivated: used}, nil
}

// endBotSessions hands every customer inside the chatbot to a person.
func (s *Service) endBotSessions(ctx context.Context, botID uint, note string) error {
	convs, err := s.repo.BotSessionConversations(ctx, botID)
	if err != nil {
		return errs.Internal(fmt.Errorf("chatbot sessions could not be listed: %w", err))
	}
	for _, c := range convs {
		conv, ticket, err := s.repo.Conversation(ctx, c)
		if err != nil || ticket == nil {
			continue
		}
		// The session goes first, so the chatbot timeout never finds it
		// later and closes a ticket a person is now working on.
		s.endBot(ctx, conv.ID, "handoff")
		if ch, err := s.repo.Channel(ctx, conv.ChannelID); err == nil {
			s.botToHuman(ctx, ch, conv, ticket, 0, note)
		}
	}
	return nil
}

// record writes an audit entry for a WhatsApp settings change.
func (s *Service) record(ctx context.Context, actorID uint, action, targetType string, id uint, ip string, detail map[string]any) {
	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   strconv.FormatUint(uint64(id), 10),
		IP:         ip,
		Detail:     detail,
	})
}
