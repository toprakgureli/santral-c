// Package user owns user persistence and account logic.
package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
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

// EmailExists reports whether an account already uses the email.
func (r *Repository) EmailExists(ctx context.Context, email string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("email = ?", email).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("email existence could not be checked: %w", err)
	}
	return count > 0, nil
}

// EmailExistsExcept reports whether another account already uses the email.
func (r *Repository) EmailExistsExcept(ctx context.Context, email string, exceptID uint) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("email = ? AND id <> ?", email, exceptID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("email existence could not be checked: %w", err)
	}
	return count > 0, nil
}

// Avatar reads one user's stored photo (a data URI, or "").
func (r *Repository) Avatar(ctx context.Context, id uint) (string, error) {
	var rows []string
	if err := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Limit(1).Pluck("avatar", &rows).Error; err != nil {
		return "", fmt.Errorf("avatar could not be read: %w", err)
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0], nil
}

// UpdateCore updates a user's profile columns.
func (r *Repository) UpdateCore(ctx context.Context, id uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Updates(fields).Error; err != nil {
		return fmt.Errorf("user could not be updated: %w", err)
	}
	return nil
}

// RolesByIDs loads roles with their permissions for the given ids.
func (r *Repository) RolesByIDs(ctx context.Context, ids []uint) ([]models.Role, error) {
	var roles []models.Role
	if err := r.db.WithContext(ctx).
		Preload("Permissions").
		Where("id IN ?", ids).
		Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("roles could not be fetched by ids: %w", err)
	}
	return roles, nil
}

// Create inserts a user and its role assignments in one transaction.
func (r *Repository) Create(ctx context.Context, u *models.User, roles []models.Role) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Roles").Create(u).Error; err != nil {
			return fmt.Errorf("user could not be created: %w", err)
		}
		if err := tx.Model(u).Association("Roles").Replace(roles); err != nil {
			return fmt.Errorf("user roles could not be assigned: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// ReplaceRoles sets a user's role assignment.
func (r *Repository) ReplaceRoles(ctx context.Context, u *models.User, roles []models.Role) error {
	if err := r.db.WithContext(ctx).Model(u).Omit("Roles.*").Association("Roles").Replace(roles); err != nil {
		return fmt.Errorf("user roles could not be replaced: %w", err)
	}
	return nil
}

// List returns a filtered, paginated page of users and the total count.
func (r *Repository) List(ctx context.Context, f requests.UserFilter) ([]models.User, int64, error) {
	q := r.db.WithContext(ctx).Model(&models.User{})
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("name ILIKE ? OR email ILIKE ?", like, like)
	}
	if f.Active != nil {
		q = q.Where("active = ?", *f.Active)
	}
	if f.RoleID != nil {
		q = q.Where("id IN (?)", r.db.
			Table("user_roles").
			Select("user_id").
			Where("role_id = ?", *f.RoleID))
	}
	if f.ExcludeInvisibleAdmin {
		q = q.Where("id NOT IN (?)", r.db.
			Table("user_roles").
			Select("user_roles.user_id").
			Joins("JOIN roles ON roles.id = user_roles.role_id").
			Where("roles.name = ?", string(enums.RoleInvisibleAdmin)))
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("users could not be counted: %w", err)
	}

	var users []models.User
	if err := q.
		Preload("Roles.Permissions").
		Order("created_at DESC").
		Limit(f.PerPage).
		Offset((f.Page - 1) * f.PerPage).
		Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("users could not be listed: %w", err)
	}
	return users, total, nil
}

// SetActive updates a user's active flag.
func (r *Repository) SetActive(ctx context.Context, id uint, active bool) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Update("active", active).Error; err != nil {
		return fmt.Errorf("user active state could not be updated: %w", err)
	}
	return nil
}
