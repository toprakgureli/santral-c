package store

import (
	"context"
	"time"
)

// The report functions count the time range from inclusive to exclusive.
// A channelID of zero means every device.

// AgentFigures is one person's tickets over a time range.
type AgentFigures struct {
	UserID         uint
	Owned          int64
	Helped         int64
	Resolved       int64
	AvgFirst       float64
	AvgResolve     float64
	WaitingEntries int64
	Ratings        int64
	AvgRating      float64
}

// AgentFigures sums the tickets opened in a time range by the people who
// took part in them.
func (r *Repository) AgentFigures(ctx context.Context, from, to time.Time, channelID uint) ([]AgentFigures, error) {
	chFilter, args := channelFilter(channelID)
	var agents []AgentFigures
	q := `SELECT p.user_id,
		count(*) FILTER (WHERE p.role = 'owner') AS owned,
		count(*) FILTER (WHERE p.role = 'helper') AS helped,
		count(*) FILTER (WHERE t.resolved_by = p.user_id) AS resolved,
		COALESCE(avg(EXTRACT(EPOCH FROM (p.first_reply_at - t.created_at))) FILTER (WHERE p.role = 'owner' AND p.first_reply_at IS NOT NULL), 0) AS avg_first,
		COALESCE(avg(EXTRACT(EPOCH FROM (t.resolved_at - t.created_at))) FILTER (WHERE t.resolved_by = p.user_id), 0) AS avg_resolve,
		COALESCE(sum(t.waiting_count) FILTER (WHERE p.role = 'owner'), 0) AS waiting_entries,
		count(t.rating) AS ratings,
		COALESCE(avg(t.rating), 0) AS avg_rating
		FROM wa_ticket_participants p JOIN wa_tickets t ON t.id = p.ticket_id
		WHERE t.created_at >= ? AND t.created_at < ?` + chFilter + ` GROUP BY p.user_id`
	if err := r.db.WithContext(ctx).Raw(q, append([]any{from, to}, args...)...).Scan(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// AgentMessages is how many messages one person sent.
type AgentMessages struct {
	UserID uint
	N      int64
}

// MessagesByAgent counts the messages each person sent in a time range.
func (r *Repository) MessagesByAgent(ctx context.Context, from, to time.Time, channelID uint) ([]AgentMessages, error) {
	chFilter, args := channelFilter(channelID)
	var msgs []AgentMessages
	mq := "SELECT m.sender_user_id AS user_id, count(*) AS n FROM wa_messages m JOIN wa_tickets t ON t.id = m.ticket_id WHERE m.direction = 'out' AND m.sender_user_id IS NOT NULL AND m.created_at >= ? AND m.created_at < ?" + chFilter + " GROUP BY m.sender_user_id"
	if err := r.db.WithContext(ctx).Raw(mq, append([]any{from, to}, args...)...).Scan(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

// channelFilter limits a ticket report to one device when channelID is set.
func channelFilter(channelID uint) (string, []any) {
	chFilter, args := "", []any{}
	if channelID > 0 {
		chFilter = " AND t.channel_id = ?"
		args = append(args, channelID)
	}
	return chFilter, args
}

// ChannelFigures is one device's figures over a time range.
type ChannelFigures struct {
	ID               uint
	Name             string
	Tickets          int64
	Resolved         int64
	BotResolved      int64
	Inbound          int64
	Outbound         int64
	Failed           int64
	AvgFirstReplySec int64
	WaitingEntries   int64
	AvgRating        float64
}

// ChannelFigures sums each device's tickets and messages over a time range,
// in the order the devices were added.
func (r *Repository) ChannelFigures(ctx context.Context, from, to time.Time, channelID uint) ([]ChannelFigures, error) {
	cq := `SELECT c.id, c.name,
		(SELECT count(*) FROM wa_tickets t WHERE t.channel_id = c.id AND t.created_at >= ? AND t.created_at < ?) AS tickets,
		(SELECT count(*) FROM wa_tickets t WHERE t.channel_id = c.id AND t.resolved_at >= ? AND t.resolved_at < ?) AS resolved,
		(SELECT count(*) FROM wa_tickets t WHERE t.channel_id = c.id AND t.resolved_at >= ? AND t.resolved_at < ? AND t.resolved_by IS NULL) AS bot_resolved,
		(SELECT count(*) FROM wa_messages m WHERE m.channel_id = c.id AND m.direction = 'in' AND m.created_at >= ? AND m.created_at < ?) AS inbound,
		(SELECT count(*) FROM wa_messages m WHERE m.channel_id = c.id AND m.direction = 'out' AND m.created_at >= ? AND m.created_at < ?) AS outbound,
		(SELECT count(*) FROM wa_messages m WHERE m.channel_id = c.id AND m.status = 'failed' AND m.created_at >= ? AND m.created_at < ?) AS failed,
		(SELECT COALESCE(avg(EXTRACT(EPOCH FROM (t.first_response_at - t.created_at))), 0)::bigint FROM wa_tickets t WHERE t.channel_id = c.id AND t.first_response_at IS NOT NULL AND t.created_at >= ? AND t.created_at < ?) AS avg_first_reply_sec,
		(SELECT COALESCE(sum(t.waiting_count), 0) FROM wa_tickets t WHERE t.channel_id = c.id AND t.created_at >= ? AND t.created_at < ?) AS waiting_entries,
		(SELECT COALESCE(avg(t.rating), 0) FROM wa_tickets t WHERE t.channel_id = c.id AND t.rated_at >= ? AND t.rated_at < ?) AS avg_rating
		FROM wa_channels c`
	cargs := []any{}
	for i := 0; i < 9; i++ {
		cargs = append(cargs, from, to)
	}
	if channelID > 0 {
		cq += " WHERE c.id = ?"
		cargs = append(cargs, channelID)
	}
	var out []ChannelFigures
	if err := r.db.WithContext(ctx).Raw(cq+" ORDER BY c.id", cargs...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// HourCount is how many messages came in one hour of the day.
type HourCount struct {
	H int
	N int64
}

// InboundByHour counts the customers' messages in a time range by the
// hour of the day they came in, in Istanbul time.
func (r *Repository) InboundByHour(ctx context.Context, from, to time.Time, channelID uint) ([]HourCount, error) {
	var hours []HourCount
	hq := "SELECT EXTRACT(HOUR FROM m.created_at AT TIME ZONE 'Europe/Istanbul')::int AS h, count(*) AS n FROM wa_messages m WHERE m.direction = 'in' AND m.created_at >= ? AND m.created_at < ?"
	hargs := []any{from, to}
	if channelID > 0 {
		hq += " AND m.channel_id = ?"
		hargs = append(hargs, channelID)
	}
	if err := r.db.WithContext(ctx).Raw(hq+" GROUP BY h", hargs...).Scan(&hours).Error; err != nil {
		return nil, err
	}
	return hours, nil
}
