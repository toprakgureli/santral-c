// Package setting reads and writes runtime flags in system_settings.
package setting

import (
	"context"
	"fmt"
	"net/netip"
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

// KeyMFARequired is the old on/off flag forcing MFA enrollment at login. It
// still seeds the mode when no mode has been stored yet.
const KeyMFARequired = "mfa_required"

// KeyMFAMode is the MFA policy: on, off or trusted. KeyMFATrustedIPs holds
// the addresses (one per line) that are not asked for a code in trusted mode.
const (
	KeyMFAMode       = "mfa_mode"
	KeyMFATrustedIPs = "mfa_trusted_ips"
)

// MFA modes.
const (
	MFAOn      = "on"
	MFAOff     = "off"
	MFATrusted = "trusted"
)

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

// MFAPolicy returns the MFA mode and, for trusted mode, the addresses that
// skip the code. Without a stored mode the old on/off flag decides.
func (s *Service) MFAPolicy(ctx context.Context) (string, []string) {
	mode := strings.TrimSpace(s.value(ctx, KeyMFAMode))
	switch mode {
	case MFAOn, MFAOff, MFATrusted:
	default:
		mode = MFAOff
		if s.flag(ctx, KeyMFARequired) {
			mode = MFAOn
		}
	}
	return mode, splitIPs(s.value(ctx, KeyMFATrustedIPs))
}

// Settings returns the current flags to a user holding system.settings.
func (s *Service) Settings(ctx context.Context, actorID uint, ip string) (*responses.Settings, error) {
	if err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	mode, trusted := s.MFAPolicy(ctx)
	return &responses.Settings{MFAMode: mode, MFATrustedIPs: trusted, ClientIP: ip}, nil
}

// Update writes the flags and records the change.
func (s *Service) Update(ctx context.Context, actorID uint, req requests.SettingsUpdate, ip string) (*responses.Settings, error) {
	if err := s.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	trusted, err := normalizeIPs(req.MFATrustedIPs)
	if err != nil {
		return nil, err
	}
	if req.MFAMode == MFATrusted && len(trusted) == 0 {
		return nil, errs.Invalid("Güvenilir IP modu için en az bir adres girin.", nil)
	}
	if err := s.set(ctx, KeyMFAMode, req.MFAMode); err != nil {
		return nil, errs.Internal(err)
	}
	if err := s.set(ctx, KeyMFATrustedIPs, strings.Join(trusted, "\n")); err != nil {
		return nil, errs.Internal(err)
	}
	// Keep the old flag in step for anything still reading it.
	old := "0"
	if req.MFAMode != MFAOff {
		old = "1"
	}
	if err := s.set(ctx, KeyMFARequired, old); err != nil {
		return nil, errs.Internal(err)
	}
	if s.audit != nil {
		s.audit.Record(ctx, audit.Entry{
			ActorID:    &actorID,
			Action:     enums.AuditSettingsUpdated,
			TargetType: "settings",
			TargetID:   KeyMFAMode,
			IP:         ip,
			Detail:     map[string]any{"mfaMode": req.MFAMode, "mfaTrustedIps": trusted},
		})
	}
	return &responses.Settings{MFAMode: req.MFAMode, MFATrustedIPs: trusted, ClientIP: ip}, nil
}

// splitIPs turns the stored newline list into entries.
func splitIPs(raw string) []string {
	out := []string{}
	for _, line := range strings.Split(raw, "\n") {
		if v := strings.TrimSpace(line); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// normalizeIPs validates addresses and CIDR blocks, drops duplicates and
// keeps them in canonical form.
func normalizeIPs(entries []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range entries {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		var canon string
		if strings.Contains(v, "/") {
			p, err := netip.ParsePrefix(v)
			if err != nil {
				return nil, errs.Invalid(fmt.Sprintf("Geçersiz IP bloğu: %s", v), nil)
			}
			canon = p.Masked().String()
		} else {
			a, err := netip.ParseAddr(v)
			if err != nil {
				return nil, errs.Invalid(fmt.Sprintf("Geçersiz IP adresi: %s", v), nil)
			}
			canon = a.Unmap().String()
		}
		if !seen[canon] {
			seen[canon] = true
			out = append(out, canon)
		}
	}
	return out, nil
}

// IPTrusted reports whether ip is one of the entries or inside one of the
// blocks.
func IPTrusted(ip string, entries []string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, e := range entries {
		if strings.Contains(e, "/") {
			if p, err := netip.ParsePrefix(e); err == nil && p.Contains(addr) {
				return true
			}
			continue
		}
		if a, err := netip.ParseAddr(e); err == nil && a.Unmap() == addr {
			return true
		}
	}
	return false
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

// value reads one setting, "" when missing.
func (s *Service) value(ctx context.Context, key string) string {
	var values []string
	err := s.db.WithContext(ctx).
		Model(&models.SystemSetting{}).
		Where("key = ?", key).
		Limit(1).
		Pluck("value", &values).Error
	if err != nil || len(values) == 0 {
		return ""
	}
	return values[0]
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
