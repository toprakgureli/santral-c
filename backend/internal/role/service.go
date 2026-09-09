package role

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// Service is the role application service.
type Service struct {
	repo  *Repository
	users IActorResolver
}

// NewService builds a role service.
func NewService(repo *Repository, users IActorResolver) *Service {
	return &Service{repo: repo, users: users}
}

// List returns the assignable roles. The invisible-admin role is hidden from
// actors who are not themselves invisible admins.
func (s *Service) List(ctx context.Context, actorID uint) ([]responses.Role, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.RoleView) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}

	roles, err := s.repo.List(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}

	if !actor.IsInvisibleAdmin() {
		visible := make([]models.Role, 0, len(roles))
		for i := range roles {
			if enums.Role(roles[i].Name) == enums.RoleInvisibleAdmin {
				continue
			}
			visible = append(visible, roles[i])
		}
		roles = visible
	}
	return responses.NewRoles(roles), nil
}
