package store

import (
	"context"
	"fmt"

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

// FileInBot reports whether a chatbot uses an uploaded file, in its draft
// or in any version it was published with.
func (r *Repository) FileInBot(ctx context.Context, id uint) (bool, error) {
	// A stored graph writes a file as "fileId": <id> followed by a comma
	// or the end of its box.
	pattern := fmt.Sprintf(`"fileId": %d[,}]`, id)
	var used bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM wa_bots WHERE draft::text ~ ?)
		OR EXISTS (SELECT 1 FROM wa_bot_versions WHERE graph::text ~ ?)`, pattern, pattern).Scan(&used).Error
	return used, err
}
