package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the security data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a security repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Record persists one login attempt.
func (r *Repository) Record(ctx context.Context, a *models.LoginAttempt) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return fmt.Errorf("login attempt could not be recorded: %w", err)
	}
	return nil
}

// FailuresByIP counts recent failures from an IP.
func (r *Repository) FailuresByIP(ctx context.Context, ip string, since time.Time) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.LoginAttempt{}).
		Where("ip = ? AND success = false AND created_at >= ?", ip, since).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("ip failures could not be counted: %w", err)
	}
	return count, nil
}

// FailingIPs lists the distinct IPs recently failing against an email.
func (r *Repository) FailingIPs(ctx context.Context, email string, since time.Time) ([]string, error) {
	var ips []string
	if err := r.db.WithContext(ctx).
		Model(&models.LoginAttempt{}).
		Distinct().
		Where("email = ? AND success = false AND created_at >= ?", email, since).
		Pluck("ip", &ips).Error; err != nil {
		return nil, fmt.Errorf("failing ips could not be listed: %w", err)
	}
	return ips, nil
}

// ActiveBan returns an unexpired ban for an IP, or nil.
func (r *Repository) ActiveBan(ctx context.Context, ip string, at time.Time) (*models.IPBan, error) {
	var ban models.IPBan
	err := r.db.WithContext(ctx).Where("ip = ?", ip).First(&ban).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ip ban could not be fetched: %w", err)
	}
	if ban.Until.Before(at) {
		return nil, nil
	}
	return &ban, nil
}

// Ban bans an IP with exponential backoff on repeats.
func (r *Repository) Ban(ctx context.Context, ip, reason string, base time.Duration) error {
	minutes := int(base.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	now := time.Now()
	ban := models.IPBan{IP: ip, Reason: reason, Attempts: 1, Until: now.Add(base)}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "ip"}},
		DoUpdates: clause.Assignments(map[string]any{
			"reason":     reason,
			"attempts":   gorm.Expr("ip_bans.attempts + 1"),
			"until":      gorm.Expr("now() + make_interval(mins => (? * LEAST(ip_bans.attempts + 1, 48))::int)", minutes),
			"updated_at": now,
		}),
	}).Create(&ban).Error; err != nil {
		return fmt.Errorf("ip could not be banned: %w", err)
	}
	return nil
}

// Attempts returns a filtered page of login attempts, newest first.
func (r *Repository) Attempts(ctx context.Context, f requests.SecurityFilter) ([]models.LoginAttempt, int64, error) {
	build := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&models.LoginAttempt{})
		if s := strings.TrimSpace(f.Email); s != "" {
			q = q.Where("email ILIKE ?", "%"+s+"%")
		}
		if s := strings.TrimSpace(f.IP); s != "" {
			q = q.Where("ip = ?", s)
		}
		if f.Success != nil {
			q = q.Where("success = ?", *f.Success)
		}
		return q
	}
	var total int64
	if err := build().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("login attempts could not be counted: %w", err)
	}
	var list []models.LoginAttempt
	if err := build().
		Order("created_at DESC").
		Limit(f.PerPage).
		Offset((f.Page - 1) * f.PerPage).
		Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("login attempts could not be listed: %w", err)
	}
	return list, total, nil
}

// Bans lists the bans still in force at the given time.
func (r *Repository) Bans(ctx context.Context, at time.Time) ([]models.IPBan, error) {
	var list []models.IPBan
	if err := r.db.WithContext(ctx).
		Where("until > ?", at).
		Order("until DESC").
		Find(&list).Error; err != nil {
		return nil, fmt.Errorf("ip bans could not be listed: %w", err)
	}
	return list, nil
}

// BanByID loads one ban, or nil when absent.
func (r *Repository) BanByID(ctx context.Context, id uint) (*models.IPBan, error) {
	var ban models.IPBan
	err := r.db.WithContext(ctx).First(&ban, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ip ban could not be fetched: %w", err)
	}
	return &ban, nil
}

// Unban removes a ban row.
func (r *Repository) Unban(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&models.IPBan{}, id).Error; err != nil {
		return fmt.Errorf("ip ban could not be removed: %w", err)
	}
	return nil
}

// MarkUserLock sets or clears a user's locked_until.
func (r *Repository) MarkUserLock(ctx context.Context, email string, until *time.Time) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("email = ?", email).
		Update("locked_until", until).Error; err != nil {
		return fmt.Errorf("user lock could not be marked: %w", err)
	}
	return nil
}

// ResetFailures clears a user's failure counters and lock.
func (r *Repository) ResetFailures(ctx context.Context, email string) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("email = ?", email).
		Updates(map[string]any{"failed_count": 0, "locked_until": nil}).Error; err != nil {
		return fmt.Errorf("user failures could not be reset: %w", err)
	}
	return nil
}
