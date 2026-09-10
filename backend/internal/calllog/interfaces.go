// Package calllog owns the panel's own record of softphone calls, so call
// history no longer depends on the rate-limited hosted CDR API.
package calllog

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}
