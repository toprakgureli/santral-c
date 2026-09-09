// Package sip provisions Asterisk PJSIP endpoints for agents and serves
// softphone credentials.
package sip

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository reads and writes the SIP fields on users.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a SIP repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetUser loads a user, or nil when absent.
func (r *Repository) GetUser(ctx context.Context, id uint) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user could not be fetched: %w", err)
	}
	return &u, nil
}

// SetProvisioning stores the extension and encrypted secret and marks the user
// provisioned.
func (r *Repository) SetProvisioning(ctx context.Context, id uint, ext, encSecret string) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"sip_extension":   ext,
			"sip_secret":      encSecret,
			"sip_provisioned": true,
		}).Error; err != nil {
		return fmt.Errorf("provisioning could not be stored: %w", err)
	}
	return nil
}

// Provisioned returns all users with a provisioned extension.
func (r *Repository) Provisioned(ctx context.Context) ([]models.User, error) {
	var users []models.User
	if err := r.db.WithContext(ctx).
		Where("sip_provisioned = true AND sip_extension IS NOT NULL AND sip_secret IS NOT NULL").
		Order("sip_extension ASC").
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("provisioned users could not be listed: %w", err)
	}
	return users, nil
}
