package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/toprakgureli/santral-c/backend/pkg/callrule"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

// The day's summary: what the person did today, shown when they end their
// shift and five minutes before the working day ends. Today is set against
// their own target and their own last week, never against colleagues.

// weekBack is how many days before today the comparison reaches.
const weekBack = 7

// ITargets reads the daily target of someone holding the given roles.
type ITargets interface {
	TargetFor(ctx context.Context, roleIDs []uint) int
}

// SetTargets wires the daily targets.
func (s *Service) SetTargets(t ITargets) { s.targets = t }

// DayFollowups counts the day's follow-ups: numbers the person could not
// reach today, how many of them someone reached since, and the call backs
// still to make.
type DayFollowups struct {
	Unreached     int64 `json:"unreached"`
	Reached       int64 `json:"reached"`
	RemindersOpen int64 `json:"remindersOpen"`
}

// DaySummary is the person's day.
type DaySummary struct {
	Today Record `json:"today"`
	// Target is the person's daily target of real calls, 0 for none.
	Target int `json:"target"`
	// WeekAverage is the real calls per day over the days with calls in
	// the week before today; WeekBest the best of those days.
	WeekAverage float64      `json:"weekAverage"`
	WeekBest    int64        `json:"weekBest"`
	WeekDays    int          `json:"weekDays"`
	Followups   DayFollowups `json:"followups"`
	RealSeconds int          `json:"realSeconds"`
}

func (r *Repository) followups(ctx context.Context, id uint, since time.Time) (DayFollowups, error) {
	var out DayFollowups
	err := r.db.WithContext(ctx).Raw(`SELECT
		(SELECT count(*) FROM call_unreached WHERE user_id = ? AND first_at >= ?) AS unreached,
		(SELECT count(*) FROM call_unreached WHERE user_id = ? AND first_at >= ? AND status = 'reached') AS reached,
		(SELECT count(*) FROM call_reminders WHERE user_id = ? AND done_at IS NULL) AS reminders_open`,
		id, since, id, since, id).Scan(&out).Error
	if err != nil {
		return out, fmt.Errorf("the day's follow-ups could not be counted: %w", err)
	}
	return out, nil
}

// Summary returns the actor's own day.
func (s *Service) Summary(ctx context.Context, actorID uint) (*DaySummary, error) {
	u, err := s.repo.User(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if u == nil {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	real := callrule.Seconds(ctx, s.realCall)
	now := time.Now().In(tz.Istanbul)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz.Istanbul)
	out := &DaySummary{RealSeconds: real}
	if out.Today, err = s.repo.Record(ctx, actorID, midnight, midnight.AddDate(0, 0, 1), real); err != nil {
		return nil, errs.Internal(err)
	}
	day := midnight.Format("2006-01-02")
	out.Today.From, out.Today.To = day, day
	week, err := s.repo.Record(ctx, actorID, midnight.AddDate(0, 0, -weekBack), midnight, real)
	if err != nil {
		return nil, errs.Internal(err)
	}
	var sum int64
	for _, d := range week.Days {
		if d.Real > 0 {
			sum += d.Real
			out.WeekDays++
			out.WeekBest = max(out.WeekBest, d.Real)
		}
	}
	if out.WeekDays > 0 {
		out.WeekAverage = float64(sum) / float64(out.WeekDays)
	}
	if s.targets != nil {
		ids := make([]uint, 0, len(u.Roles))
		for _, r := range u.Roles {
			ids = append(ids, r.ID)
		}
		out.Target = s.targets.TargetFor(ctx, ids)
	}
	if out.Followups, err = s.repo.followups(ctx, actorID, midnight); err != nil {
		return nil, errs.Internal(err)
	}
	return out, nil
}

// Summary returns the caller's day.
func (h *Handler) Summary(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Summary(c.UserContext(), id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}
