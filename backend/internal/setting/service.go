// Package setting reads runtime flags from system_settings.
package setting

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Service reads system settings.
type Service struct {
	db *gorm.DB
}

// NewService builds a setting service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// MFARequired reports whether MFA enrollment is forced at login.
func (s *Service) MFARequired(ctx context.Context) bool {
	return s.flag(ctx, "mfa_required")
}

func (s *Service) flag(ctx context.Context, key string) bool {
	var values []string
	err := s.db.WithContext(ctx).
		Model(&models.SystemSetting{}).
		Where("key = ?", key).
		Limit(1).
		Pluck("value", &values).Error
	if err != nil || len(values) == 0 {
		return false
	}
	v := values[0]
	return v == "1" || strings.EqualFold(v, "true")
}
