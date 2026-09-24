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
	UserID         uint  `json:"-"`
	Total          int64 `json:"total"`
	Answered       int64 `json:"answered"`
	Short          int64 `json:"short"`
	Long           int64 `json:"long"`
	Unanswered     int64 `json:"unanswered"`
	Inbound        int64 `json:"inbound"`
	Outbound       int64 `json:"outbound"`
	InboundMissed  int64 `json:"inboundMissed"`
	OutboundMissed int64 `json:"outboundMissed"`
	InboundReal    int64 `json:"inboundReal"`
	OutboundReal   int64 `json:"outboundReal"`
	TalkSeconds    int64 `json:"talkSeconds"`
	// Mean length of real conversations (30 s and up) in the window.
	AvgTalkSeconds int64 `json:"avgTalkSeconds"`
	// The longest answered call in the window.
	LongestSeconds int64 `json:"longestSeconds"`
}

// CallCounts aggregates the call log per user for [from, to).
func (r *Repository) CallCounts(ctx context.Context, from, to time.Time, shortLong int) (map[uint]Counts, error) {
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
			"count(*) FILTER (WHERE direction = 'inbound' AND disposition NOT IN ('answered', 'in_progress')) AS inbound_missed, "+
			"count(*) FILTER (WHERE direction = 'outbound' AND disposition NOT IN ('answered', 'in_progress')) AS outbound_missed, "+
			"count(*) FILTER (WHERE direction = 'inbound' AND disposition = 'answered' AND duration_seconds >= ?) AS inbound_real, "+
			"count(*) FILTER (WHERE direction = 'outbound' AND disposition = 'answered' AND duration_seconds >= ?) AS outbound_real, "+
			"COALESCE(SUM(duration_seconds) FILTER (WHERE disposition = 'answered'), 0) AS talk_seconds, "+
			"COALESCE(AVG(duration_seconds) FILTER (WHERE disposition = 'answered' AND duration_seconds >= ?), 0)::bigint AS avg_talk_seconds, "+
			"COALESCE(MAX(duration_seconds) FILTER (WHERE disposition = 'answered'), 0) AS longest_seconds",
			shortLong, shortLong, shortLong, shortLong, shortLong).
		Where("user_id IS NOT NULL AND started_at >= ? AND started_at < ?", from, to).
		// A ring that a teammate answered is not this agent's call.
		Where("NOT (" + calllog.NotMineSQL + ")").
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

// Escalations counts the escalations each agent recorded in [from, to).
func (r *Repository) Escalations(ctx context.Context, from, to time.Time) (map[uint]int64, error) {
	var rows []struct {
		AgentID uint
		N       int64
	}
	if err := r.db.WithContext(ctx).Model(&models.CallEscalation{}).
		Select("agent_id, count(*) AS n").
		Where("agent_id IS NOT NULL AND created_at >= ? AND created_at < ?", from, to).
		Group("agent_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("escalation counts could not be computed: %w", err)
	}
	out := make(map[uint]int64, len(rows))
	for _, row := range rows {
		out[row.AgentID] = row.N
	}
	return out, nil
}

// BreakSeconds sums the time each agent spent on break inside [from, to),
// counting an open break up to now.
func (r *Repository) BreakSeconds(ctx context.Context, from, to time.Time) (map[uint]int64, error) {
	var rows []struct {
		UserID  uint
		Seconds int64
	}
	if err := r.db.WithContext(ctx).Model(&models.PresenceEvent{}).
		Select("user_id, COALESCE(SUM(EXTRACT(EPOCH FROM (LEAST(COALESCE(ended_at, now()), ?) - GREATEST(started_at, ?)))), 0)::bigint AS seconds", to, from).
		Where("state = 'break' AND started_at < ? AND COALESCE(ended_at, now()) > ?", to, from).
		Group("user_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("break time could not be computed: %w", err)
	}
	out := make(map[uint]int64, len(rows))
	for _, row := range rows {
		out[row.UserID] = row.Seconds
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

// ShiftInfo is an agent's shift picture: when the open shift began (nil when
// off shift), the first shift start and the last shift end inside the
// requested window (LastEnd nil while the latest one is still open), and the
// seconds worked inside the window.
type ShiftInfo struct {
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FirstStart *time.Time `json:"firstStart,omitempty"`
	LastEnd    *time.Time `json:"lastEnd,omitempty"`
	Open       bool       `json:"open"`
	Seconds    int64      `json:"seconds"`
}

// Shifts sums shift time per user inside [from, to) and reports the open
// shift, if any.
func (r *Repository) Shifts(ctx context.Context, from, to time.Time) (map[uint]ShiftInfo, error) {
	var shifts []models.Shift
	err := r.db.WithContext(ctx).
		Where("started_at < ? AND (ended_at IS NULL OR ended_at >= ?)", to, from).
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
		if start.Before(from) {
			start = from
		}
		if info.FirstStart == nil {
			first := s.StartedAt
			info.FirstStart = &first
		}
		end := now
		if s.EndedAt != nil {
			end = *s.EndedAt
			if !info.Open && (info.LastEnd == nil || end.After(*info.LastEnd)) {
				ended := end
				info.LastEnd = &ended
			}
		} else {
			started := s.StartedAt
			info.StartedAt = &started
			info.Open = true
			info.LastEnd = nil
		}
		if end.After(to) {
			end = to
		}
		if end.After(start) {
			info.Seconds += int64(end.Sub(start).Seconds())
		}
		out[s.UserID] = info
	}
	return out, nil
}
