// Package security records login attempts and enforces lockout.
package security

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Attempt is one authentication attempt to record.
type Attempt struct {
	Email     string
	UserID    *uint
	IP        string
	UserAgent string
	Reason    string
}

// IRepository is the security data store.
type IRepository interface {
	Record(ctx context.Context, a *models.LoginAttempt) error
	FailuresByIP(ctx context.Context, ip string, since time.Time) (int64, error)
	FailingIPs(ctx context.Context, email string, since time.Time) ([]string, error)
	ActiveBan(ctx context.Context, ip string, at time.Time) (*models.IPBan, error)
	Ban(ctx context.Context, ip, reason string, base time.Duration) error
	MarkUserLock(ctx context.Context, email string, until *time.Time) error
	ResetFailures(ctx context.Context, email string) error
}

// IUserService resolves users for authorization checks.
type IUserService interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// ILockout tracks account lock windows.
type ILockout interface {
	Remaining(ctx context.Context, key string) (time.Duration, error)
	Set(ctx context.Context, key string, ttl time.Duration) error
	Clear(ctx context.Context, key string) error
}
