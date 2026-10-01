package store

import (
	"context"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// CreateFile stores a file uploaded from the panel.
func (r *Repository) CreateFile(ctx context.Context, f *models.WAFile) error {
	return r.db.WithContext(ctx).Create(f).Error
}

// LoadFile reads a file uploaded from the panel.
func (r *Repository) LoadFile(ctx context.Context, id uint) (*models.WAFile, error) {
	var f models.WAFile
	if err := r.db.WithContext(ctx).First(&f, id).Error; err != nil {
		return nil, err
	}
	return &f, nil
}
