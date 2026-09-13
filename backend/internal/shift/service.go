// Package shift tracks agents' working hours: a shift is started before the
// dialer opens and closed at the end of the day, by the agent or by the server.
package shift

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Working-day cutoffs in Istanbul time. The day nominally ends at 18:30; a
// shift still open at 19:20 is closed by the server.
const (
	reminderHour, reminderMinute = 18, 30
	autoEndHour, autoEndMinute   = 19, 20
	sweepEvery                   = time.Minute
)

// istanbul is the tenant's timezone (UTC+3, no DST), fixed so the cutoff does
// not depend on the host's tzdata.
var istanbul = time.FixedZone("+03", 3*3600)

// Who closed a shift.
const (
	EndedByUser = "user"
	EndedByAuto = "auto"
)

// IPresence lets the shift lifecycle drive the agent's telephony presence.
// Implementations must tolerate users without an extension.
type IPresence interface {
	ShiftStarted(ctx context.Context, userID uint)
	ShiftEnded(ctx context.Context, userID uint)
}

// IAudit records shift transitions.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// Service is the shift application service.
type Service struct {
	repo     *Repository
	audit    IAudit
	presence IPresence
}

// NewService builds a shift service.
func NewService(repo *Repository, auditor IAudit) *Service {
	return &Service{repo: repo, audit: auditor}
}

// SetPresence wires the telephony hook. It is set after construction because
// the telephony service in turn reads shifts.
func (s *Service) SetPresence(p IPresence) { s.presence = p }

// Active reports whether the user is on shift.
func (s *Service) Active(ctx context.Context, userID uint) (bool, error) {
	return s.repo.Active(ctx, userID)
}

// Current returns the user's open shift with the day's cutoffs.
func (s *Service) Current(ctx context.Context, userID uint) (*responses.ShiftStatus, error) {
	open, err := s.repo.Open(ctx, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := &responses.ShiftStatus{Shift: responses.NewShift(open)}
	if open != nil {
		auto := autoEndFor(open.StartedAt)
		// The reminder sits on the same evening as the automatic close, so a
		// shift opened late at night is not flagged as overtime right away.
		reminder := at(auto, reminderHour, reminderMinute)
		out.ReminderAt = &reminder
		out.AutoEndAt = &auto
	}
	return out, nil
}

// Start opens a shift for the user; starting twice is a conflict.
func (s *Service) Start(ctx context.Context, userID uint, ip string) (*responses.ShiftStatus, error) {
	open, err := s.repo.Open(ctx, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if open != nil {
		return nil, errs.Conflict("Mesai zaten başlatılmış.", nil)
	}
	sh, err := s.repo.Start(ctx, userID, time.Now())
	if err != nil {
		return nil, errs.Internal(err)
	}
	if s.presence != nil {
		s.presence.ShiftStarted(ctx, userID)
	}
	s.record(ctx, &userID, enums.AuditShiftStarted, sh, ip, nil)
	return s.Current(ctx, userID)
}

// End closes the user's open shift.
func (s *Service) End(ctx context.Context, userID uint, ip string) (*responses.ShiftStatus, error) {
	open, err := s.repo.Open(ctx, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if open == nil {
		return nil, errs.Conflict("Başlatılmış bir mesai yok.", nil)
	}
	if err := s.close(ctx, open, time.Now(), EndedByUser, ip); err != nil {
		return nil, errs.Internal(err)
	}
	return s.Current(ctx, userID)
}

// StartSweeper launches the loop that closes shifts left open past the cutoff.
func (s *Service) StartSweeper(ctx context.Context) {
	go func() {
		s.sweep(ctx)
		t := time.NewTicker(sweepEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sweep(ctx)
			}
		}
	}()
}

// sweep closes every open shift whose cutoff has passed. The close is stamped
// at the cutoff itself, not at sweep time, so a restart after 19:20 does not
// credit the gap as working time.
func (s *Service) sweep(ctx context.Context) {
	open, err := s.repo.AllOpen(ctx)
	if err != nil {
		slog.WarnContext(ctx, "shift sweep failed", "error", err)
		return
	}
	now := time.Now()
	for i := range open {
		cutoff := autoEndFor(open[i].StartedAt)
		if now.Before(cutoff) {
			continue
		}
		if err := s.close(ctx, &open[i], cutoff, EndedByAuto, ""); err != nil {
			slog.WarnContext(ctx, "shift could not be auto-closed", "shift", open[i].ID, "error", err)
		}
	}
}

func (s *Service) close(ctx context.Context, sh *models.Shift, at time.Time, by, ip string) error {
	if err := s.repo.End(ctx, sh.ID, at, by); err != nil {
		return err
	}
	if s.presence != nil {
		s.presence.ShiftEnded(ctx, sh.UserID)
	}
	var actor *uint
	if by == EndedByUser {
		actor = &sh.UserID
	}
	s.record(ctx, actor, enums.AuditShiftEnded, sh, ip, map[string]any{"endedBy": by, "endedAt": at})
	return nil
}

func (s *Service) record(ctx context.Context, actor *uint, action string, sh *models.Shift, ip string, detail map[string]any) {
	if s.audit == nil {
		return
	}
	if detail == nil {
		detail = map[string]any{}
	}
	detail["userId"] = sh.UserID
	detail["startedAt"] = sh.StartedAt
	s.audit.Record(ctx, audit.Entry{
		ActorID:    actor,
		Action:     action,
		TargetType: "shift",
		TargetID:   strconv.FormatUint(uint64(sh.ID), 10),
		IP:         ip,
		Detail:     detail,
	})
}

// at returns the given wall-clock time (Istanbul) on the day the shift started.
func at(started time.Time, hour, minute int) time.Time {
	d := started.In(istanbul)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, istanbul)
}

// autoEndFor is the first 19:20 (Istanbul) after the shift started: the same
// evening normally, or the next one for a shift opened late at night.
func autoEndFor(started time.Time) time.Time {
	cutoff := at(started, autoEndHour, autoEndMinute)
	if !started.Before(cutoff) {
		cutoff = cutoff.Add(24 * time.Hour)
	}
	return cutoff
}
