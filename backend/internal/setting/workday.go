package setting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// The working day: when it ends, which drives the shift reminder, the day's
// summary five minutes before and the automatic close fifty minutes after,
// and each role's daily target of real calls.

const (
	// KeyShiftEnd holds the end of the working day as "HH:MM".
	KeyShiftEnd = "shift_end"
	// keyTargetPrefix starts the key of a role's daily target.
	keyTargetPrefix = "daily_target_"
	defaultShiftEnd = "18:30"
	maxTarget       = 500
)

// The end of the day stays between 12:00 and 23:00, so the automatic close
// fifty minutes later falls on the same day.
var (
	earliestEnd = 12 * 60
	latestEnd   = 23 * 60
)

// RoleTarget is one role and its daily target (0 for none).
type RoleTarget struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Target int    `json:"target"`
}

// Workday is the working-day settings.
type Workday struct {
	ShiftEnd string       `json:"shiftEnd"`
	Roles    []RoleTarget `json:"roles"`
}

// WorkdayInput changes them.
type WorkdayInput struct {
	ShiftEnd string       `json:"shiftEnd"`
	Targets  map[uint]int `json:"targets"`
}

// parseClock reads "HH:MM" as minutes after midnight.
func parseClock(v string) (int, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// ShiftEnd returns the end of the working day, 18:30 unless set.
func (s *Service) ShiftEnd(ctx context.Context) (hour, minute int) {
	m, ok := parseClock(s.value(ctx, KeyShiftEnd))
	if !ok || m < earliestEnd || m > latestEnd {
		m, _ = parseClock(defaultShiftEnd)
	}
	return m / 60, m % 60
}

// targets reads every role's daily target.
func (s *Service) targets(ctx context.Context) map[uint]int {
	var rows []models.SystemSetting
	out := map[uint]int{}
	if err := s.db.WithContext(ctx).Where("key LIKE ?", keyTargetPrefix+"%").Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		id, err := strconv.ParseUint(strings.TrimPrefix(r.Key, keyTargetPrefix), 10, 64)
		n, err2 := strconv.Atoi(r.Value)
		if err == nil && err2 == nil && n > 0 {
			out[uint(id)] = n
		}
	}
	return out
}

// TargetFor returns the daily target of someone holding the given roles:
// the highest of their roles' targets, 0 when none has one.
func (s *Service) TargetFor(ctx context.Context, roleIDs []uint) int {
	all := s.targets(ctx)
	best := 0
	for _, id := range roleIDs {
		best = max(best, all[id])
	}
	return best
}

// Workday returns the working-day settings with every role.
func (s *Service) Workday(ctx context.Context) (*Workday, error) {
	h, m := s.ShiftEnd(ctx)
	out := &Workday{ShiftEnd: fmt.Sprintf("%02d:%02d", h, m), Roles: []RoleTarget{}}
	var roles []models.Role
	if err := s.db.WithContext(ctx).Where("name <> ?", string(enums.RoleInvisibleAdmin)).Order("display_name").Find(&roles).Error; err != nil {
		return nil, errs.Internal(err)
	}
	all := s.targets(ctx)
	for _, r := range roles {
		out.Roles = append(out.Roles, RoleTarget{ID: r.ID, Name: r.DisplayName, Target: all[r.ID]})
	}
	return out, nil
}

// SetWorkday stores the end of the working day and the roles' targets;
// needs agent.workday.
func (s *Service) SetWorkday(ctx context.Context, actorID uint, in WorkdayInput, ip string) (*Workday, error) {
	if s.users == nil {
		return nil, errs.Forbidden("Bu işlem için yetkin yok.")
	}
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.AgentWorkday) {
		return nil, errs.Forbidden("Mesai ayarlarını değiştirme yetkin yok.")
	}
	end, ok := parseClock(in.ShiftEnd)
	if !ok || end < earliestEnd || end > latestEnd {
		return nil, errs.Invalid("Mesai bitişi 12:00 ile 23:00 arasında bir saat olmalı.", nil)
	}
	for id, n := range in.Targets {
		if n < 0 || n > maxTarget {
			return nil, errs.Invalid(fmt.Sprintf("Günlük hedef 0 ile %d arasında olmalı.", maxTarget), nil)
		}
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.Role{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return nil, errs.Internal(err)
		}
		if count == 0 {
			return nil, errs.Invalid("Bilinmeyen bir rol için hedef girildi.", nil)
		}
	}
	before := s.value(ctx, KeyShiftEnd)
	if err := s.set(ctx, KeyShiftEnd, fmt.Sprintf("%02d:%02d", end/60, end%60)); err != nil {
		return nil, errs.Internal(err)
	}
	for id, n := range in.Targets {
		if err := s.set(ctx, fmt.Sprintf("%s%d", keyTargetPrefix, id), strconv.Itoa(n)); err != nil {
			return nil, errs.Internal(err)
		}
	}
	if s.audit != nil {
		s.audit.Record(ctx, audit.Entry{
			ActorID:    &actorID,
			Action:     enums.AuditSettingsUpdated,
			TargetType: "settings",
			TargetID:   KeyShiftEnd,
			IP:         ip,
			Detail:     map[string]any{"shiftEnd": in.ShiftEnd, "before": before, "targets": in.Targets},
		})
	}
	return s.Workday(ctx)
}
