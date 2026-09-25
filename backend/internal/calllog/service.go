package calllog

import (
	"context"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// todayLimit caps how many of today's calls the panel history lists.
const todayLimit = 200

// shortLongSeconds is the boundary between a short and a long conversation.
const shortLongSeconds = 30

// istanbul is the tenant timezone (UTC+3, no DST); today resets at local 00:00.
var istanbul = time.FixedZone("+03", 3*3600)

// Service is the call-log application service.
type Service struct {
	repo  *Repository
	users IActorResolver
	// OnEnded, when set, hears every call the moment it is final.
	OnEnded func(ctx context.Context, log models.CallLog)
}

// NewService builds a call-log service.
func NewService(repo *Repository, users IActorResolver) *Service {
	return &Service{repo: repo, users: users}
}

// Entry is the panel view of a call log.
type Entry struct {
	UUID            string `json:"uuid"`
	Direction       string `json:"direction"`
	Disposition     string `json:"disposition"`
	FromNumber      string `json:"fromNumber"`
	ToNumber        string `json:"toNumber"`
	StartedAt       string `json:"startedAt"`
	DurationSeconds int    `json:"durationSeconds"`
}

// EntryList is today's call logs plus their breakdown for the user.
type EntryList struct {
	Items          []Entry `json:"items"`
	Short          int64   `json:"short"`
	Long           int64   `json:"long"`
	Unanswered     int64   `json:"unanswered"`
	Inbound        int64   `json:"inbound"`
	Outbound       int64   `json:"outbound"`
	InboundMissed  int64   `json:"inboundMissed"`
	OutboundMissed int64   `json:"outboundMissed"`
	InboundReal    int64   `json:"inboundReal"`
	OutboundReal   int64   `json:"outboundReal"`
	// Real (short+long) and Total (all) are derived on the client.
}

// Record applies one phase (start, answer, end) of a softphone call, keyed by
// the client-generated call id so the phases upsert one row.
func (s *Service) Record(ctx context.Context, actorID uint, req requests.CallLogEvent) error {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.CallOriginate) && !canViewOwn(actor) {
		return errs.Forbidden("Bu işlem için yetkiniz yok.")
	}

	existing, err := s.repo.Get(ctx, req.CallID)
	if err != nil {
		return errs.Internal(err)
	}

	switch req.Phase {
	case "start":
		if existing != nil {
			return nil // idempotent
		}
		id := actorID
		log := &models.CallLog{
			CallID:      req.CallID,
			UserID:      &id,
			Direction:   directionOr(req.Direction, "outbound"),
			PeerNumber:  strings.TrimSpace(req.Peer),
			PeerKey:     phone.Key(req.Peer),
			Disposition: "in_progress",
			StartedAt:   time.Now(),
		}
		if err := s.repo.Create(ctx, log); err != nil {
			return errs.Internal(err)
		}
	case "answer":
		if existing == nil {
			return nil
		}
		now := time.Now()
		if err := s.repo.Update(ctx, existing.ID, map[string]any{"answered_at": now, "disposition": "answered"}); err != nil {
			return errs.Internal(err)
		}
	case "end":
		now := time.Now()
		fields := map[string]any{
			"ended_at":         now,
			"disposition":      dispositionOr(req.Disposition, existing),
			"duration_seconds": req.DurationSeconds,
		}
		if existing == nil {
			// The start was never recorded (e.g. a very short call); create a
			// finalized row so nothing is lost.
			id := actorID
			log := &models.CallLog{
				CallID:          req.CallID,
				UserID:          &id,
				Direction:       directionOr(req.Direction, "outbound"),
				PeerNumber:      strings.TrimSpace(req.Peer),
				PeerKey:         phone.Key(req.Peer),
				Disposition:     dispositionOr(req.Disposition, nil),
				StartedAt:       now.Add(-time.Duration(req.DurationSeconds) * time.Second),
				EndedAt:         &now,
				DurationSeconds: req.DurationSeconds,
			}
			if err := s.repo.Create(ctx, log); err != nil {
				return errs.Internal(err)
			}
			s.ended(ctx, *log)
			return nil
		}
		if err := s.repo.Update(ctx, existing.ID, fields); err != nil {
			return errs.Internal(err)
		}
		final := *existing
		final.EndedAt = &now
		final.Disposition = fields["disposition"].(string)
		final.DurationSeconds = req.DurationSeconds
		s.ended(ctx, final)
	}
	return nil
}

// ended hands a final call to the hook, outliving the request.
func (s *Service) ended(ctx context.Context, log models.CallLog) {
	if s.OnEnded == nil {
		return
	}
	go s.OnEnded(context.WithoutCancel(ctx), log)
}

// lookupDays is how far back the number lookup reaches.
const lookupDays = 90

// LookupItem is one call with a number, with the agent who handled it.
type LookupItem struct {
	UUID            string `json:"uuid"`
	Direction       string `json:"direction"`
	Disposition     string `json:"disposition"`
	AgentID         uint   `json:"agentId"`
	AgentName       string `json:"agentName"`
	StartedAt       string `json:"startedAt"`
	DurationSeconds int    `json:"durationSeconds"`
}

// LookupAgent is one agent's share of the calls with a number.
type LookupAgent struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Calls       int    `json:"calls"`
	Answered    int    `json:"answered"`
	TalkSeconds int    `json:"talkSeconds"`
}

// Lookup is everything the panel knows about a number: who spoke with it,
// how often and for how long, newest first.
type Lookup struct {
	Number      string        `json:"number"`
	Scope       string        `json:"scope"` // all or own
	Days        int           `json:"days"`
	Total       int           `json:"total"`
	Answered    int           `json:"answered"`
	TalkSeconds int           `json:"talkSeconds"`
	LastAt      string        `json:"lastAt,omitempty"`
	Agents      []LookupAgent `json:"agents"`
	Items       []LookupItem  `json:"items"`
}

// Lookup answers "who spoke with this number": everyone's calls for agents
// who may see all calls, the actor's own otherwise.
func (s *Service) Lookup(ctx context.Context, actorID uint, number string) (*Lookup, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	var only *uint
	scope := "all"
	switch {
	case canViewAll(actor):
	case canViewOwn(actor):
		only, scope = &actorID, "own"
	default:
		return nil, errs.Forbidden("Çağrı geçmişini görme yetkiniz yok.")
	}
	key := phone.Key(number)
	if len(key) < 3 {
		return nil, errs.Invalid("Aramak için en az üç rakam girin.", nil)
	}
	logs, err := s.repo.ByPeer(ctx, key, only, time.Now().AddDate(0, 0, -lookupDays), 60)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ids := make([]uint, 0, len(logs))
	seen := map[uint]bool{}
	for _, l := range logs {
		if l.UserID != nil && !seen[*l.UserID] {
			seen[*l.UserID] = true
			ids = append(ids, *l.UserID)
		}
	}
	names, err := s.repo.Names(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := &Lookup{Number: strings.TrimSpace(number), Scope: scope, Days: lookupDays, Agents: []LookupAgent{}, Items: []LookupItem{}}
	byAgent := map[uint]*LookupAgent{}
	for _, l := range logs {
		uid := *l.UserID
		item := LookupItem{UUID: l.CallID, Direction: l.Direction, Disposition: l.Disposition, AgentID: uid, AgentName: names[uid], StartedAt: l.StartedAt.In(istanbul).Format(time.RFC3339), DurationSeconds: l.DurationSeconds}
		out.Items = append(out.Items, item)
		out.Total++
		if out.LastAt == "" {
			out.LastAt = item.StartedAt
		}
		a := byAgent[uid]
		if a == nil {
			a = &LookupAgent{ID: uid, Name: names[uid]}
			byAgent[uid] = a
			out.Agents = append(out.Agents, LookupAgent{})
		}
		a.Calls++
		if l.Disposition == "answered" {
			out.Answered++
			out.TalkSeconds += l.DurationSeconds
			a.Answered++
			a.TalkSeconds += l.DurationSeconds
		}
	}
	out.Agents = out.Agents[:0]
	for _, id := range ids {
		out.Agents = append(out.Agents, *byAgent[id])
	}
	return out, nil
}

// Recent returns the actor's own call history for today (since local midnight)
// with a short/long/unanswered breakdown. The panel history is always personal:
// every agent sees only their own calls and it resets at 00:00 local.
func (s *Service) Recent(ctx context.Context, actorID uint) (*EntryList, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !canViewAll(actor) && !canViewOwn(actor) {
		return nil, errs.Forbidden("Çağrı kayıtlarını görme yetkiniz yok.")
	}
	now := time.Now().In(istanbul)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, istanbul)
	logs, counts, err := s.repo.Today(ctx, actorID, from, shortLongSeconds, todayLimit)
	if err != nil {
		return nil, errs.Internal(err)
	}
	ids := make([]uint, 0, len(logs))
	for i := range logs {
		ids = append(ids, logs[i].ID)
	}
	elsewhere, err := s.repo.AnsweredElsewhere(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	repeats, err := s.repo.RepeatRings(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	items := make([]Entry, 0, len(logs))
	for i := range logs {
		e := toEntry(actor, &logs[i])
		switch {
		case elsewhere[logs[i].ID]:
			e.Disposition = "elsewhere" // rang here, another agent answered
		case repeats[logs[i].ID]:
			e.Disposition = "repeat" // the queue offered the same call again
		}
		items = append(items, e)
	}
	return &EntryList{Items: items, Short: counts.Short, Long: counts.Long, Unanswered: counts.Unanswered, Inbound: counts.Inbound, Outbound: counts.Outbound, InboundMissed: counts.InboundMissed, OutboundMissed: counts.OutboundMissed, InboundReal: counts.InboundReal, OutboundReal: counts.OutboundReal}, nil
}

func toEntry(actor *models.User, log *models.CallLog) Entry {
	self := ""
	if actor.SIPExtension != nil {
		self = *actor.SIPExtension
	}
	from, to := self, log.PeerNumber
	if log.Direction == "inbound" {
		from, to = log.PeerNumber, self
	}
	return Entry{
		UUID:            log.CallID,
		Direction:       log.Direction,
		Disposition:     log.Disposition,
		FromNumber:      from,
		ToNumber:        to,
		StartedAt:       log.StartedAt.Format(time.RFC3339),
		DurationSeconds: log.DurationSeconds,
	}
}

func directionOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func dispositionOr(v string, existing *models.CallLog) string {
	if v != "" {
		return v
	}
	if existing != nil && existing.Disposition != "in_progress" {
		return existing.Disposition
	}
	return "no_answer"
}

func canViewOwn(u *models.User) bool {
	return u.Can(enums.CDRViewOwn) || u.Can(enums.CallViewOwn) || u.Can(enums.CallOriginate)
}

func canViewAll(u *models.User) bool {
	return u.Can(enums.CDRViewAll) || u.Can(enums.CallViewAll)
}
