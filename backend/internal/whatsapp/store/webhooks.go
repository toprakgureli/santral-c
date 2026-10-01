package store

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// CreateWebhookEvent stores a notice from Meta for the worker.
func (r *Repository) CreateWebhookEvent(ctx context.Context, ev *models.WAWebhookEvent) error {
	return r.db.WithContext(ctx).Create(ev).Error
}

// PendingWebhookEvents reads up to 50 notices waiting to be handled whose
// next try is due, oldest first.
func (r *Repository) PendingWebhookEvents(ctx context.Context) ([]models.WAWebhookEvent, error) {
	var list []models.WAWebhookEvent
	if err := r.db.WithContext(ctx).Where("status = 'pending' AND next_try_at <= now()").Order("id").Limit(50).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// WebhookEventDone marks a notice handled.
func (r *Repository) WebhookEventDone(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_webhook_events SET status = 'done', processed_at = now(), attempts = attempts + 1, last_error = '' WHERE id = ?", id).Error
}

// WebhookEventFailed records a failed try of a notice: its new status, how
// many tries it had, why the last one failed and when to try again.
func (r *Repository) WebhookEventFailed(ctx context.Context, id uint, status string, attempts int, lastError string, nextTry time.Time) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_webhook_events SET status = ?, attempts = ?, last_error = ?, next_try_at = ? WHERE id = ?",
		status, attempts, lastError, nextTry, id).Error
}

// UnfinishedWebhookEvents reads up to 200 notices that are not handled
// yet, newest first.
func (r *Repository) UnfinishedWebhookEvents(ctx context.Context) ([]models.WAWebhookEvent, error) {
	var list []models.WAWebhookEvent
	if err := r.db.WithContext(ctx).Where("status <> 'done'").Order("id DESC").Limit(200).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// RetryWebhookEvent queues a notice that is not handled yet for a try now.
func (r *Repository) RetryWebhookEvent(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Exec("UPDATE wa_webhook_events SET status = 'pending', next_try_at = now() WHERE id = ? AND status <> 'done'", id).Error
}
