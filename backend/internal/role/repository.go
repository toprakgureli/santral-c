// Package role exposes role listing for administration screens.
package role

import (
	"context"
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

// List returns all roles ordered by id.
func (r *Repository) List(ctx context.Context) ([]models.Role, error) {
	var roles []models.Role
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("roles could not be listed: %w", err)
	}
	return roles, nil
}
