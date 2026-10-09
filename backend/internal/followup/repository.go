package followup

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository reads and writes the follow-up records.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds the repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// AddAttempt records a call to a number that did not get through: a new
// open row for that person and number, or one more attempt on it.
func (r *Repository) AddAttempt(ctx context.Context, peerKey, peerNumber string, userID uint, at time.Time, callID, reason string) error {
	err := r.db.WithContext(ctx).Exec(`INSERT INTO call_unreached (peer_key, peer_number, user_id, attempts, first_at, last_at, last_call_id, last_reason)
		VALUES (?, ?, ?, 1, ?, ?, ?, ?)
		ON CONFLICT (peer_key, user_id) WHERE status = 'open' DO UPDATE SET
			attempts = call_unreached.attempts + 1,
			last_at = GREATEST(call_unreached.last_at, excluded.last_at),
			last_call_id = excluded.last_call_id, last_reason = excluded.last_reason,
			peer_number = excluded.peer_number, updated_at = now()`,
		peerKey, peerNumber, userID, at, at, callID, reason).Error
	if err != nil {
		return fmt.Errorf("unreached call could not be recorded: %w", err)
	}
	return nil
}

// Reach closes every open row of a number that a real conversation ended
// at `at` answers, and returns them.
func (r *Repository) Reach(ctx context.Context, peerKey string, by uint, direction string, seconds int, callID string, at time.Time) ([]models.CallUnreached, error) {
	var rows []models.CallUnreached
	err := r.db.WithContext(ctx).Raw(`UPDATE call_unreached SET status = 'reached', reached_at = ?, reached_by = ?,
			reached_direction = ?, reached_seconds = ?, reached_call_id = ?, claimed_by = NULL, claimed_until = NULL, updated_at = now()
		WHERE peer_key = ? AND status = 'open' AND first_at <= ?
		RETURNING *`, at, by, direction, seconds, callID, peerKey, at).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("unreached calls could not be closed: %w", err)
	}
	return rows, nil
}

// row is a follow-up record with the names it points at.
type row struct {
	models.CallUnreached
	UserName      string
	ReachedByName string
	ClaimedByName string
	DroppedByName string
}

const rowSelect = `SELECT u.*, COALESCE(o.name, '') AS user_name, COALESCE(rb.name, '') AS reached_by_name,
		COALESCE(cb.name, '') AS claimed_by_name, COALESCE(db.name, '') AS dropped_by_name
	FROM call_unreached u
	LEFT JOIN users o ON o.id = u.user_id
	LEFT JOIN users rb ON rb.id = u.reached_by
	LEFT JOIN users cb ON cb.id = u.claimed_by
	LEFT JOIN users db ON db.id = u.dropped_by`

// List reads the open rows whose last attempt is after since, or the rows
// closed after since, newest first; only one person's when userID is set.
func (r *Repository) List(ctx context.Context, userID *uint, open bool, since time.Time, roleID *uint) ([]row, error) {
	var rows []row
	args := []interface{}{userID, userID}
	q := rowSelect + ` WHERE (?::bigint IS NULL OR u.user_id = ?)`

	if roleID != nil {
		q += ` AND u.user_id IN (SELECT user_id FROM user_roles WHERE role_id = ?)`
		args = append(args, *roleID)
	}

	if open {
		q += ` AND u.status = 'open' AND u.last_at >= ? ORDER BY u.last_at DESC LIMIT 300`
	} else {
		q += ` AND u.status <> 'open' AND u.updated_at >= ? ORDER BY u.updated_at DESC LIMIT 300`
	}
	args = append(args, since)

	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("unreached calls could not be listed: %w", err)
	}
	return rows, nil
}

// OpenByPeer reads the open rows of a number whose last attempt is after
// since.
func (r *Repository) OpenByPeer(ctx context.Context, peerKey string, since time.Time) ([]row, error) {
	var rows []row
	if err := r.db.WithContext(ctx).Raw(rowSelect+` WHERE u.peer_key = ? AND u.status = 'open' AND u.last_at >= ? ORDER BY u.last_at DESC`,
		peerKey, since).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("unreached calls of a number could not be read: %w", err)
	}
	return rows, nil
}

// Get loads one row; nil when there is none.
func (r *Repository) Get(ctx context.Context, id uint) (*models.CallUnreached, error) {
	var rows []models.CallUnreached
	if err := r.db.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("unreached call could not be read: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// Claim marks an open row as being called back by someone until a time,
// unless another person holds it. It reports whether it was taken.
func (r *Repository) Claim(ctx context.Context, id, by uint, until time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE call_unreached SET claimed_by = ?, claimed_until = ?, updated_at = now()
		WHERE id = ? AND status = 'open' AND (claimed_by IS NULL OR claimed_until < now() OR claimed_by = ?)`, by, until, id, by)
	return res.RowsAffected > 0, res.Error
}

// Unclaim releases a row the person holds.
func (r *Repository) Unclaim(ctx context.Context, id, by uint) error {
	return r.db.WithContext(ctx).Exec(`UPDATE call_unreached SET claimed_by = NULL, claimed_until = NULL, updated_at = now()
		WHERE id = ? AND claimed_by = ?`, id, by).Error
}

// Drop closes an open row by hand: no call back is needed.
func (r *Repository) Drop(ctx context.Context, id, by uint) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`UPDATE call_unreached SET status = 'dropped', dropped_by = ?, dropped_at = now(),
			claimed_by = NULL, claimed_until = NULL, updated_at = now()
		WHERE id = ? AND status = 'open'`, by, id)
	return res.RowsAffected > 0, res.Error
}

// InboundBetween counts the calls a number made to the company between two
// times, from the phone system's records.
func (r *Repository) InboundBetween(ctx context.Context, peerKey string, from, to time.Time) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM pbx_cdrs
		WHERE direction = 'inbound' AND caller_num LIKE ? AND start_at >= ? AND start_at < ?`,
		"%"+peerKey, from, to).Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("calls of a number could not be counted: %w", err)
	}
	return int(n), nil
}

// talk is the last real conversation with a number.
type talk struct {
	UserID          uint
	Name            string
	StartedAt       time.Time
	DurationSeconds int
	Direction       string
}

// LastTalk reads the latest finished real conversation with a number since
// a time, whoever had it.
func (r *Repository) LastTalk(ctx context.Context, peerKey string, since time.Time, minSeconds int) (*talk, error) {
	var rows []talk
	if err := r.db.WithContext(ctx).Raw(`SELECT c.user_id, COALESCE(u.name, '') AS name, c.started_at, c.duration_seconds, c.direction
		FROM call_logs c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.peer_key = ? AND c.disposition = 'answered' AND c.ended_at IS NOT NULL AND c.user_id IS NOT NULL
			AND (c.duration_unknown OR c.duration_seconds >= ?) AND c.started_at >= ?
		ORDER BY c.started_at DESC LIMIT 1`, peerKey, minSeconds, since).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("last conversation with a number could not be read: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}
