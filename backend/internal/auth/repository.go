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

// spentKept is how long a replaced refresh token is remembered: longer
// than any session lasts, after which the token is worthless anyway.
const spentKept = 8 * 24 * time.Hour

// CreateSession inserts a new session. It also forgets replaced tokens of
// sessions long over, so their list stays small.
func (r *Repository) CreateSession(ctx context.Context, s *models.Session) error {
	if err := r.db.WithContext(ctx).Omit("User").Create(s).Error; err != nil {
		return fmt.Errorf("session could not be created: %w", err)
	}
	if err := r.db.WithContext(ctx).Exec("DELETE FROM session_spent_tokens WHERE spent_at < ?", time.Now().Add(-spentKept)).Error; err != nil {
		return fmt.Errorf("old replaced tokens could not be removed: %w", err)
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

// SpentBy is the browser and address that handed in a replaced token.
type SpentBy struct {
	Device string
	IP     string
}

// SessionBySpentHash loads the session a token was replaced in, when it
// was replaced and by whom, or nil when the token was never replaced.
// Tokens replaced before every replacement was remembered are found as the
// previous token.
func (r *Repository) SessionBySpentHash(ctx context.Context, tokenHash string) (*models.Session, time.Time, SpentBy, error) {
	var spent struct {
		SessionID uint
		SpentAt   time.Time
		Device    string
		IP        string
	}
	if err := r.db.WithContext(ctx).Raw("SELECT session_id, spent_at, device, ip FROM session_spent_tokens WHERE token_hash = ?", tokenHash).
		Scan(&spent).Error; err != nil {
		return nil, time.Time{}, SpentBy{}, fmt.Errorf("replaced token could not be looked up: %w", err)
	}
	var s models.Session
	q := r.db.WithContext(ctx)
	if spent.SessionID != 0 {
		q = q.Where("id = ?", spent.SessionID)
	} else {
		q = q.Where("previous_hash = ?", tokenHash)
	}
	err := q.First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, time.Time{}, SpentBy{}, nil
	}
	if err != nil {
		return nil, time.Time{}, SpentBy{}, fmt.Errorf("session could not be fetched: %w", err)
	}
	at := spent.SpentAt
	if spent.SessionID == 0 {
		if s.RotatedAt == nil {
			return nil, time.Time{}, SpentBy{}, nil
		}
		at = *s.RotatedAt
	}
	return &s, at, SpentBy{Device: spent.Device, IP: spent.IP}, nil
}

// RotateSession replaces a session's token, keeping the old one as the
// previous token. It reports false when the session no longer holds
// oldHash, because another request rotated or revoked it first. The expiry
// does not move: a session ends a fixed time after signing in.
//
// The replaced token is remembered, so it is recognised if it comes back.
func (r *Repository) RotateSession(ctx context.Context, id uint, oldHash, newHash string, by SpentBy, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`WITH rotated AS (
			UPDATE sessions SET previous_hash = token_hash, token_hash = ?, rotated_at = ?, last_used_at = ?, updated_at = ?
			WHERE id = ? AND token_hash = ? AND revoked_at IS NULL
			RETURNING id
		)
		INSERT INTO session_spent_tokens (token_hash, session_id, spent_at, device, ip) SELECT ?, id, ?, ?, ? FROM rotated`,
		newHash, at, at, at, id, oldHash, oldHash, at, by.Device, by.IP)
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
