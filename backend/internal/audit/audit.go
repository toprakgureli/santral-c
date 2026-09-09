// Package audit records privileged actions, including invisible-admin actions.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Entry is one action to record.
type Entry struct {
	ActorID    *uint
	Action     string
	TargetType string
	TargetID   string
	IP         string
	Detail     map[string]any
}

// Service writes audit rows.
type Service struct {
	db *gorm.DB
}

// NewService builds an audit service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Record persists an audit entry. It is best-effort and never blocks the
// caller's operation; failures are logged.
func (s *Service) Record(ctx context.Context, e Entry) {
	detail := "{}"
	if e.Detail != nil {
		if raw, err := json.Marshal(e.Detail); err == nil {
			detail = string(raw)
		}
	}
	row := &models.AuditLog{
		ActorID:    e.ActorID,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		IP:         e.IP,
		Detail:     detail,
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		slog.Warn("audit entry could not be recorded", "action", e.Action, "error", err)
	}
}
