package verimor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository reads and writes the SIP fields on users.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a Verimor repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// GetUser loads a user, or nil when absent.
func (r *Repository) GetUser(ctx context.Context, id uint) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user could not be fetched: %w", err)
	}
	return &u, nil
}

// SetPresence upserts the actor's presence state.
func (r *Repository) SetPresence(ctx context.Context, userID uint, state string) error {
	presence := models.AgentPresence{UserID: userID, State: state, UpdatedAt: time.Now()}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
		}).Create(&presence).Error; err != nil {
		return fmt.Errorf("presence could not be stored: %w", err)
	}
	return nil
}

// GetPresence returns the actor's presence state and since when it has held,
// defaulting to "available" with a zero time when no row exists yet.
func (r *Repository) GetPresence(ctx context.Context, userID uint) (string, time.Time, error) {
	var presence models.AgentPresence
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&presence).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "available", time.Time{}, nil
	}
	if err != nil {
		return "available", time.Time{}, fmt.Errorf("presence could not be fetched: %w", err)
	}
	return presence.State, presence.UpdatedAt, nil
}

// RecordTransition closes the agent's open presence stretch and opens a new one.
func (r *Repository) RecordTransition(ctx context.Context, userID uint, state string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&models.PresenceEvent{}).
			Where("user_id = ? AND ended_at IS NULL", userID).
			Update("ended_at", now).Error; err != nil {
			return fmt.Errorf("open presence event could not be closed: %w", err)
		}
		if err := tx.Create(&models.PresenceEvent{UserID: userID, State: state, StartedAt: now}).Error; err != nil {
			return fmt.Errorf("presence event could not be opened: %w", err)
		}
		return nil
	})
}

// EnsureOpenEvent opens a stretch for the agent's current state if none is open,
// so the timers start counting from when the agent first appears.
func (r *Repository) EnsureOpenEvent(ctx context.Context, userID uint, state string) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.PresenceEvent{}).
		Where("user_id = ? AND ended_at IS NULL", userID).Count(&count).Error; err != nil {
		return fmt.Errorf("open presence event could not be checked: %w", err)
	}
	if count > 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&models.PresenceEvent{UserID: userID, State: state, StartedAt: time.Now()}).Error
}

// OpenEventStartedAt returns when the agent's current (open) presence stretch
// began, so the panel's "since" timer is stable across page navigation.
func (r *Repository) OpenEventStartedAt(ctx context.Context, userID uint) (time.Time, bool, error) {
	var e models.PresenceEvent
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND ended_at IS NULL", userID).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("open presence event could not be read: %w", err)
	}
	return e.StartedAt, true, nil
}

// PresenceTotals sums the seconds the agent spent in each state since `from`,
// clamping each stretch to the window and counting open stretches up to now.
func (r *Repository) PresenceTotals(ctx context.Context, userID uint, from time.Time) (map[string]int64, error) {
	type row struct {
		State   string
		Seconds int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&models.PresenceEvent{}).
		Select("state, COALESCE(SUM(EXTRACT(EPOCH FROM (COALESCE(ended_at, now()) - GREATEST(started_at, ?)))), 0)::bigint AS seconds", from).
		Where("user_id = ? AND COALESCE(ended_at, now()) >= ?", userID, from).
		Group("state").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("presence totals could not be computed: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		if r.Seconds > 0 {
			out[r.State] = r.Seconds
		}
	}
	return out, nil
}

// openCallCap bounds how long a still-open call log (one whose hangup was never
// recorded, e.g. the tab closed mid-call) may contribute to today's call time.
// A closed call always uses its real end; only an open row is clamped, so a lost
// call can never inflate the total indefinitely.
const openCallCap = 30 * time.Minute

// CallSecondsToday sums the time the agent spent on calls since `from` (from the
// attempt to hangup, including a call still in progress). This is subtracted
// from available time so a call does not also count as idle/available. A call
// still in progress is clamped to openCallCap so an unclosed row (its end phase
// never arrived) cannot count up to now() forever.
func (r *Repository) CallSecondsToday(ctx context.Context, userID uint, from time.Time) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Table("call_logs").
		Where("user_id = ? AND started_at >= ?", userID, from).
		Select(
			"COALESCE(SUM(EXTRACT(EPOCH FROM ("+
				"CASE WHEN ended_at IS NULL THEN LEAST(now(), started_at + ?::interval) ELSE ended_at END"+
				" - GREATEST(started_at, ?)))), 0)::bigint",
			fmt.Sprintf("%d seconds", int64(openCallCap.Seconds())), from).
		Row().Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("call seconds could not be summed: %w", err)
	}
	if total < 0 {
		total = 0
	}
	return total, nil
}

// FinalizeStaleCalls closes call logs that are still open past openCallCap (their
// hangup was never recorded), so they stop reading as "in progress" and stop
// contributing unbounded time. A genuine late end phase still overwrites the row
// by call id, so finalizing early is safe. Returns how many rows were closed.
func (r *Repository) FinalizeStaleCalls(ctx context.Context) (int64, error) {
	bound := fmt.Sprintf("%d seconds", int64(openCallCap.Seconds()))
	res := r.db.WithContext(ctx).Exec(
		"UPDATE call_logs SET "+
			"ended_at = LEAST(now(), started_at + ?::interval), "+
			"duration_seconds = GREATEST(0, EXTRACT(EPOCH FROM (LEAST(now(), started_at + ?::interval) - COALESCE(answered_at, started_at)))::int), "+
			"disposition = CASE WHEN answered_at IS NULL THEN 'no_answer' ELSE 'answered' END "+
			"WHERE ended_at IS NULL AND started_at < now() - ?::interval",
		bound, bound, bound)
	if res.Error != nil {
		return 0, fmt.Errorf("stale calls could not be finalized: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// PresenceByExtension maps each extension to its stored non-available presence
// state, so the live agent list can reflect who is on a break or in backoffice.
func (r *Repository) PresenceByExtension(ctx context.Context) (map[string]string, error) {
	type row struct {
		Extension string
		State     string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("agent_presence AS p").
		Select("u.sip_extension AS extension, p.state AS state").
		Joins("JOIN users u ON u.id = p.user_id").
		Where("p.state <> 'available' AND u.sip_extension IS NOT NULL AND u.sip_extension <> ''").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("presence map could not be built: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Extension] = r.State
	}
	return out, nil
}

// UserExtension pairs a user id with their SIP extension.
type UserExtension struct {
	ID        uint
	Extension string
}

// UsersWithExtension returns every user that has a SIP extension assigned.
func (r *Repository) UsersWithExtension(ctx context.Context) ([]UserExtension, error) {
	var rows []UserExtension
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Select("id, sip_extension AS extension").
		Where("sip_extension IS NOT NULL AND sip_extension <> ''").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("users with extension could not be listed: %w", err)
	}
	return rows, nil
}

// SetSIP stores a user's SIP extension and encrypted SIP password.
func (r *Repository) SetSIP(ctx context.Context, id uint, extension, encPassword string) error {
	if err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"sip_extension":   extension,
			"sip_secret":      encPassword,
			"sip_provisioned": true,
		}).Error; err != nil {
		return fmt.Errorf("sip credentials could not be stored: %w", err)
	}
	return nil
}
