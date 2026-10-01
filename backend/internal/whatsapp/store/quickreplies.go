package store

import (
	"context"
	"fmt"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// QuickReplies lists the ready answers by shortcut; with a device, only
// the ones switched on for it.
func (r *Repository) QuickReplies(ctx context.Context, channelID uint) ([]models.WAQuickReply, error) {
	q := r.db.WithContext(ctx).Order("shortcut")
	if channelID > 0 {
		q = q.Where("channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", channelID))
	}
	var list []models.WAQuickReply
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// QuickRepliesForSuggestions reads up to 40 ready answers switched on for
// a device, by shortcut.
func (r *Repository) QuickRepliesForSuggestions(ctx context.Context, channelID uint) ([]models.WAQuickReply, error) {
	var qs []models.WAQuickReply
	err := r.db.WithContext(ctx).Where("channel_ids @> ?::jsonb", fmt.Sprintf("[%d]", channelID)).Order("shortcut").Limit(40).Find(&qs).Error
	return qs, err
}

// CreateQuickReply stores a new ready answer.
func (r *Repository) CreateQuickReply(ctx context.Context, q *models.WAQuickReply) error {
	return r.db.WithContext(ctx).Create(q).Error
}

// UpdateQuickReply changes the given columns of a ready answer.
func (r *Repository) UpdateQuickReply(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WAQuickReply{}).Where("id = ?", id).Updates(fields).Error
}

// DeleteQuickReply removes a ready answer.
func (r *Repository) DeleteQuickReply(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.WAQuickReply{}, id).Error
}

// ShareQuickReplies switches a device's ready answers on for another
// device too, skipping a shortcut the other device already has. It
// returns how many answers it switched on.
func (r *Repository) ShareQuickReplies(ctx context.Context, from, to uint) (int64, error) {
	src, dst := fmt.Sprintf("[%d]", from), fmt.Sprintf("[%d]", to)
	res := r.db.WithContext(ctx).Exec(`UPDATE wa_quick_replies q SET channel_ids = q.channel_ids || ?::jsonb, updated_at = now()
			WHERE q.channel_ids @> ?::jsonb AND NOT q.channel_ids @> ?::jsonb
			AND NOT EXISTS (SELECT 1 FROM wa_quick_replies o WHERE o.id <> q.id AND lower(o.shortcut) = lower(q.shortcut) AND o.channel_ids @> ?::jsonb)`,
		dst, src, dst, dst)
	return res.RowsAffected, res.Error
}
