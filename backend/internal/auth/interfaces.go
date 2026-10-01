// Package auth handles login, MFA, sessions and the first-login flow.
package auth

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/security"
)

// IUserService resolves and mutates users for auth.
type IUserService interface {
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByID(ctx context.Context, id uint) (*models.User, error)
	MarkLogin(ctx context.Context, id uint) error
	SetMFA(ctx context.Context, id uint, secret *string, enabled bool) error
	ChangePassword(ctx context.Context, id uint, password string) error
}

// IRepository stores refresh sessions.
type IRepository interface {
	CreateSession(ctx context.Context, s *models.Session) error
	SessionByHash(ctx context.Context, tokenHash string) (*models.Session, error)
	SessionBySpentHash(ctx context.Context, tokenHash string) (*models.Session, time.Time, SpentBy, error)
	RotateSession(ctx context.Context, id uint, oldHash, newHash string, by SpentBy, at time.Time) (bool, error)
	RevokeSession(ctx context.Context, id uint, at time.Time) error
	RevokeUserSessions(ctx context.Context, userID uint, at time.Time) error
}

// ISecurityService gates and records authentication attempts.
type ISecurityService interface {
	Guard(ctx context.Context, a security.Attempt) error
	Success(ctx context.Context, a security.Attempt)
	Failure(ctx context.Context, a security.Attempt)
}

// IAttempts counts tries and remembers one-time uses.
type IAttempts interface {
	Hit(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Once(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

// IAudit records security events.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// IDenylist revokes one-time token ids.
type IDenylist interface {
	Add(ctx context.Context, id string, ttl time.Duration) error
	Has(ctx context.Context, id string) (bool, error)
}

// ISettingService exposes runtime flags. MFAPolicy is the mode (on, off,
// trusted) and the trusted addresses.
type ISettingService interface {
	MFAPolicy(ctx context.Context) (string, []string)
}

// IService is the auth application service.
type IService interface {
	Login(ctx context.Context, req requests.Login, meta RequestMeta) (*LoginResult, error)
	Refresh(ctx context.Context, refreshToken string, meta RequestMeta) (*LoginResult, error)
	Logout(ctx context.Context, accessToken, refreshToken string) error
	Me(ctx context.Context, userID uint) (*responses.User, error)
	MFASetup(ctx context.Context, userID uint) (*responses.MFASetup, error)
	MFAEnable(ctx context.Context, userID uint, code string) error
	MFAVerify(ctx context.Context, token, code string, meta RequestMeta) (*LoginResult, error)
	MFAEnroll(ctx context.Context, token string) (*responses.MFASetup, error)
	MFAEnrollVerify(ctx context.Context, token, code string, meta RequestMeta) (*LoginResult, error)
	PasswordChange(ctx context.Context, req requests.PasswordChange, meta RequestMeta) (*LoginResult, error)
	ChangeOwnPassword(ctx context.Context, userID uint, current, next string, meta RequestMeta) (*LoginResult, error)
	LogoutEverywhere(ctx context.Context, userID uint) error
}

var _ IService = (*Service)(nil)
