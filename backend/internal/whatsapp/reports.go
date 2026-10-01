package whatsapp

import (
	"context"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/hours"
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
	from, err := time.ParseInLocation("2006-01-02", fromDay, hours.Zone)
	if err != nil {
		return nil, errs.Invalid("Başlangıç tarihi geçersiz.", err)
	}
	toStart, err := time.ParseInLocation("2006-01-02", toDay, hours.Zone)
	if err != nil || toStart.Before(from) {
		return nil, errs.Invalid("Bitiş tarihi geçersiz.", err)
	}
	to := toStart.AddDate(0, 0, 1)
	out := &Report{From: fromDay, To: toDay, Agents: []AgentReport{}, Channels: []ChannelReport{}}

	agents, err := s.repo.AgentFigures(ctx, from, to, channelID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	msgs, err := s.repo.MessagesByAgent(ctx, from, to, channelID)
	if err != nil {
		return nil, errs.Internal(err)
	}
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

	channels, err := s.repo.ChannelFigures(ctx, from, to, channelID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out.Channels = make([]ChannelReport, 0, len(channels))
	for _, c := range channels {
		out.Channels = append(out.Channels, ChannelReport(c))
	}
	hours, err := s.repo.InboundByHour(ctx, from, to, channelID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, h := range hours {
		if h.H >= 0 && h.H < 24 {
			out.Hours[h.H] = h.N
		}
	}
	return out, nil
}
