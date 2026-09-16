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
//
// A ring group rings every agent at once, so an inbound call one agent answers
// leaves a "missed" row on every other agent's log. Those rows are not this
// agent's missed calls: they are excluded from the counts and flagged on the
// list as answered elsewhere.
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
		Where("NOT (" + AnsweredElsewhereSQL + ")").
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

// AnsweredElsewhereSQL is true for an unanswered inbound row whose caller was
// answered by another agent within the ring window: the same call rang this
// agent too and someone else picked it up. It refers to the call_logs row
// under evaluation, so it must be used in a query on call_logs.
const AnsweredElsewhereSQL = "call_logs.direction = 'inbound' AND call_logs.disposition IN ('missed', 'no_answer', 'canceled') " +
	"AND EXISTS (SELECT 1 FROM call_logs o WHERE o.id <> call_logs.id AND o.peer_key = call_logs.peer_key " +
	"AND o.user_id IS DISTINCT FROM call_logs.user_id AND o.answered_at IS NOT NULL " +
	"AND o.started_at BETWEEN call_logs.started_at - interval '90 seconds' AND call_logs.started_at + interval '90 seconds')"

// AnsweredElsewhere returns the ids among the given logs that another agent
// answered, so the list can label them instead of showing them as missed.
func (r *Repository) AnsweredElsewhere(ctx context.Context, ids []uint) (map[uint]bool, error) {
	out := make(map[uint]bool)
	if len(ids) == 0 {
		return out, nil
	}
	var hits []uint
	err := r.db.WithContext(ctx).Model(&models.CallLog{}).
		Where("call_logs.id IN ?", ids).
		Where(AnsweredElsewhereSQL).
		Pluck("call_logs.id", &hits).Error
	if err != nil {
		return nil, fmt.Errorf("answered-elsewhere check failed: %w", err)
	}
	for _, id := range hits {
		out[id] = true
	}
	return out, nil
}
