// Package telephony turns Asterisk AMI activity into call records and serves
// the call log.
package telephony

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the call data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a telephony repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateCall inserts a new call and populates its id.
func (r *Repository) CreateCall(ctx context.Context, c *models.Call) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("call could not be created: %w", err)
	}
	return nil
}

// CallByLinkedid loads a call by its Asterisk linkedid, or nil when absent.
func (r *Repository) CallByLinkedid(ctx context.Context, linkedid string) (*models.Call, error) {
	var c models.Call
	err := r.db.WithContext(ctx).Where("linkedid = ?", linkedid).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("call could not be fetched by linkedid: %w", err)
	}
	return &c, nil
}

// UpdateCall applies a field patch to a call.
func (r *Repository) UpdateCall(ctx context.Context, id uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).
		Model(&models.Call{}).
		Where("id = ?", id).
		Updates(fields).Error; err != nil {
		return fmt.Errorf("call could not be updated: %w", err)
	}
	return nil
}

// AddEvent appends an entry to a call's timeline.
func (r *Repository) AddEvent(ctx context.Context, e *models.CallEvent) error {
	if err := r.db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("call event could not be recorded: %w", err)
	}
	return nil
}

// AddQuality stores an RTCP quality sample.
func (r *Repository) AddQuality(ctx context.Context, q *models.CallQuality) error {
	if err := r.db.WithContext(ctx).Create(q).Error; err != nil {
		return fmt.Errorf("call quality sample could not be recorded: %w", err)
	}
	return nil
}

// GetByID loads a call, or nil when absent.
func (r *Repository) GetByID(ctx context.Context, id uint) (*models.Call, error) {
	var c models.Call
	err := r.db.WithContext(ctx).First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("call could not be fetched by id: %w", err)
	}
	return &c, nil
}

// Events returns a call's timeline in order.
func (r *Repository) Events(ctx context.Context, callID uint) ([]models.CallEvent, error) {
	var events []models.CallEvent
	if err := r.db.WithContext(ctx).
		Where("call_id = ?", callID).
		Order("at ASC, seq ASC, id ASC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("call events could not be listed: %w", err)
	}
	return events, nil
}

// Quality returns a call's quality samples in order.
func (r *Repository) Quality(ctx context.Context, callID uint) ([]models.CallQuality, error) {
	var quality []models.CallQuality
	if err := r.db.WithContext(ctx).
		Where("call_id = ?", callID).
		Order("at ASC, id ASC").
		Find(&quality).Error; err != nil {
		return nil, fmt.Errorf("call quality could not be listed: %w", err)
	}
	return quality, nil
}

// UserIDByExtension resolves an active user id from a SIP extension.
func (r *Repository) UserIDByExtension(ctx context.Context, ext string) (*uint, error) {
	var id uint
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("sip_extension = ?", ext).
		Limit(1).
		Pluck("id", &id).Error
	if err != nil {
		return nil, fmt.Errorf("user could not be resolved by extension: %w", err)
	}
	if id == 0 {
		return nil, nil
	}
	return &id, nil
}

// ContactIDByNumber resolves a contact id from a normalized phone number.
func (r *Repository) ContactIDByNumber(ctx context.Context, e164 string) (*uint, error) {
	var id uint
	err := r.db.WithContext(ctx).
		Model(&models.ContactPhone{}).
		Where("number_e164 = ?", e164).
		Limit(1).
		Pluck("contact_id", &id).Error
	if err != nil {
		return nil, fmt.Errorf("contact could not be resolved by number: %w", err)
	}
	if id == 0 {
		return nil, nil
	}
	return &id, nil
}

// List returns a filtered, paginated page of calls and the total count.
func (r *Repository) List(ctx context.Context, f requests.CallFilter) ([]models.Call, int64, error) {
	q := r.db.WithContext(ctx).Model(&models.Call{})
	if f.Direction != "" {
		q = q.Where("direction = ?", f.Direction)
	}
	if f.Disposition != "" {
		q = q.Where("disposition = ?", f.Disposition)
	}
	if f.Number != "" {
		like := "%" + f.Number + "%"
		q = q.Where("from_number ILIKE ? OR to_number ILIKE ?", like, like)
	}
	if f.UserID != nil {
		q = q.Where("from_user_id = ? OR to_user_id = ?", *f.UserID, *f.UserID)
	}
	if f.OwnerID != nil {
		q = q.Where("from_user_id = ? OR to_user_id = ?", *f.OwnerID, *f.OwnerID)
	}
	if f.From != nil {
		q = q.Where("started_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("started_at <= ?", *f.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("calls could not be counted: %w", err)
	}

	var calls []models.Call
	if err := q.
		Order("started_at DESC").
		Limit(f.PerPage).
		Offset((f.Page - 1) * f.PerPage).
		Find(&calls).Error; err != nil {
		return nil, 0, fmt.Errorf("calls could not be listed: %w", err)
	}
	return calls, total, nil
}
