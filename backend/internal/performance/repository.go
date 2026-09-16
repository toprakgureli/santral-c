package performance

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/calllog"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// Repository reads the per-agent figures the team page shows.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a performance repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Agents lists active users that have an extension, with their roles. When
// roleIDs is non-empty only users holding at least one of those roles are
// returned.
func (r *Repository) Agents(ctx context.Context, roleIDs []uint) ([]models.User, error) {
	q := r.db.WithContext(ctx).Preload("Roles").
		Where("users.active = ? AND users.sip_extension IS NOT NULL AND users.sip_extension <> ''", true)
	if len(roleIDs) > 0 {
		q = q.Where("users.id IN (?)", r.db.Table("user_roles").Select("user_id").Where("role_id IN ?", roleIDs))
	}
	var users []models.User
	if err := q.Order("users.name").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("agents could not be listed: %w", err)
	}
	return users, nil
}

// Counts is one agent's call breakdown since a point in time.
type Counts struct {
	UserID      uint  `json:"-"`
	Total       int64 `json:"total"`
	Answered    int64 `json:"answered"`
	Short       int64 `json:"short"`
	Long        int64 `json:"long"`
	Unanswered  int64 `json:"unanswered"`
	Inbound     int64 `json:"inbound"`
	Outbound    int64 `json:"outbound"`
	TalkSeconds int64 `json:"talkSeconds"`
}

// CallCounts aggregates the call log per user since `from`.
func (r *Repository) CallCounts(ctx context.Context, from time.Time, shortLong int) (map[uint]Counts, error) {
	var rows []Counts
	err := r.db.WithContext(ctx).Model(&models.CallLog{}).
		Select("user_id, "+
			"count(*) AS total, "+
			"count(*) FILTER (WHERE disposition = 'answered') AS answered, "+
			"count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds < ?) AS short, "+
			"count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds >= ?) AS long, "+
			"count(*) FILTER (WHERE disposition NOT IN ('answered', 'in_progress')) AS unanswered, "+
			"count(*) FILTER (WHERE direction = 'inbound') AS inbound, "+
			"count(*) FILTER (WHERE direction = 'outbound') AS outbound, "+
			"COALESCE(SUM(duration_seconds) FILTER (WHERE disposition = 'answered'), 0) AS talk_seconds",
			shortLong, shortLong).
		Where("user_id IS NOT NULL AND started_at >= ?", from).
		// A ring that a teammate answered is not this agent's call.
		Where("NOT (" + calllog.AnsweredElsewhereSQL + ")").
		Group("user_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("call counts could not be computed: %w", err)
	}
	out := make(map[uint]Counts, len(rows))
	for _, c := range rows {
		out[c.UserID] = c
	}
	return out, nil
}

// OpenCalls returns each user's call that is still in progress, newest first
// so a stale duplicate never shadows the live one.
func (r *Repository) OpenCalls(ctx context.Context) (map[uint]models.CallLog, error) {
	var logs []models.CallLog
	err := r.db.WithContext(ctx).
		Where("disposition = 'in_progress' AND user_id IS NOT NULL").
		Order("started_at DESC").
		Find(&logs).Error
	if err != nil {
		return nil, fmt.Errorf("open calls could not be listed: %w", err)
	}
	out := make(map[uint]models.CallLog)
	for _, l := range logs {
		if _, seen := out[*l.UserID]; !seen {
			out[*l.UserID] = l
		}
	}
	return out, nil
}

// Presence returns every stored presence state keyed by user.
func (r *Repository) Presence(ctx context.Context) (map[uint]models.AgentPresence, error) {
	var rows []models.AgentPresence
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("presence could not be listed: %w", err)
	}
	out := make(map[uint]models.AgentPresence, len(rows))
	for _, p := range rows {
		out[p.UserID] = p
	}
	return out, nil
}

// ShiftInfo is an agent's shift picture for today: when the open shift began
// (nil when off shift) and the seconds worked since the day started.
type ShiftInfo struct {
	StartedAt *time.Time `json:"startedAt,omitempty"`
	Seconds   int64      `json:"seconds"`
}

// Shifts sums today's shift time per user and reports the open shift, if any.
func (r *Repository) Shifts(ctx context.Context, dayStart time.Time) (map[uint]ShiftInfo, error) {
	var shifts []models.Shift
	err := r.db.WithContext(ctx).
		Where("ended_at IS NULL OR ended_at >= ?", dayStart).
		Order("started_at").
		Find(&shifts).Error
	if err != nil {
		return nil, fmt.Errorf("shifts could not be listed: %w", err)
	}
	now := time.Now()
	out := make(map[uint]ShiftInfo)
	for i := range shifts {
		s := shifts[i]
		info := out[s.UserID]
		start := s.StartedAt
		if start.Before(dayStart) {
			start = dayStart
		}
		end := now
		if s.EndedAt != nil {
			end = *s.EndedAt
		} else {
			started := s.StartedAt
			info.StartedAt = &started
		}
		if end.After(start) {
			info.Seconds += int64(end.Sub(start).Seconds())
		}
		out[s.UserID] = info
	}
	return out, nil
}
