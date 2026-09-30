package whatsapp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Removing a device or a chatbot keeps its history. One that has history is
// turned off instead of deleted; the history goes only through a purge, which
// needs the item to be off already and its name typed as confirmation.

// RemoveResult says what removing a device or chatbot did.
type RemoveResult struct {
	// Deactivated is true when the item had history and was turned off and
	// kept instead of deleted.
	Deactivated bool `json:"deactivated"`
}

// PurgeInput confirms a purge by repeating the item's name.
type PurgeInput struct {
	Confirm string `json:"confirm"`
}

// RemoveChannel deletes a device that never had a conversation, and turns off
// one that did, so its conversations stay readable.
func (s *Service) RemoveChannel(ctx context.Context, actorID, id uint, ip string) (*RemoveResult, error) {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz silme yetkiniz yok."); err != nil {
		return nil, err
	}
	ch, err := s.repo.Channel(ctx, id)
	if err != nil {
		return nil, err
	}
	used, err := s.exists(ctx, "SELECT 1 FROM wa_conversations WHERE channel_id = ? LIMIT 1", id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	res := &RemoveResult{Deactivated: used}
	action := enums.AuditWAChannelDeleted
	if used {
		action = enums.AuditWAChannelDeactivated
		err = s.db.WithContext(ctx).Model(&models.WAChannel{}).Where("id = ?", id).Update("active", false).Error
	} else {
		err = s.db.WithContext(ctx).Delete(&models.WAChannel{}, id).Error
	}
	if err != nil {
		return nil, errs.Internal(err)
	}
	s.forget()
	s.forgetHookPaths()
	s.record(ctx, actorID, action, "wa_channel", id, ip, map[string]any{"name": ch.Name, "phone": ch.DisplayPhone})
	return res, nil
}

// PurgeChannel deletes a turned-off device together with every conversation,
// ticket and message on it.
func (s *Service) PurgeChannel(ctx context.Context, actorID, id uint, confirm, ip string) error {
	if _, err := s.require(ctx, actorID, enums.WAChannelManage, "Cihaz silme yetkiniz yok."); err != nil {
		return err
	}
	ch, err := s.repo.Channel(ctx, id)
	if err != nil {
		return err
	}
	if err := checkPurge(ch.Active, ch.Name, confirm); err != nil {
		return err
	}
	var counts struct{ Conversations, Messages int64 }
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		steps := []struct {
			query string
			count *int64
		}{
			{"DELETE FROM wa_messages WHERE channel_id = ?", &counts.Messages},
			{"DELETE FROM wa_tickets WHERE channel_id = ?", nil},
			{"DELETE FROM wa_conversations WHERE channel_id = ?", &counts.Conversations},
			{"DELETE FROM wa_channels WHERE id = ?", nil},
		}
		for _, step := range steps {
			res := tx.Exec(step.query, id)
			if res.Error != nil {
				return fmt.Errorf("device history could not be purged: %w", res.Error)
			}
			if step.count != nil {
				*step.count = res.RowsAffected
			}
		}
		return nil
	})
	if err != nil {
		return errs.Internal(err)
	}
	s.forget()
	s.forgetHookPaths()
	s.record(ctx, actorID, enums.AuditWAChannelPurged, "wa_channel", id, ip, map[string]any{
		"name": ch.Name, "phone": ch.DisplayPhone, "conversations": counts.Conversations, "messages": counts.Messages,
	})
	return nil
}

// RemoveBot deletes a chatbot that was never published or run, and turns off
// one that was, so its versions and reports stay. Customers inside the flow
// go to a person either way.
func (s *Service) RemoveBot(ctx context.Context, actorID, id uint, ip string) (*RemoveResult, error) {
	if _, err := s.require(ctx, actorID, enums.WABotPublish, "Chatbot silme yetkiniz yok."); err != nil {
		return nil, err
	}
	b, err := s.repo.Bot(ctx, id)
	if err != nil {
		return nil, err
	}
	used, err := s.exists(ctx, `SELECT 1 FROM wa_bot_versions WHERE bot_id = ?
		UNION ALL SELECT 1 FROM wa_bot_events WHERE bot_id = ? LIMIT 1`, id, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	// Turn it off first so no new customer enters while the ones inside are
	// handed to a person.
	if err := s.db.WithContext(ctx).Model(&models.WABot{}).Where("id = ?", id).Update("active", false).Error; err != nil {
		return nil, errs.Internal(err)
	}
	if err := s.endBotSessions(ctx, id, "Chatbot kaldırıldı."); err != nil {
		return nil, err
	}
	action := enums.AuditWABotDeactivated
	if !used {
		action = enums.AuditWABotDeleted
		if err := s.db.WithContext(ctx).Delete(&models.WABot{}, id).Error; err != nil {
			return nil, errs.Internal(err)
		}
	}
	s.record(ctx, actorID, action, "wa_bot", id, ip, map[string]any{"name": b.Name})
	return &RemoveResult{Deactivated: used}, nil
}

// PurgeBot deletes a turned-off chatbot with its published versions and its
// report history.
func (s *Service) PurgeBot(ctx context.Context, actorID, id uint, confirm, ip string) error {
	if _, err := s.require(ctx, actorID, enums.WABotPublish, "Chatbot silme yetkiniz yok."); err != nil {
		return err
	}
	b, err := s.repo.Bot(ctx, id)
	if err != nil {
		return err
	}
	if err := checkPurge(b.Active, b.Name, confirm); err != nil {
		return err
	}
	if err := s.endBotSessions(ctx, id, "Chatbot silindi."); err != nil {
		return err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, q := range []string{
			"DELETE FROM wa_bot_events WHERE bot_id = ?",
			"DELETE FROM wa_bot_versions WHERE bot_id = ?",
			"DELETE FROM wa_bot_sessions WHERE bot_id = ?",
			"DELETE FROM wa_bots WHERE id = ?",
		} {
			if err := tx.Exec(q, id).Error; err != nil {
				return fmt.Errorf("chatbot history could not be purged: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return errs.Internal(err)
	}
	s.record(ctx, actorID, enums.AuditWABotPurged, "wa_bot", id, ip, map[string]any{"name": b.Name})
	return nil
}

// checkPurge allows a purge only on an item that is off and whose name was
// typed back.
func checkPurge(active bool, name, confirm string) error {
	if active {
		return errs.Invalid("Önce kapatın; tamamen silme yalnızca kapalı olanlarda yapılır.", nil)
	}
	if !strings.EqualFold(strings.TrimSpace(confirm), strings.TrimSpace(name)) {
		return errs.Invalid("Onay için adı aynen yazın.", nil)
	}
	return nil
}

// endBotSessions hands every customer inside the chatbot to a person.
func (s *Service) endBotSessions(ctx context.Context, botID uint, note string) error {
	var convs []uint
	if err := s.db.WithContext(ctx).Raw("SELECT conversation_id FROM wa_bot_sessions WHERE bot_id = ?", botID).Scan(&convs).Error; err != nil {
		return errs.Internal(fmt.Errorf("chatbot sessions could not be listed: %w", err))
	}
	for _, c := range convs {
		conv, ticket, err := s.repo.Conversation(ctx, c)
		if err != nil || ticket == nil {
			continue
		}
		if ch, err := s.repo.Channel(ctx, conv.ChannelID); err == nil {
			s.botToHuman(ctx, ch, conv, ticket, 0, note)
		}
	}
	return nil
}

// exists reports whether query returns a row.
func (s *Service) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var hits []int
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&hits).Error; err != nil {
		return false, fmt.Errorf("existence check failed: %w", err)
	}
	return len(hits) > 0, nil
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
