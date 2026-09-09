package user

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
)

// Service is the user application service.
type Service struct {
	repo *Repository
}

// NewService builds a user service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// GetByEmail loads a user by email, or nil when absent.
func (s *Service) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return u, nil
}

// GetByID loads a user by id and fails when absent.
func (s *Service) GetByID(ctx context.Context, id uint) (*models.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if u == nil {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	return u, nil
}

// MarkLogin stamps the last login time.
func (s *Service) MarkLogin(ctx context.Context, id uint) error {
	if err := s.repo.UpdateLastLogin(ctx, id, time.Now()); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// MarkOnboarded records that the user finished onboarding.
func (s *Service) MarkOnboarded(ctx context.Context, id uint) error {
	if err := s.repo.MarkOnboarded(ctx, id, time.Now()); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// SetMFA updates the encrypted secret and enabled flag.
func (s *Service) SetMFA(ctx context.Context, id uint, secret *string, enabled bool) error {
	if err := s.repo.SetMFA(ctx, id, secret, enabled); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// ChangePassword hashes and stores a new password and clears the must-change flag.
func (s *Service) ChangePassword(ctx context.Context, id uint, password string) error {
	hashed, err := hash.Password(password)
	if err != nil {
		return errs.Internal(err)
	}
	if err := s.repo.SetPassword(ctx, id, hashed, false); err != nil {
		return errs.Internal(err)
	}
	return nil
}
