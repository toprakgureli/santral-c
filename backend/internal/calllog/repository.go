package calllog

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the call-log data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a call-log repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Get loads a call log by its client correlation id, or nil when absent.
func (r *Repository) Get(ctx context.Context, callID string) (*models.CallLog, error) {
	var log models.CallLog
	err := r.db.WithContext(ctx).Where("call_id = ?", callID).First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("call log could not be fetched: %w", err)
	}
	return &log, nil
}

// Create inserts a call log.
func (r *Repository) Create(ctx context.Context, log *models.CallLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("call log could not be created: %w", err)
	}
	return nil
}

// Update applies changed fields to a call log by id.
func (r *Repository) Update(ctx context.Context, id uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).
		Model(&models.CallLog{}).
		Where("id = ?", id).
		Updates(fields).Error; err != nil {
		return fmt.Errorf("call log could not be updated: %w", err)
	}
	return nil
}

// Recent returns the most recent call logs. When ownerID is set, only that
// user's calls are returned (own-scope).
func (r *Repository) Recent(ctx context.Context, ownerID *uint, limit int) ([]models.CallLog, error) {
	q := r.db.WithContext(ctx).Model(&models.CallLog{})
	if ownerID != nil {
		q = q.Where("user_id = ?", *ownerID)
	}
	var logs []models.CallLog
	if err := q.Order("started_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("call logs could not be listed: %w", err)
	}
	return logs, nil
}
