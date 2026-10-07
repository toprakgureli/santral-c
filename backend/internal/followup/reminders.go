package followup

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// Planned call backs: someone promises a customer a call at a time ("we
// will call you at three"). Once due, the panel reminds its owner on every
// page until they call, snooze it or close it; a real conversation with
// the number, by anyone, closes it by itself.

const (
	// reminderAhead is the furthest a call back can be planned.
	reminderAhead = 30 * 24 * time.Hour
	// reminderSkew lets a time a moment in the past through (the clock of
	// the browser that picked "now").
	reminderSkew    = 5 * time.Minute
	reminderNoteMax = 300
	// doneShownFor is how long closed call backs stay listed.
	doneShownFor = 7 * 24 * time.Hour
)

// ReminderInput is a new call back.
type ReminderInput struct {
	Number string    `json:"number"`
	Note   string    `json:"note"`
	DueAt  time.Time `json:"dueAt"`
}

// Reminder is one planned call back.
type Reminder struct {
	ID         uint       `json:"id"`
	Number     string     `json:"number"`
	Note       string     `json:"note,omitempty"`
	DueAt      time.Time  `json:"dueAt"`
	User       Person     `json:"user"`
	Mine       bool       `json:"mine"`
	Snoozes    int        `json:"snoozes"`
	CreatedAt  time.Time  `json:"createdAt"`
	DoneAt     *time.Time `json:"doneAt,omitempty"`
	DoneReason string     `json:"doneReason,omitempty"`
	DoneBy     *Person    `json:"doneBy,omitempty"`
}

// reminderRow is a call back with the names it points at.
type reminderRow struct {
	models.CallReminder
	UserName   string
	DoneByName string
}

const reminderSelect = `SELECT r.*, COALESCE(o.name, '') AS user_name, COALESCE(d.name, '') AS done_by_name
	FROM call_reminders r LEFT JOIN users o ON o.id = r.user_id LEFT JOIN users d ON d.id = r.done_by`

func (r *Repository) createReminder(ctx context.Context, m *models.CallReminder) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *Repository) reminders(ctx context.Context, userID *uint, done bool, since time.Time) ([]reminderRow, error) {
	var rows []reminderRow
	q := reminderSelect + ` WHERE (?::bigint IS NULL OR r.user_id = ?)`
	if done {
		q += ` AND r.done_at IS NOT NULL AND r.done_at >= ? ORDER BY r.done_at DESC LIMIT 300`
	} else {
		q += ` AND r.done_at IS NULL AND r.created_at >= ? ORDER BY r.due_at LIMIT 300`
		since = time.Time{}
	}
	if err := r.db.WithContext(ctx).Raw(q, userID, userID, since).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("call backs could not be listed: %w", err)
	}
	return rows, nil
}

func (r *Repository) remindersOfPeer(ctx context.Context, peerKey string) ([]reminderRow, error) {
	var rows []reminderRow
	if err := r.db.WithContext(ctx).Raw(reminderSelect+` WHERE r.peer_key = ? AND r.done_at IS NULL ORDER BY r.due_at`, peerKey).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("call backs of a number could not be read: %w", err)
	}
	return rows, nil
}

func (r *Repository) reminder(ctx context.Context, id uint) (*models.CallReminder, error) {
	var rows []models.CallReminder
	if err := r.db.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("call back could not be read: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (r *Repository) snoozeReminder(ctx context.Context, id uint, until time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE call_reminders SET due_at = ?, snoozes = snoozes + 1 WHERE id = ? AND done_at IS NULL`, until, id)
	return res.RowsAffected > 0, res.Error
}

func (r *Repository) closeReminder(ctx context.Context, id, by uint, reason string) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE call_reminders SET done_at = now(), done_reason = ?, done_by = ? WHERE id = ? AND done_at IS NULL`, reason, by, id)
	return res.RowsAffected > 0, res.Error
}

// reachReminders closes the open call backs of a number planned before a
// real conversation ended at `at`, and returns them.
func (r *Repository) reachReminders(ctx context.Context, peerKey string, by uint, at time.Time) ([]models.CallReminder, error) {
	var rows []models.CallReminder
	if err := r.db.WithContext(ctx).Raw(`UPDATE call_reminders SET done_at = ?, done_reason = 'reached', done_by = ?
		WHERE peer_key = ? AND done_at IS NULL AND created_at <= ? RETURNING *`, at, by, peerKey, at).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("call backs could not be closed: %w", err)
	}
	return rows, nil
}

func reminderView(r reminderRow, actorID uint) Reminder {
	v := Reminder{ID: r.ID, Number: r.PeerNumber, Note: r.Note, DueAt: r.DueAt, User: Person{r.UserID, r.UserName}, Mine: r.UserID == actorID,
		Snoozes: r.Snoozes, CreatedAt: r.CreatedAt, DoneAt: r.DoneAt}
	if r.DoneReason != nil {
		v.DoneReason = *r.DoneReason
	}
	if r.DoneBy != nil {
		v.DoneBy = &Person{*r.DoneBy, r.DoneByName}
	}
	return v
}

// CreateReminder plans a call back to a number for the actor.
func (s *Service) CreateReminder(ctx context.Context, actorID uint, in ReminderInput) (*Reminder, error) {
	actor, _, err := s.scope(ctx, actorID, false)
	if err != nil {
		return nil, err
	}
	number := strings.TrimSpace(in.Number)
	key := phone.Key(number)
	if len(key) <= 5 {
		return nil, errs.Invalid("Geri aranacak telefon numarasını yaz.", nil)
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > reminderNoteMax {
		return nil, errs.Invalid(fmt.Sprintf("Not en fazla %d karakter olabilir.", reminderNoteMax), nil)
	}
	now := s.now()
	if in.DueAt.Before(now.Add(-reminderSkew)) || in.DueAt.After(now.Add(reminderAhead)) {
		return nil, errs.Invalid("Geri arama zamanı şimdiden sonra ve en fazla 30 gün ileride olmalı.", nil)
	}
	m := &models.CallReminder{UserID: actor.ID, PeerNumber: number, PeerKey: key, Note: note, DueAt: in.DueAt}
	if err := s.repo.createReminder(ctx, m); err != nil {
		return nil, errs.Internal(err)
	}
	v := reminderView(reminderRow{CallReminder: *m, UserName: actor.Name}, actor.ID)
	return &v, nil
}

// Reminders lists the call backs still to make, soonest first, or those
// closed in the last week; the actor's own or, with all, everyone's.
func (s *Service) Reminders(ctx context.Context, actorID uint, all, done bool) ([]Reminder, error) {
	actor, only, err := s.scope(ctx, actorID, all)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.reminders(ctx, only, done, s.now().Add(-doneShownFor))
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]Reminder, 0, len(rows))
	for _, r := range rows {
		out = append(out, reminderView(r, actor.ID))
	}
	return out, nil
}

// ownReminder loads a call back the actor may change: their own, or
// anyone's with call.view_all.
func (s *Service) ownReminder(ctx context.Context, actorID, id uint) (*models.CallReminder, error) {
	actor, _, err := s.scope(ctx, actorID, false)
	if err != nil {
		return nil, err
	}
	r, err := s.repo.reminder(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	seesAll := actor.Can(enums.CallViewAll) || actor.Can(enums.CDRViewAll)
	if r == nil || (r.UserID != actorID && !seesAll) {
		return nil, errs.NotFound("Geri arama bulunamadı.")
	}
	return r, nil
}

// SnoozeReminder moves a call back later by some minutes.
func (s *Service) SnoozeReminder(ctx context.Context, actorID, id uint, minutes int) error {
	if minutes < 5 || minutes > 24*60 {
		return errs.Invalid("Erteleme 5 dakika ile 1 gün arasında olmalı.", nil)
	}
	if _, err := s.ownReminder(ctx, actorID, id); err != nil {
		return err
	}
	ok, err := s.repo.snoozeReminder(ctx, id, s.now().Add(time.Duration(minutes)*time.Minute))
	if err != nil {
		return errs.Internal(err)
	}
	if !ok {
		return errs.NotFound("Geri arama zaten kapanmış.")
	}
	return nil
}

// CloseReminder closes a call back by hand: made ("done") or no longer
// needed ("canceled").
func (s *Service) CloseReminder(ctx context.Context, actorID, id uint, reason string) error {
	if reason != "done" && reason != "canceled" {
		return errs.Invalid("Geçersiz işlem.", nil)
	}
	if _, err := s.ownReminder(ctx, actorID, id); err != nil {
		return err
	}
	ok, err := s.repo.closeReminder(ctx, id, actorID, reason)
	if err != nil {
		return errs.Internal(err)
	}
	if !ok {
		return errs.NotFound("Geri arama zaten kapanmış.")
	}
	return nil
}

// tellRemindersReached tells the owners of call backs a colleague's
// conversation made unneeded.
func (s *Service) tellRemindersReached(ctx context.Context, rows []models.CallReminder, log models.CallLog, who string, at time.Time) {
	for _, r := range rows {
		if r.UserID == *log.UserID {
			continue
		}
		text := fmt.Sprintf("%s için planladığın geri arama (saat %s) kapandı: %s ile %s görüştü (%s).",
			r.PeerNumber, clock(r.DueAt), who, talkLabel(log.DurationSeconds), clock(at))
		s.notices.Add(ctx, r.UserID, "reminder_reached", text, "/followups?tab=reminders")
	}
}
