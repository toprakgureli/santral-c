package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/hours"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/store"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/sheet"
)

// Ratings brings together every score customers gave: the survey at the end
// of a WhatsApp conversation (the list inside WhatsApp or the Tally form)
// and the survey after a phone call.

// RatingFilter narrows the list.
type RatingFilter struct {
	From, To  string // days, Turkey time
	ChannelID uint
	AgentID   uint
	Source    string // "", chat, call
	Score     string // "", 1..5, low (1-2), high (4-5)
	Comment   bool   // only those with a comment
	Q         string // customer name, number or words in the comment
	Page      int
}

// RatingItem is one score.
type RatingItem struct {
	Source         string         `json:"source"` // chat | call
	At             time.Time      `json:"at"`
	Score          int            `json:"score"`
	Comment        string         `json:"comment,omitempty"`
	Customer       string         `json:"customer"`
	Phone          string         `json:"phone"`
	ConversationID *uint          `json:"conversationId,omitempty"`
	TicketNumber   *int64         `json:"ticketNumber,omitempty"`
	Channel        string         `json:"channel,omitempty"`
	Agent          *PersonView    `json:"agent,omitempty"`
	TalkSeconds    int            `json:"talkSeconds,omitempty"`
	Answers        []RatingAnswer `json:"answers"` // each question of the form
	Texts          []RatingText   `json:"texts"`   // written answers, each under its question
}

// RatingQuestion is one survey question over the period: its average and
// how its scores spread.
type RatingQuestion struct {
	Question string   `json:"question"`
	Count    int64    `json:"count"`
	Average  float64  `json:"average"`
	Dist     [5]int64 `json:"dist"`
}

// RatingAgentQuestion is one person's average on one question.
type RatingAgentQuestion struct {
	Question string  `json:"question"`
	Count    int64   `json:"count"`
	Average  float64 `json:"average"`
}

// RatingAgent is one person's scores in the period.
type RatingAgent struct {
	Agent     PersonView            `json:"agent"`
	Count     int64                 `json:"count"`
	Average   float64               `json:"average"`
	Low       int64                 `json:"low"`
	Questions []RatingAgentQuestion `json:"questions"`
}

// RatingsView is the page: totals, per person, and the scores themselves.
type RatingsView struct {
	Count       int64         `json:"count"`
	Average     float64       `json:"average"`
	Dist        [5]int64      `json:"dist"` // how many 1s, 2s ... 5s
	WithComment int64         `json:"withComment"`
	Agents      []RatingAgent `json:"agents"`
	// per question, the most answered first
	Questions []RatingQuestion `json:"questions"`
	Items     []RatingItem     `json:"items"`
	Total     int64            `json:"total"` // matching the filter, for paging
	PageSize  int              `json:"pageSize"`
}

const ratingPage = 50

func (f RatingFilter) args() (map[string]any, error) {
	from, err := time.ParseInLocation("2006-01-02", f.From, hours.Zone)
	if err != nil {
		return nil, errs.Invalid("Başlangıç tarihi geçersiz.", err)
	}
	toStart, err := time.ParseInLocation("2006-01-02", f.To, hours.Zone)
	if err != nil || toStart.Before(from) {
		return nil, errs.Invalid("Bitiş tarihi geçersiz.", err)
	}
	lo, hi := 0, 0
	switch f.Score {
	case "low":
		lo, hi = 1, 2
	case "high":
		lo, hi = 4, 5
	case "1", "2", "3", "4", "5":
		lo = int(f.Score[0] - '0')
		hi = lo
	}
	source := f.Source
	if source != "chat" && source != "call" {
		source = ""
	}
	q := strings.TrimSpace(f.Q)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, q)
	if len(digits) >= 3 {
		digits = "%" + strings.TrimPrefix(digits, "0") + "%"
	} else {
		digits = "-" // numbers hold only digits, so this matches nothing
	}
	return map[string]any{
		"from": from, "to": toStart.AddDate(0, 0, 1),
		"channel": f.ChannelID, "agent": f.AgentID, "source": source,
		"lo": lo, "hi": hi, "comment": f.Comment,
		"q": q, "like": "%" + q + "%", "digits": digits,
	}, nil
}

func (s *Service) ratingItems(ctx context.Context, rows []store.Rating) []RatingItem {
	var ids []uint
	for _, r := range rows {
		if r.AgentID != nil {
			ids = append(ids, *r.AgentID)
		}
	}
	people := s.people(ctx, ids)
	names := map[uint]string{}
	out := make([]RatingItem, 0, len(rows))
	for _, r := range rows {
		it := RatingItem{Source: r.Source, At: r.At, Score: r.Score, Comment: r.Comment, Customer: r.Name, Phone: r.WAID,
			ConversationID: r.ConversationID, TicketNumber: r.TicketNumber, TalkSeconds: r.TalkSeconds, Answers: []RatingAnswer{}}
		_ = json.Unmarshal([]byte(r.Answers), &it.Answers)
		it.Texts = []RatingText{}
		_ = json.Unmarshal([]byte(r.Texts), &it.Texts)
		if r.AgentID != nil {
			if p, ok := people[*r.AgentID]; ok {
				it.Agent = &p
			}
		}
		if r.ChannelID != nil {
			n, ok := names[*r.ChannelID]
			if !ok {
				if ch, err := s.repo.Channel(ctx, *r.ChannelID); err == nil {
					n = ch.Name
				}
				names[*r.ChannelID] = n
			}
			it.Channel = n
		}
		out = append(out, it)
	}
	return out
}

// Ratings lists the scores of a period with totals and per person.
func (s *Service) Ratings(ctx context.Context, actorID uint, f RatingFilter) (*RatingsView, error) {
	if _, err := s.require(ctx, actorID, enums.WARatings, "Puanlamaları görme yetkiniz yok."); err != nil {
		return nil, err
	}
	args, err := f.args()
	if err != nil {
		return nil, err
	}
	out := &RatingsView{Agents: []RatingAgent{}, Items: []RatingItem{}, PageSize: ratingPage}
	tot, err := s.repo.RatingTotals(ctx, args)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out.Count, out.Total, out.Average, out.WithComment = tot.Count, tot.Count, tot.Average, tot.WithComment
	out.Dist = [5]int64{tot.S1, tot.S2, tot.S3, tot.S4, tot.S5}

	agents, err := s.repo.RatingAgents(ctx, args)
	if err != nil {
		return nil, errs.Internal(err)
	}

	// per question, and per person and question
	qs, err := s.repo.RatingQuestions(ctx, args)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out.Questions = make([]RatingQuestion, 0, len(qs))
	for _, q := range qs {
		out.Questions = append(out.Questions, RatingQuestion{Question: q.Question, Count: q.Count, Average: q.Average, Dist: [5]int64{q.S1, q.S2, q.S3, q.S4, q.S5}})
	}
	aq, err := s.repo.RatingAgentQuestions(ctx, args)
	if err != nil {
		return nil, errs.Internal(err)
	}
	byAgent := map[uint][]RatingAgentQuestion{}
	for _, x := range aq {
		byAgent[x.AgentID] = append(byAgent[x.AgentID], RatingAgentQuestion{Question: x.Question, Count: x.Count, Average: x.Average})
	}
	var ids []uint
	for _, a := range agents {
		ids = append(ids, a.AgentID)
	}
	people := s.people(ctx, ids)
	for _, a := range agents {
		if p, ok := people[a.AgentID]; ok {
			qs := byAgent[a.AgentID]
			if qs == nil {
				qs = []RatingAgentQuestion{}
			}
			out.Agents = append(out.Agents, RatingAgent{Agent: p, Count: a.Count, Average: a.Average, Low: a.Low, Questions: qs})
		}
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	rows, err := s.repo.Ratings(ctx, args, ratingPage, (page-1)*ratingPage)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out.Items = s.ratingItems(ctx, rows)
	return out, nil
}

// RatingsCSV writes the filtered scores as a spreadsheet file.
func (s *Service) RatingsCSV(ctx context.Context, actorID uint, f RatingFilter) ([]byte, string, error) {
	if _, err := s.require(ctx, actorID, enums.WARatings, "Puanlamaları görme yetkiniz yok."); err != nil {
		return nil, "", err
	}
	args, err := f.args()
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.LatestRatings(ctx, args, 50000)
	if err != nil {
		return nil, "", errs.Internal(err)
	}
	w := sheet.NewWriter()
	items := s.ratingItems(ctx, rows)
	// one column per question, in the order they first appear
	var questions []string
	seen := map[string]bool{}
	for _, it := range items {
		for _, a := range it.Answers {
			if !seen[a.Question] {
				seen[a.Question] = true
				questions = append(questions, a.Question)
			}
		}
	}
	head := []string{"Tarih", "Kaynak", "Puan (ortalama)"}
	head = append(head, questions...)
	if err := w.Row(append(head, "Yorum", "Müşteri", "Numara", "Sohbet no", "Cihaz", "Temsilci")...); err != nil {
		return nil, "", errs.Internal(err)
	}
	for _, it := range items {
		src := "WhatsApp sohbeti"
		if it.Source == "call" {
			src = "Telefon görüşmesi"
		}
		num := ""
		if it.TicketNumber != nil {
			num = fmt.Sprint(*it.TicketNumber)
		}
		agent := ""
		if it.Agent != nil {
			agent = it.Agent.Name
		}
		row := []string{it.At.In(hours.Zone).Format("02.01.2006 15:04"), src, fmt.Sprint(it.Score)}
		for _, q := range questions {
			cell := ""
			for _, a := range it.Answers {
				if a.Question == q {
					cell = fmt.Sprint(a.Score)
				}
			}
			row = append(row, cell)
		}
		if err := w.Row(append(row, it.Comment, it.Customer, "+"+it.Phone, num, it.Channel, agent)...); err != nil {
			return nil, "", errs.Internal(err)
		}
	}
	data, err := w.Bytes()
	if err != nil {
		return nil, "", errs.Internal(err)
	}
	return data, fmt.Sprintf("puanlamalar_%s_%s.csv", f.From, f.To), nil
}
