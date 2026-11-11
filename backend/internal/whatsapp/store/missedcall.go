package store

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// HasMissedCallRecovery reports whether a missed call with callUUID was already processed.
func (r *Repository) HasMissedCallRecovery(ctx context.Context, callUUID string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.WAMissedCallRecovery{}).
		Where("call_uuid = ?", callUUID).Count(&n).Error
	return n > 0, err
}

// RecentMissedCallRecoveries counts recovery messages sent to peerKey since a moment.
func (r *Repository) RecentMissedCallRecoveries(ctx context.Context, peerKey string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.WAMissedCallRecovery{}).
		Where("peer_key = ? AND status = 'sent' AND created_at >= ?", peerKey, since).
		Count(&n).Error
	return n, err
}

// LogMissedCallRecovery stores an audit record of a recovery attempt (sent, skipped, failed, or pending).
func (r *Repository) LogMissedCallRecovery(ctx context.Context, row *models.WAMissedCallRecovery) error {
	return r.db.WithContext(ctx).Create(row).Error
}

// MissedCallRecoveryByID loads a recovery record by primary key.
func (r *Repository) MissedCallRecoveryByID(ctx context.Context, id uint) (*models.WAMissedCallRecovery, error) {
	var row models.WAMissedCallRecovery
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// UpdateMissedCallRecovery updates status, note, etc. of a recovery record.
func (r *Repository) UpdateMissedCallRecovery(ctx context.Context, row *models.WAMissedCallRecovery) error {
	return r.db.WithContext(ctx).Save(row).Error
}

// ListMissedCallRecoveries retrieves recovery items joined with customer and channel names.
func (r *Repository) ListMissedCallRecoveries(ctx context.Context, status string, limit int) ([]models.WAMissedCallRecoveryItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Table("wa_missed_call_recoveries r").
		Select(`r.id, r.call_uuid, r.peer_number, r.peer_key, r.channel_id, r.template, r.status, r.note, r.created_at,
		COALESCE((SELECT COALESCE(NULLIF(c.name, ''), NULLIF(c.profile_name, '')) FROM wa_contacts c WHERE c.peer_key = r.peer_key ORDER BY c.id DESC LIMIT 1), '') AS customer_name,
		COALESCE(ch.name, '') AS channel_name`).
		Joins("LEFT JOIN wa_channels ch ON ch.id = r.channel_id")

	if status != "" {
		q = q.Where("r.status = ?", status)
	}
	var rows []models.WAMissedCallRecoveryItem
	err := q.Order("r.created_at DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}
