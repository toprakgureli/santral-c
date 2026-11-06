package custommenu

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// Repository persists custom menu definitions and their role assignments.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a custommenu repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// List returns all custom menus ordered by sort_order and id, with roles preloaded.
func (r *Repository) List(ctx context.Context, onlyActive bool) ([]models.CustomMenu, error) {
	var out []models.CustomMenu
	q := r.db.WithContext(ctx).Preload("Roles").Order("sort_order ASC, id ASC")
	if onlyActive {
		q = q.Where("active = ?", true)
	}
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// GetByID loads one custom menu by ID with its roles.
func (r *Repository) GetByID(ctx context.Context, id uint) (*models.CustomMenu, error) {
	var m models.CustomMenu
	err := r.db.WithContext(ctx).Preload("Roles").First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// RolesByIDs loads roles matching ids, excluding the invisible admin role.
func (r *Repository) RolesByIDs(ctx context.Context, ids []uint) ([]models.Role, error) {
	if len(ids) == 0 {
		return []models.Role{}, nil
	}
	var out []models.Role
	if err := r.db.WithContext(ctx).
		Where("id IN ? AND name <> ?", ids, string(enums.RoleInvisibleAdmin)).
		Order("id ASC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// AllRoles returns all selectable roles (excluding invisible_admin).
func (r *Repository) AllRoles(ctx context.Context) ([]models.Role, error) {
	var out []models.Role
	if err := r.db.WithContext(ctx).
		Where("name <> ?", string(enums.RoleInvisibleAdmin)).
		Order("system DESC, display_name ASC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// NextSortOrder returns max(sort_order)+1 for a newly created menu item.
func (r *Repository) NextSortOrder(ctx context.Context) (int, error) {
	var maxOrder int
	if err := r.db.WithContext(ctx).Model(&models.CustomMenu{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error; err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// Create inserts a custom menu and its role associations.
func (r *Repository) Create(ctx context.Context, m *models.CustomMenu, roles []models.Role) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Roles").Create(m).Error; err != nil {
			return err
		}
		if len(roles) > 0 {
			if err := tx.Model(m).Omit("Roles.*").Association("Roles").Replace(roles); err != nil {
				return err
			}
		}
		m.Roles = roles
		return nil
	})
}

// Update saves a custom menu and replaces its role associations.
func (r *Repository) Update(ctx context.Context, m *models.CustomMenu, roles []models.Role) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(m).Select("label", "icon", "target_url", "open_mode", "active", "updated_at").Updates(m).Error; err != nil {
			return err
		}
		if err := tx.Model(m).Omit("Roles.*").Association("Roles").Replace(roles); err != nil {
			return err
		}
		m.Roles = roles
		return nil
	})
}

// Delete removes a custom menu by ID.
func (r *Repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.CustomMenu{}, id).Error
}

// Reorder updates sort_order for the given menu IDs in order.
func (r *Repository) Reorder(ctx context.Context, ids []uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&models.CustomMenu{}).Where("id = ?", id).Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
