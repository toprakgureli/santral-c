package user

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
)

// IAudit records privileged mutations.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// ISessionRevoker revokes a user's active sessions.
type ISessionRevoker interface {
	RevokeUserSessions(ctx context.Context, userID uint, at time.Time) error
}

// IManagement is the user administration service consumed by the handler.
type IManagement interface {
	CreateUser(ctx context.Context, actorID uint, req requests.UserCreate, meta Meta) (*responses.User, error)
	List(ctx context.Context, actorID uint, filter requests.UserFilter) (*responses.UserList, error)
	SetActive(ctx context.Context, actorID, targetID uint, active bool, meta Meta) error
	ResetPassword(ctx context.Context, actorID, targetID uint, password string, meta Meta) error
}
