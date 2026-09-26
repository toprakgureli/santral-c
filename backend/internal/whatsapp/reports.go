package whatsapp

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// AgentReport is one person's figures over a range.
type AgentReport struct {
	User             PersonView `json:"user"`
	Owned            int64      `json:"owned"`
	Helped           int64      `json:"helped"`
	Resolved         int64      `json:"resolved"`
	Messages         int64      `json:"messages"`
	AvgFirstReplySec int64      `json:"avgFirstReplySec"`
	AvgResolveSec    int64      `json:"avgResolveSec"`
	WaitingEntries   int64      `json:"waitingEntries"`
	Ratings          int64      `json:"ratings"`
	AvgRating        float64    `json:"avgRating"`
}

// ChannelReport is one device's figures over a range.
type ChannelReport struct {
	ID               uint    `json:"id"`
	Name             string  `json:"name"`
	Tickets          int64   `json:"tickets"`
	Resolved         int64   `json:"resolved"`
	BotResolved      int64   `json:"botResolved"`
	Inbound          int64   `json:"inbound"`
	Outbound         int64   `json:"outbound"`
	Failed           int64   `json:"failed"`
	AvgFirstReplySec int64   `json:"avgFirstReplySec"`
	WaitingEntries   int64   `json:"waitingEntries"`
	AvgRating        float64 `json:"avgRating"`
}

// Report is the WhatsApp report page.
type Report struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	Agents   []AgentReport   `json:"agents"`
	Channels []ChannelReport `json:"channels"`
	Hours    [24]int64       `json:"hours"`
}

// Reports computes the figures for an inclusive local day range.
func (s *Service) Reports(ctx context.Context, actorID uint, fromDay, toDay string, channelID uint) (*Report, error) {
	if _, err := s.require(ctx, actorID, enums.WAReports, "WhatsApp raporlarını görme yetkiniz yok."); err != nil {
		return nil, err
	}
	from, err := time.ParseInLocation("2006-01-02", fromDay, istanbul)
	if err != nil {
		return nil, errs.Invalid("Başlangıç tarihi geçersiz.", err)
	}
	toStart, err := time.ParseInLocation("2006-01-02", toDay, istanbul)
	if err != nil || toStart.Before(from) {
		return nil, errs.Invalid("Bitiş tarihi geçersiz.", err)
	}
	to := toStart.AddDate(0, 0, 1)
	chFilter, args := "", []any{}
	if channelID > 0 {
		chFilter = " AND t.channel_id = ?"
		args = append(args, channelID)
	}
	out := &Report{From: fromDay, To: toDay, Agents: []AgentReport{}, Channels: []ChannelReport{}}

	var agents []struct {
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
	if err := s.db.WithContext(ctx).Raw(q, append([]any{from, to}, args...)...).Scan(&agents).Error; err != nil {
		return nil, errs.Internal(err)
	}
	var msgs []struct {
		UserID uint
		N      int64
	}
	mq := "SELECT m.sender_user_id AS user_id, count(*) AS n FROM wa_messages m JOIN wa_tickets t ON t.id = m.ticket_id WHERE m.direction = 'out' AND m.sender_user_id IS NOT NULL AND m.created_at >= ? AND m.created_at < ?" + chFilter + " GROUP BY m.sender_user_id"
	_ = s.db.WithContext(ctx).Raw(mq, append([]any{from, to}, args...)...).Scan(&msgs).Error
	mm := map[uint]int64{}
	var ids []uint
	for _, m := range msgs {
		mm[m.UserID] = m.N
	}
	for _, a := range agents {
		ids = append(ids, a.UserID)
	}
	people := s.people(ctx, ids)
	for _, a := range agents {
		out.Agents = append(out.Agents, AgentReport{User: people[a.UserID], Owned: a.Owned, Helped: a.Helped, Resolved: a.Resolved, Messages: mm[a.UserID],
			AvgFirstReplySec: int64(a.AvgFirst), AvgResolveSec: int64(a.AvgResolve), WaitingEntries: a.WaitingEntries, Ratings: a.Ratings, AvgRating: a.AvgRating})
	}

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
	if err := s.db.WithContext(ctx).Raw(cq+" ORDER BY c.id", cargs...).Scan(&out.Channels).Error; err != nil {
		return nil, errs.Internal(err)
	}
	if out.Channels == nil {
		out.Channels = []ChannelReport{}
	}
	var hours []struct {
		H int
		N int64
	}
	hq := "SELECT EXTRACT(HOUR FROM m.created_at AT TIME ZONE 'Europe/Istanbul')::int AS h, count(*) AS n FROM wa_messages m WHERE m.direction = 'in' AND m.created_at >= ? AND m.created_at < ?"
	hargs := []any{from, to}
	if channelID > 0 {
		hq += " AND m.channel_id = ?"
		hargs = append(hargs, channelID)
	}
	_ = s.db.WithContext(ctx).Raw(hq+" GROUP BY h", hargs...).Scan(&hours).Error
	for _, h := range hours {
		if h.H >= 0 && h.H < 24 {
			out.Hours[h.H] = h.N
		}
	}
	return out, nil
}
