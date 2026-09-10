// Package escalation owns the customer escalation catalog and the per-number
// escalation records agents log during calls.
package escalation

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// IActorResolver loads the acting user for authorization and agent attribution.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}
