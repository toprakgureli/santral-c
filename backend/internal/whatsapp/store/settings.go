package store

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// GlobalSetting reads a module wide setting by key.
func (r *Repository) GlobalSetting(ctx context.Context, key string) (*models.WAGlobalSetting, error) {
	var row models.WAGlobalSetting
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// SaveGlobalSetting stores a module wide setting, replacing the old value.
func (r *Repository) SaveGlobalSetting(ctx context.Context, row *models.WAGlobalSetting) error {
	return r.db.WithContext(ctx).Save(row).Error
}
