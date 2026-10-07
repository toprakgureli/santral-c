package performance

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

// Live alerts for whoever leads the team (performance.live_alerts): what
// needs a look right now, so they do not have to watch the team page all
// day. Each alert keeps the same key while its cause lasts, so the panel
// shows a new one once and drops it when it is over.

const (
	// longCallAfter: a conversation longer than this is pointed out.
	longCallAfter = 20 * time.Minute
	// missedWindow and missedAtLeast: this many incoming calls left
	// unanswered within the window is a pile-up.
	missedWindow  = 15 * time.Minute
	missedAtLeast = 3
	// unreachedWait: a number nobody called back for this long.
	unreachedWait = 2 * time.Hour
	// reminderLate: a planned call back this far past its time.
	reminderLate = 30 * time.Minute
)

// IBreakLimit reads the daily break limit.
type IBreakLimit interface {
	BreakLimitMinutes(ctx context.Context) int
}

// SetBreakLimit wires the daily break limit.
func (s *Service) SetBreakLimit(b IBreakLimit) { s.breakLimit = b }

// Alert is one thing that needs a look.
type Alert struct {
	Key   string    `json:"key"`
	Kind  string    `json:"kind"`  // long_call, break_over, missed, unreached, reminder_late
	Level string    `json:"level"` // warning or info
	Text  string    `json:"text"`
	Link  string    `json:"link,omitempty"`
	Since time.Time `json:"since"`
}

// pile is a count with the time its oldest member started.
type pile struct {
	N      int64
	Oldest *time.Time
}

func (r *Repository) missedSince(ctx context.Context, since time.Time) (pile, error) {
	var p pile
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) AS n, min(start_at) AS oldest FROM pbx_cdrs
		WHERE direction = 'inbound' AND start_at >= ? AND COALESCE(answer_stamp, '') = ''`, since).Scan(&p).Error
	if err != nil {
		return p, fmt.Errorf("missed calls could not be counted: %w", err)
	}
	return p, nil
}

func (r *Repository) unreachedWaiting(ctx context.Context, before, after time.Time) (pile, error) {
	var p pile
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) AS n, min(last_at) AS oldest FROM call_unreached
		WHERE status = 'open' AND last_at < ? AND last_at >= ? AND (claimed_by IS NULL OR claimed_until < now())`, before, after).Scan(&p).Error
	if err != nil {
		return p, fmt.Errorf("waiting follow-ups could not be counted: %w", err)
	}
	return p, nil
}

type lateReminders struct {
	UserID uint
	Name   string
	N      int64
	Oldest time.Time
}

func (r *Repository) remindersLate(ctx context.Context, before time.Time) ([]lateReminders, error) {
	var rows []lateReminders
	err := r.db.WithContext(ctx).Raw(`SELECT c.user_id, COALESCE(u.name, '') AS name, count(*) AS n, min(c.due_at) AS oldest
		FROM call_reminders c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.done_at IS NULL AND c.due_at < ? GROUP BY c.user_id, u.name`, before).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("late call backs could not be counted: %w", err)
	}
	return rows, nil
}

// Alerts lists what needs a look right now, the most urgent first.
func (s *Service) Alerts(ctx context.Context, actorID uint) ([]Alert, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.PerformanceLiveAlerts) {
		return nil, errs.Forbidden("Canlı uyarıları görme yetkin yok.")
	}
	now := time.Now()
	local := now.In(tz.Istanbul)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, tz.Istanbul)
	out := []Alert{}

	agents, err := s.repo.Agents(ctx, nil, false)
	if err != nil {
		return nil, errs.Internal(err)
	}
	names := make(map[uint]string, len(agents))
	for _, a := range agents {
		names[a.ID] = a.Name
	}

	// Conversations running long.
	open, err := s.repo.OpenCalls(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	for uid, c := range open {
		name, ok := names[uid]
		if !ok || c.AnsweredAt == nil || now.Sub(*c.AnsweredAt) < longCallAfter {
			continue
		}
		out = append(out, Alert{Key: "long_call:" + c.CallID, Kind: "long_call", Level: "info",
			Text: fmt.Sprintf("%s %d dakikadır görüşmede.", name, int(now.Sub(*c.AnsweredAt).Minutes())), Link: "/performance", Since: *c.AnsweredAt})
	}

	// Someone on break past the day's limit.
	if s.breakLimit != nil {
		limit := int64(s.breakLimit.BreakLimitMinutes(ctx)) * 60
		presence, err := s.repo.Presence(ctx)
		if err != nil {
			return nil, errs.Internal(err)
		}
		breaks, err := s.repo.BreakSeconds(ctx, midnight, now)
		if err != nil {
			return nil, errs.Internal(err)
		}
		for uid, p := range presence {
			name, ok := names[uid]
			if !ok || p.State != "break" || limit <= 0 || breaks[uid] <= limit {
				continue
			}
			out = append(out, Alert{Key: fmt.Sprintf("break_over:%d:%s", uid, midnight.Format("2006-01-02")), Kind: "break_over", Level: "warning",
				Text: fmt.Sprintf("%s molada; bugünkü mola %d dk, sınır %d dk.", name, breaks[uid]/60, limit/60), Link: "/performance", Since: p.UpdatedAt})
		}
	}

	// Incoming calls left unanswered in a short time.
	missed, err := s.repo.missedSince(ctx, now.Add(-missedWindow))
	if err != nil {
		return nil, errs.Internal(err)
	}
	if missed.N >= missedAtLeast && missed.Oldest != nil {
		out = append(out, Alert{Key: "missed", Kind: "missed", Level: "warning",
			Text: fmt.Sprintf("Son %d dakikada %d gelen çağrı cevaplanmadı.", int(missedWindow.Minutes()), missed.N), Link: "/calls", Since: *missed.Oldest})
	}

	// Numbers nobody called back.
	waiting, err := s.repo.unreachedWaiting(ctx, now.Add(-unreachedWait), now.Add(-24*time.Hour))
	if err != nil {
		return nil, errs.Internal(err)
	}
	if waiting.N > 0 && waiting.Oldest != nil {
		out = append(out, Alert{Key: "unreached", Kind: "unreached", Level: "info",
			Text: fmt.Sprintf("%d müşteriye 2 saattir kimse geri dönmedi.", waiting.N), Link: "/followups", Since: *waiting.Oldest})
	}

	// Planned call backs past their time.
	late, err := s.repo.remindersLate(ctx, now.Add(-reminderLate))
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, l := range late {
		if _, ok := names[l.UserID]; !ok {
			continue
		}
		out = append(out, Alert{Key: fmt.Sprintf("reminder_late:%d", l.UserID), Kind: "reminder_late", Level: "info",
			Text: fmt.Sprintf("%s adına %d planlı geri arama 30 dakikadan uzun süredir gecikiyor.", l.Name, l.N), Link: "/followups?tab=reminders", Since: l.Oldest})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Level == "warning") != (out[j].Level == "warning") {
			return out[i].Level == "warning"
		}
		return out[i].Since.Before(out[j].Since)
	})
	return out, nil
}
