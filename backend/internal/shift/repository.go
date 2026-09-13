package shift

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository stores shifts.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a shift repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Open returns the user's open shift, or nil when off shift.
func (r *Repository) Open(ctx context.Context, userID uint) (*models.Shift, error) {
	var s models.Shift
	err := r.db.WithContext(ctx).Where("user_id = ? AND ended_at IS NULL", userID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open shift could not be read: %w", err)
	}
	return &s, nil
}

// Active reports whether the user has an open shift.
func (r *Repository) Active(ctx context.Context, userID uint) (bool, error) {
	s, err := r.Open(ctx, userID)
	return s != nil, err
}

// Start opens a shift for the user.
func (r *Repository) Start(ctx context.Context, userID uint, at time.Time) (*models.Shift, error) {
	s := models.Shift{UserID: userID, StartedAt: at}
	if err := r.db.WithContext(ctx).Create(&s).Error; err != nil {
		return nil, fmt.Errorf("shift could not be opened: %w", err)
	}
	return &s, nil
}

// End closes a shift. It is a no-op if the shift was already closed, so the
// user's click and the automatic close cannot both write.
func (r *Repository) End(ctx context.Context, id uint, at time.Time, by string) error {
	res := r.db.WithContext(ctx).Model(&models.Shift{}).
		Where("id = ? AND ended_at IS NULL", id).
		Updates(map[string]any{"ended_at": at, "ended_by": by})
	if res.Error != nil {
		return fmt.Errorf("shift could not be closed: %w", res.Error)
	}
	return nil
}

// AllOpen lists every open shift, for the automatic close.
func (r *Repository) AllOpen(ctx context.Context) ([]models.Shift, error) {
	var out []models.Shift
	if err := r.db.WithContext(ctx).Where("ended_at IS NULL").Order("started_at").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("open shifts could not be listed: %w", err)
	}
	return out, nil
}
