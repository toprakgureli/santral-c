package verimor

import (
	"context"
	"database/sql"
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

// CloseOpenEvent ends the agent's open presence stretch without opening a new
// one, so no time accrues while off shift.
func (r *Repository) CloseOpenEvent(ctx context.Context, userID uint) error {
	if err := r.db.WithContext(ctx).Model(&models.PresenceEvent{}).
		Where("user_id = ? AND ended_at IS NULL", userID).
		Update("ended_at", time.Now()).Error; err != nil {
		return fmt.Errorf("open presence event could not be closed: %w", err)
	}
	return nil
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

// PauseStretches lists the agent's non-available presence stretches that
// touch the window starting at `from`, oldest first, so the panel can show
// the breaks of the day.
func (r *Repository) PauseStretches(ctx context.Context, userID uint, from time.Time) ([]models.PresenceEvent, error) {
	var events []models.PresenceEvent
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND state <> 'available' AND COALESCE(ended_at, now()) >= ?", userID, from).
		Order("started_at").
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("pause stretches could not be listed: %w", err)
	}
	return events, nil
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
// Long support calls are real, so the cap is generous; a genuine end phase
// still overwrites the row with the true duration whenever it arrives.
const openCallCap = 2 * time.Hour

// CallSecondsToday sums the agent's actual talk time since `from`: from when a
// call was answered to when it ended. Only answered calls count, so ring time
// and unanswered calls are excluded (talk time can never exceed online time).
// A call still in progress is clamped to openCallCap so an unclosed row (its end
// phase never arrived) cannot count up to now() forever. This is also subtracted
// from available time so a call does not double as idle/available.
func (r *Repository) CallSecondsToday(ctx context.Context, userID uint, from time.Time) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Table("call_logs").
		Where("user_id = ? AND answered_at IS NOT NULL AND answered_at >= ?", userID, from).
		Select(
			"COALESCE(SUM(EXTRACT(EPOCH FROM ("+
				"CASE WHEN ended_at IS NULL THEN LEAST(now(), answered_at + ?::interval) ELSE ended_at END"+
				" - answered_at))), 0)::bigint",
			fmt.Sprintf("%d seconds", int64(openCallCap.Seconds()))).
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
			"duration_seconds = CASE WHEN answered_at IS NULL THEN 0 "+
			"ELSE GREATEST(0, EXTRACT(EPOCH FROM (LEAST(now(), answered_at + ?::interval) - answered_at))::int) END, "+
			"disposition = CASE WHEN answered_at IS NULL THEN 'no_answer' ELSE 'answered' END "+
			"WHERE ended_at IS NULL AND started_at < now() - ?::interval",
		bound, bound, bound)
	if res.Error != nil {
		return 0, fmt.Errorf("stale calls could not be finalized: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// LastCallEndedAt returns when the agent's most recent answered call ended
// today, so the "current idle" timer counts only the stretch since the last
// call rather than the whole available presence stretch (which spans calls).
func (r *Repository) LastCallEndedAt(ctx context.Context, userID uint, from time.Time) (time.Time, bool, error) {
	var last sql.NullTime
	err := r.db.WithContext(ctx).
		Table("call_logs").
		Where("user_id = ? AND answered_at IS NOT NULL AND ended_at IS NOT NULL AND ended_at >= ?", userID, from).
		Select("MAX(ended_at)").
		Row().Scan(&last)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last call end could not be read: %w", err)
	}
	if !last.Valid {
		return time.Time{}, false, nil
	}
	return last.Time, true, nil
}

// OpenPeersByExtension maps each extension to the other party of its open
// panel call (the newest in-progress call log of a user on that extension),
// so the agent list can say who a talking agent is talking to. Only calls
// started within openCallCap count, so a lost hangup cannot pin a stale peer.
func (r *Repository) OpenPeersByExtension(ctx context.Context) (map[string]string, error) {
	type row struct {
		Extension string
		Peer      string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("call_logs AS c").
		Select("u.sip_extension AS extension, c.peer_number AS peer").
		Joins("JOIN users u ON u.id = c.user_id").
		Where("c.disposition = 'in_progress' AND c.started_at >= ? AND u.sip_extension IS NOT NULL AND u.sip_extension <> ''", time.Now().Add(-openCallCap)).
		Order("c.started_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("open peers could not be listed: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		if _, seen := out[r.Extension]; !seen {
			out[r.Extension] = r.Peer
		}
	}
	return out, nil
}

// NamesByExtension maps each extension to the active users registered on it,
// ordered by name, so the agent list can show who sits behind a number.
func (r *Repository) NamesByExtension(ctx context.Context) (map[string][]string, error) {
	users, err := r.UsersByExtension(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(users))
	for ext, list := range users {
		for _, u := range list {
			out[ext] = append(out[ext], u.Name)
		}
	}
	return out, nil
}

// ExtUser is one active panel user on an extension.
type ExtUser struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	HasAvatar bool   `json:"hasAvatar"`
}

// UsersByExtension maps each extension to the active panel users on it, so
// the agent list can show their photos.
func (r *Repository) UsersByExtension(ctx context.Context) (map[string][]ExtUser, error) {
	type row struct {
		Extension string
		ID        uint
		Name      string
		Avatar    string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("users").
		Select("sip_extension AS extension, id, name, avatar").
		Where("active = TRUE AND sip_extension IS NOT NULL AND sip_extension <> ''").
		Order("name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("extension users could not be listed: %w", err)
	}
	out := make(map[string][]ExtUser, len(rows))
	for _, r := range rows {
		out[r.Extension] = append(out[r.Extension], ExtUser{ID: r.ID, Name: r.Name, HasAvatar: r.Avatar != ""})
	}
	return out, nil
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
