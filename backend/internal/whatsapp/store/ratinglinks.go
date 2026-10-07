package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// CreateRatingLink stores a new link to the ratings.
func (r *Repository) CreateRatingLink(ctx context.Context, l *models.WARatingLink) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// RatingLink loads one link; found is false when there is none.
func (r *Repository) RatingLink(ctx context.Context, id uint) (*models.WARatingLink, bool, error) {
	var l models.WARatingLink
	err := r.db.WithContext(ctx).First(&l, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &l, true, nil
}

// RatingLinks lists the links still working and those that stopped since
// the given time, newest first.
func (r *Repository) RatingLinks(ctx context.Context, endedSince time.Time) ([]models.WARatingLink, error) {
	var out []models.WARatingLink
	err := r.db.WithContext(ctx).
		Where("COALESCE(revoked_at, expires_at) >= ?", endedSince).
		Order("created_at DESC").Limit(200).Find(&out).Error
	return out, err
}

// RevokeRatingLink cancels a link that still works. It reports false when
// the link is unknown, already cancelled or already past its end.
func (r *Repository) RevokeRatingLink(ctx context.Context, id, by uint) (bool, error) {
	res := r.db.WithContext(ctx).Model(&models.WARatingLink{}).
		Where("id = ? AND revoked_at IS NULL AND expires_at > now()", id).
		Updates(map[string]any{"revoked_at": gorm.Expr("now()"), "revoked_by": by})
	return res.RowsAffected > 0, res.Error
}

// RatingLinkOpened counts one opening of a link.
func (r *Repository) RatingLinkOpened(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&models.WARatingLink{}).Where("id = ?", id).
		Updates(map[string]any{"open_count": gorm.Expr("open_count + 1"), "last_opened_at": gorm.Expr("now()")}).Error
}
