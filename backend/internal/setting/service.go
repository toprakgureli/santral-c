// Package setting reads and writes runtime flags in system_settings.
package setting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// KeyMFARequired is the flag forcing MFA enrollment at login.
const KeyMFARequired = "mfa_required"

// KeyBreakLimit is the daily break allowance in minutes; past it the break
// card turns red and counts the excess.
const KeyBreakLimit = "break_limit_minutes"

// Break limit bounds (minutes). The default applies when nothing is stored.
const (
	DefaultBreakLimitMinutes = 60
	MinBreakLimitMinutes     = 5
	MaxBreakLimitMinutes     = 720
)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IAudit records privileged mutations.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// Service reads and writes system settings.
type Service struct {
	db    *gorm.DB
	users IActorResolver
	audit IAudit
}

// NewService builds a setting service.
func NewService(db *gorm.DB, users IActorResolver, auditor IAudit) *Service {
	return &Service{db: db, users: users, audit: auditor}
}

// MFARequired reports whether MFA enrollment is forced at login.
func (s *Service) MFARequired(ctx context.Context) bool {
	return s.flag(ctx, KeyMFARequired)
}

// Settings returns the current flags to a user holding system.settings.
func (s *Service) Settings(ctx context.Context, actorID uint) (*responses.Settings, error) {
	if err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	return &responses.Settings{MFARequired: s.MFARequired(ctx)}, nil
}

// Update writes the flags and records the change.
func (s *Service) Update(ctx context.Context, actorID uint, req requests.SettingsUpdate, ip string) (*responses.Settings, error) {
	if err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	value := "0"
	if req.MFARequired {
		value = "1"
	}
	if err := s.set(ctx, KeyMFARequired, value); err != nil {
		return nil, errs.Internal(err)
	}
	if s.audit != nil {
		s.audit.Record(ctx, audit.Entry{
			ActorID:    &actorID,
			Action:     enums.AuditSettingsUpdated,
			TargetType: "settings",
			TargetID:   KeyMFARequired,
			IP:         ip,
			Detail:     map[string]any{"mfaRequired": req.MFARequired},
		})
	}
	return &responses.Settings{MFARequired: req.MFARequired}, nil
}

// BreakLimitMinutes returns the daily break allowance, falling back to the
// default when the setting is missing or unreadable.
func (s *Service) BreakLimitMinutes(ctx context.Context) int {
	var values []string
	err := s.db.WithContext(ctx).
		Model(&models.SystemSetting{}).
		Where("key = ?", KeyBreakLimit).
		Limit(1).
		Pluck("value", &values).Error
	if err != nil || len(values) == 0 {
		return DefaultBreakLimitMinutes
	}
	n, err := strconv.Atoi(strings.TrimSpace(values[0]))
	if err != nil || n < MinBreakLimitMinutes || n > MaxBreakLimitMinutes {
		return DefaultBreakLimitMinutes
	}
	return n
}

// SetBreakLimit stores the daily break allowance; needs agent.break_limit.
func (s *Service) SetBreakLimit(ctx context.Context, actorID uint, minutes int, ip string) error {
	if s.users == nil {
		return errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.AgentBreakLimit) {
		return errs.Forbidden("Mola sınırını değiştirme yetkiniz yok.")
	}
	if minutes < MinBreakLimitMinutes || minutes > MaxBreakLimitMinutes {
		return errs.Invalid(fmt.Sprintf("Mola sınırı %d ile %d dakika arasında olmalı.", MinBreakLimitMinutes, MaxBreakLimitMinutes), nil)
	}
	if err := s.set(ctx, KeyBreakLimit, strconv.Itoa(minutes)); err != nil {
		return errs.Internal(err)
	}
	if s.audit != nil {
		s.audit.Record(ctx, audit.Entry{
			ActorID:    &actorID,
			Action:     enums.AuditSettingsUpdated,
			TargetType: "settings",
			TargetID:   KeyBreakLimit,
			IP:         ip,
			Detail:     map[string]any{"breakLimitMinutes": minutes},
		})
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, actorID uint) error {
	if s.users == nil {
		return errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.SystemSettings) {
		return errs.Forbidden("Sistem ayarlarını değiştirme yetkiniz yok.")
	}
	return nil
}

func (s *Service) set(ctx context.Context, key, value string) error {
	row := models.SystemSetting{Key: key, Value: value, UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("setting %s could not be saved: %w", key, err)
	}
	return nil
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
