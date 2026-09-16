package calllog

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Counts is a breakdown of a user's calls since a cut-off.
type Counts struct {
	Short      int64 `json:"short"`
	Long       int64 `json:"long"`
	Unanswered int64 `json:"unanswered"`
	Inbound    int64 `json:"inbound"`
	Outbound   int64 `json:"outbound"`
}

// Today returns a user's call logs since `from` plus their breakdown, split
// short/long by the shortLong threshold (seconds).
func (r *Repository) Today(ctx context.Context, userID uint, from time.Time, shortLong, limit int) ([]models.CallLog, Counts, error) {
	var counts Counts
	err := r.db.WithContext(ctx).
		Model(&models.CallLog{}).
		Select(
			"count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds < ?) AS short, "+
				"count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds >= ?) AS long, "+
				"count(*) FILTER (WHERE disposition NOT IN ('answered', 'in_progress')) AS unanswered, "+
				"count(*) FILTER (WHERE direction = 'inbound') AS inbound, "+
				"count(*) FILTER (WHERE direction = 'outbound') AS outbound",
			shortLong, shortLong).
		Where("user_id = ? AND started_at >= ?", userID, from).
		Scan(&counts).Error
	if err != nil {
		return nil, counts, fmt.Errorf("call logs could not be counted: %w", err)
	}
	var logs []models.CallLog
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND started_at >= ?", userID, from).
		Order("started_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, counts, fmt.Errorf("call logs could not be listed: %w", err)
	}
	return logs, counts, nil
}
