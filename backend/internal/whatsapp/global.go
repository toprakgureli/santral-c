package whatsapp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Module-wide settings (the reply assistant, the survey after a call) are
// kept as one JSON value per key.

func (s *Service) loadGlobal(ctx context.Context, key string, out any) bool {
	var row models.WAGlobalSetting
	if err := s.db.WithContext(ctx).Where("key = ?", key).First(&row).Error; err != nil {
		return false
	}
	return json.Unmarshal([]byte(row.Value), out) == nil
}

func (s *Service) saveGlobal(ctx context.Context, actorID uint, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return errs.Internal(err)
	}
	row := models.WAGlobalSetting{Key: key, Value: string(raw), UpdatedBy: uintPtr(actorID), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return errs.Internal(err)
	}
	return nil
}
