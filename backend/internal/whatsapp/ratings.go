package whatsapp

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
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

// singleQuestion is how a one-question survey (the list inside WhatsApp,
// the buttons after a call) is named among the form's questions.
const singleQuestion = "Tek soruluk anket"

// ratingsSQL is every score in one list. Chat scores belong to whoever
// closed the conversation, or else to its owner. Each row carries its
// answers per question; a one-question survey counts as "Tek soruluk anket".
// The slots are: what to select, what to join (the answers), and the tail.
const ratingsSQL = `
WITH r AS (
	SELECT 'chat' AS source, t.rated_at AS at, t.rating AS score, t.rating_comment AS comment,
		t.conversation_id, t.number AS ticket_number, t.channel_id, COALESCE(t.resolved_by, t.owner_id) AS agent_id,
		c.wa_id, COALESCE(NULLIF(c.name, ''), NULLIF(c.profile_name, ''), '') AS name, 0 AS talk_seconds,
		CASE WHEN jsonb_array_length(t.rating_answers) > 0 THEN t.rating_answers
			ELSE jsonb_build_array(jsonb_build_object('question', '` + singleQuestion + `', 'score', t.rating)) END AS answers
	FROM wa_tickets t JOIN wa_contacts c ON c.id = t.contact_id
	WHERE t.rating IS NOT NULL AND t.rated_at >= @from AND t.rated_at < @to
	UNION ALL
	SELECT 'call', s.answered_at, s.score, s.comment,
		s.conversation_id, NULL, s.channel_id, s.user_id,
		s.wa_id, COALESCE((SELECT COALESCE(NULLIF(c.name, ''), NULLIF(c.profile_name, ''), '') FROM wa_contacts c WHERE c.wa_id = s.wa_id LIMIT 1), ''), s.talk_seconds,
		CASE WHEN jsonb_array_length(s.answers) > 0 THEN s.answers
			ELSE jsonb_build_array(jsonb_build_object('question', '` + singleQuestion + `', 'score', s.score)) END
	FROM wa_call_surveys s
	WHERE s.status = 'answered' AND s.score IS NOT NULL AND s.answered_at >= @from AND s.answered_at < @to
)
SELECT %s FROM r %s WHERE
	(@channel = 0 OR r.channel_id = @channel)
	AND (@agent = 0 OR r.agent_id = @agent)
	AND (@source = '' OR r.source = @source)
	AND (@lo = 0 OR r.score BETWEEN @lo AND @hi)
	AND (NOT @comment OR r.comment <> '')
	AND (@q = '' OR r.name ILIKE @like OR r.wa_id LIKE @digits OR r.comment ILIKE @like)
%s`

func (f RatingFilter) args() (map[string]any, error) {
	from, err := time.ParseInLocation("2006-01-02", f.From, istanbul)
	if err != nil {
		return nil, errs.Invalid("Başlangıç tarihi geçersiz.", err)
	}
	toStart, err := time.ParseInLocation("2006-01-02", f.To, istanbul)
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

type ratingRow struct {
	Source         string
	At             time.Time
	Score          int
	Comment        string
	ConversationID *uint
	TicketNumber   *int64
	ChannelID      *uint
	AgentID        *uint
	WAID           string
	Name           string
	TalkSeconds    int
	Answers        string
}

func (s *Service) ratingRows(ctx context.Context, args map[string]any, tail string) ([]ratingRow, error) {
	var rows []ratingRow
	q := fmt.Sprintf(ratingsSQL, `r.source, r.at, r.score, r.comment, r.conversation_id, r.ticket_number, r.channel_id, r.agent_id, r.wa_id, r.name, r.talk_seconds, r.answers::text AS answers`, "", tail)
	if err := s.db.WithContext(ctx).Raw(q, args).Scan(&rows).Error; err != nil {
		return nil, errs.Internal(err)
	}
	return rows, nil
}

func (s *Service) ratingItems(ctx context.Context, rows []ratingRow) []RatingItem {
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
		if r.AgentID != nil {
			if p, ok := people[*r.AgentID]; ok {
				it.Agent = &p
			}
		}
		if r.ChannelID != nil {
			n, ok := names[*r.ChannelID]
			if !ok {
				if ch, err := s.channel(ctx, *r.ChannelID); err == nil {
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
	var tot struct {
		Count, WithComment, S1, S2, S3, S4, S5 int64
		Average                                float64
	}
	if err := s.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `count(*) AS count, COALESCE(avg(r.score), 0) AS average,
		count(*) FILTER (WHERE r.comment <> '') AS with_comment,
		count(*) FILTER (WHERE r.score = 1) AS s1, count(*) FILTER (WHERE r.score = 2) AS s2, count(*) FILTER (WHERE r.score = 3) AS s3,
		count(*) FILTER (WHERE r.score = 4) AS s4, count(*) FILTER (WHERE r.score = 5) AS s5`, "", ""), args).Scan(&tot).Error; err != nil {
		return nil, errs.Internal(err)
	}
	out.Count, out.Total, out.Average, out.WithComment = tot.Count, tot.Count, tot.Average, tot.WithComment
	out.Dist = [5]int64{tot.S1, tot.S2, tot.S3, tot.S4, tot.S5}

	var agents []struct {
		AgentID uint
		Count   int64
		Average float64
		Low     int64
	}
	_ = s.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `r.agent_id, count(*) AS count, avg(r.score) AS average, count(*) FILTER (WHERE r.score <= 2) AS low`,
		"", ` AND r.agent_id IS NOT NULL GROUP BY r.agent_id ORDER BY count(*) DESC`), args).Scan(&agents).Error

	// per question, and per person and question
	const answerJoin = "CROSS JOIN LATERAL jsonb_array_elements(r.answers) a"
	var qs []struct {
		Question           string
		Count              int64
		Average            float64
		S1, S2, S3, S4, S5 int64
	}
	_ = s.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `a->>'question' AS question, count(*) AS count, avg((a->>'score')::int) AS average,
		count(*) FILTER (WHERE (a->>'score')::int = 1) AS s1, count(*) FILTER (WHERE (a->>'score')::int = 2) AS s2,
		count(*) FILTER (WHERE (a->>'score')::int = 3) AS s3, count(*) FILTER (WHERE (a->>'score')::int = 4) AS s4,
		count(*) FILTER (WHERE (a->>'score')::int = 5) AS s5`, answerJoin, ` GROUP BY 1 ORDER BY count(*) DESC, 1`), args).Scan(&qs).Error
	out.Questions = make([]RatingQuestion, 0, len(qs))
	for _, q := range qs {
		out.Questions = append(out.Questions, RatingQuestion{Question: q.Question, Count: q.Count, Average: q.Average, Dist: [5]int64{q.S1, q.S2, q.S3, q.S4, q.S5}})
	}
	var aq []struct {
		AgentID  uint
		Question string
		Count    int64
		Average  float64
	}
	_ = s.db.WithContext(ctx).Raw(fmt.Sprintf(ratingsSQL, `r.agent_id, a->>'question' AS question, count(*) AS count, avg((a->>'score')::int) AS average`,
		answerJoin, ` AND r.agent_id IS NOT NULL GROUP BY 1, 2`), args).Scan(&aq).Error
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
	rows, err := s.ratingRows(ctx, args, fmt.Sprintf(" ORDER BY r.at DESC LIMIT %d OFFSET %d", ratingPage, (page-1)*ratingPage))
	if err != nil {
		return nil, err
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
	rows, err := s.ratingRows(ctx, args, " ORDER BY r.at DESC LIMIT 50000")
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // so spreadsheet programs read Turkish letters right
	w := csv.NewWriter(&buf)
	w.Comma = ';'
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
	_ = w.Write(append(head, "Yorum", "Müşteri", "Numara", "Sohbet no", "Cihaz", "Temsilci"))
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
		row := []string{it.At.In(istanbul).Format("02.01.2006 15:04"), src, fmt.Sprint(it.Score)}
		for _, q := range questions {
			cell := ""
			for _, a := range it.Answers {
				if a.Question == q {
					cell = fmt.Sprint(a.Score)
				}
			}
			row = append(row, cell)
		}
		_ = w.Write(append(row, it.Comment, it.Customer, "+"+it.Phone, num, it.Channel, agent))
	}
	w.Flush()
	return buf.Bytes(), fmt.Sprintf("puanlamalar_%s_%s.csv", f.From, f.To), nil
}
