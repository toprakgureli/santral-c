// Package user owns user persistence and account logic.
package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the user data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a user repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetByEmail loads a user with roles and permissions, or nil when absent.
func (r *Repository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		Where("email = ?", email).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user could not be fetched by email: %w", err)
	}
	return &u, nil
}

// GetByID loads a user with roles and permissions, or nil when absent.
func (r *Repository) GetByID(ctx context.Context, id uint) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).
		Preload("Roles.Permissions").
		First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user could not be fetched by id: %w", err)
	}
	return &u, nil
}

// UpdateLastLogin stamps the last login time.
func (r *Repository) UpdateLastLogin(ctx context.Context, id uint, at time.Time) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Update("last_login_at", at).Error; err != nil {
		return fmt.Errorf("last login could not be updated: %w", err)
	}
	return nil
}

// MarkOnboarded stamps the first-time onboarding completion.
func (r *Repository) MarkOnboarded(ctx context.Context, id uint, at time.Time) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ? AND onboarded_at IS NULL", id).
		Update("onboarded_at", at).Error; err != nil {
		return fmt.Errorf("onboarding could not be marked: %w", err)
	}
	return nil
}

// SetMFA updates the encrypted secret and enabled flag.
func (r *Repository) SetMFA(ctx context.Context, id uint, secret *string, enabled bool) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"mfa_secret":  secret,
			"mfa_enabled": enabled,
		}).Error; err != nil {
		return fmt.Errorf("mfa settings could not be updated: %w", err)
	}
	return nil
}

// SetPassword updates the password hash and the must-change flag.
func (r *Repository) SetPassword(ctx context.Context, id uint, hashed string, mustChange bool) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"password":             hashed,
			"must_change_password": mustChange,
		}).Error; err != nil {
		return fmt.Errorf("user password could not be updated: %w", err)
	}
	return nil
}
