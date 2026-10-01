package store

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// LoadIntegration reads an outside system a flow may ask.
func (r *Repository) LoadIntegration(ctx context.Context, id uint) (*models.WAIntegration, error) {
	var in models.WAIntegration
	if err := r.db.WithContext(ctx).First(&in, id).Error; err != nil {
		return nil, err
	}
	return &in, nil
}

// IntegrationURL reads the address an outside system is called at.
func (r *Repository) IntegrationURL(ctx context.Context, id uint) (string, error) {
	var in models.WAIntegration
	if err := r.db.WithContext(ctx).Select("url").First(&in, id).Error; err != nil {
		return "", err
	}
	return in.URL, nil
}

// Integrations lists the outside systems in the order of their names.
func (r *Repository) Integrations(ctx context.Context) ([]models.WAIntegration, error) {
	var list []models.WAIntegration
	if err := r.db.WithContext(ctx).Order("name").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CreateIntegration stores a new outside system.
func (r *Repository) CreateIntegration(ctx context.Context, in *models.WAIntegration) error {
	return r.db.WithContext(ctx).Create(in).Error
}

// UpdateIntegration changes the given columns of an outside system.
func (r *Repository) UpdateIntegration(ctx context.Context, id uint, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.WAIntegration{}).Where("id = ?", id).Updates(fields).Error
}

// DeleteIntegration removes an outside system.
func (r *Repository) DeleteIntegration(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.WAIntegration{}, id).Error
}
