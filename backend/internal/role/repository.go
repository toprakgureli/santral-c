// Package role exposes role listing and permission administration.
package role

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository is the role data store.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a role repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// List returns all roles with their permissions, ordered by id.
func (r *Repository) List(ctx context.Context) ([]models.Role, error) {
	var roles []models.Role
	if err := r.db.WithContext(ctx).Preload("Permissions").Order("id ASC").Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("roles could not be listed: %w", err)
	}
	return roles, nil
}

// GetByID loads a role with its permissions, or nil when absent.
func (r *Repository) GetByID(ctx context.Context, id uint) (*models.Role, error) {
	var role models.Role
	err := r.db.WithContext(ctx).Preload("Permissions").First(&role, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("role could not be fetched: %w", err)
	}
	return &role, nil
}

// UserCounts returns how many users hold each role.
func (r *Repository) UserCounts(ctx context.Context) (map[uint]int64, error) {
	type row struct {
		RoleID uint
		Count  int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("user_roles").
		Select("role_id, count(*) AS count").
		Group("role_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("role user counts could not be computed: %w", err)
	}
	out := make(map[uint]int64, len(rows))
	for _, row := range rows {
		out[row.RoleID] = row.Count
	}
	return out, nil
}

// Permissions returns the full permission catalog ordered by module then key.
func (r *Repository) Permissions(ctx context.Context) ([]models.Permission, error) {
	var perms []models.Permission
	if err := r.db.WithContext(ctx).Order("module ASC, key ASC").Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("permissions could not be listed: %w", err)
	}
	return perms, nil
}

// PermissionsByIDs loads the permissions for the given ids.
func (r *Repository) PermissionsByIDs(ctx context.Context, ids []uint) ([]models.Permission, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var perms []models.Permission
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("permissions could not be loaded: %w", err)
	}
	return perms, nil
}

// Create inserts a role with its permissions.
func (r *Repository) Create(ctx context.Context, role *models.Role) error {
	if err := r.db.WithContext(ctx).Omit("Permissions.*").Create(role).Error; err != nil {
		return fmt.Errorf("role could not be created: %w", err)
	}
	return nil
}

// UpdateCore updates a role's display fields.
func (r *Repository) UpdateCore(ctx context.Context, id uint, fields map[string]any) error {
	if err := r.db.WithContext(ctx).Model(&models.Role{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return fmt.Errorf("role could not be updated: %w", err)
	}
	return nil
}

// ReplacePermissions sets a role's permission set.
func (r *Repository) ReplacePermissions(ctx context.Context, role *models.Role, perms []models.Permission) error {
	if err := r.db.WithContext(ctx).Model(role).Omit("Permissions.*").Association("Permissions").Replace(perms); err != nil {
		return fmt.Errorf("role permissions could not be replaced: %w", err)
	}
	return nil
}

// Delete removes a role.
func (r *Repository) Delete(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&models.Role{}, id).Error; err != nil {
		return fmt.Errorf("role could not be deleted: %w", err)
	}
	return nil
}
