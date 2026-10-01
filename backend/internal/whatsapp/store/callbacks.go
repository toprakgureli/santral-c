package store

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// CreateCallback stores a "call me back" request.
func (r *Repository) CreateCallback(ctx context.Context, cb *models.WACallback) error {
	return r.db.WithContext(ctx).Create(cb).Error
}

// Callback is a "call me back" request with the names it is shown with.
type Callback struct {
	ID             uint
	ChannelName    string
	ConversationID uint
	Customer       string
	Phone          string
	Note           string
	Status         string
	DoneBy         string
	DoneAt         *time.Time
	CreatedAt      time.Time
}

// Callbacks lists up to 300 requests on the given devices, open ones first
// and then the newest. Unless all is set, only the open ones and those
// closed in the last day are listed.
func (r *Repository) Callbacks(ctx context.Context, all bool, devices Devices) ([]Callback, error) {
	q := `SELECT b.id, COALESCE(ch.name, '') AS channel_name, COALESCE(t.conversation_id, 0) AS conversation_id,
		COALESCE(NULLIF(c.name, ''), NULLIF(c.profile_name, ''), b.phone) AS customer, b.phone, b.note, b.status,
		COALESCE(u.name, '') AS done_by, b.done_at, b.created_at
		FROM wa_callbacks b LEFT JOIN wa_channels ch ON ch.id = b.channel_id LEFT JOIN wa_contacts c ON c.id = b.contact_id
		LEFT JOIN wa_tickets t ON t.id = b.ticket_id LEFT JOIN users u ON u.id = b.done_by
		WHERE true`
	cond, args := devices.clause("b.channel_id")
	q += cond
	if !all {
		q += " AND (b.status = 'open' OR b.done_at > now() - interval '1 day')"
	}
	q += " ORDER BY (b.status = 'open') DESC, b.id DESC LIMIT 300"
	var out []Callback
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// DoneCallback closes a request on the given devices in a person's name and
// returns how many requests it found.
func (r *Repository) DoneCallback(ctx context.Context, id, userID uint, devices Devices) (int64, error) {
	cond, args := devices.clause("channel_id")
	res := r.db.WithContext(ctx).Exec(`UPDATE wa_callbacks SET status = 'done', done_by = COALESCE(done_by, ?), done_at = COALESCE(done_at, now())
		WHERE id = ?`+cond, append([]any{userID, id}, args...)...)
	return res.RowsAffected, res.Error
}
