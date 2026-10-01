package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository stores refresh sessions.
type Repository struct {
	db *gorm.DB
}

var _ IRepository = (*Repository)(nil)

// NewRepository builds a session repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateSession inserts a new session.
func (r *Repository) CreateSession(ctx context.Context, s *models.Session) error {
	if err := r.db.WithContext(ctx).Omit("User").Create(s).Error; err != nil {
		return fmt.Errorf("session could not be created: %w", err)
	}
	return nil
}

// SessionByHash loads a session by its token hash, or nil when absent.
func (r *Repository) SessionByHash(ctx context.Context, tokenHash string) (*models.Session, error) {
	var s models.Session
	err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session could not be fetched: %w", err)
	}
	return &s, nil
}

// SessionByPreviousHash loads the session a token was replaced in, or nil.
func (r *Repository) SessionByPreviousHash(ctx context.Context, tokenHash string) (*models.Session, error) {
	var s models.Session
	err := r.db.WithContext(ctx).Where("previous_hash = ?", tokenHash).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session could not be fetched: %w", err)
	}
	return &s, nil
}

// RotateSession replaces a session's token, keeping the old one as the
// previous token. It reports false when the session no longer holds
// oldHash, because another request rotated or revoked it first. The expiry
// does not move: a session ends a fixed time after signing in.
func (r *Repository) RotateSession(ctx context.Context, id uint, oldHash, newHash string, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE sessions SET previous_hash = token_hash, token_hash = ?, rotated_at = ?, last_used_at = ?, updated_at = ?
		WHERE id = ? AND token_hash = ? AND revoked_at IS NULL`, newHash, at, at, at, id, oldHash)
	if res.Error != nil {
		return false, fmt.Errorf("session could not be rotated: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// RevokeSession marks one session revoked.
func (r *Repository) RevokeSession(ctx context.Context, id uint, at time.Time) error {
	if err := r.db.WithContext(ctx).
		Model(&models.Session{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", at).Error; err != nil {
		return fmt.Errorf("session could not be revoked: %w", err)
	}
	return nil
}

// RevokeUserSessions revokes all of a user's active sessions.
func (r *Repository) RevokeUserSessions(ctx context.Context, userID uint, at time.Time) error {
	if err := r.db.WithContext(ctx).
		Model(&models.Session{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", at).Error; err != nil {
		return fmt.Errorf("user sessions could not be revoked: %w", err)
	}
	return nil
}
