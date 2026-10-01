package calllog

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

// todayLimit caps how many of today's calls the panel history lists.
const todayLimit = 200

// shortLongSeconds is the boundary between a short and a long conversation.
const shortLongSeconds = 30

// Service is the call-log application service.
type Service struct {
	repo  *Repository
	users IActorResolver
	// OnEnded, when set, hears every call once it is final and, with a
	// verifier, confirmed by the phone system.
	OnEnded  func(ctx context.Context, log models.CallLog)
	verifier ICallVerifier
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
// the client-generated call id so the phases upsert one row. Only the agent
// who started a call may answer or end it, an end that arrives twice changes
// nothing, and the length the browser reports is capped by the time that
// really passed.
func (s *Service) Record(ctx context.Context, actorID uint, req requests.CallLogEvent) error {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.CallOriginate) && !canViewOwn(actor) {
		return errs.Forbidden("Bu işlem için yetkin yok.")
	}

	existing, err := s.repo.Get(ctx, req.CallID)
	if err != nil {
		return errs.Internal(err)
	}
	if existing != nil && (existing.UserID == nil || *existing.UserID != actorID) {
		return errs.NotFound("Çağrı kaydı bulunamadı.")
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
		if existing == nil || existing.EndedAt != nil {
			return nil
		}
		now := time.Now()
		if err := s.repo.Update(ctx, existing.ID, map[string]any{"answered_at": now, "disposition": "answered"}); err != nil {
			return errs.Internal(err)
		}
	case "end":
		if existing != nil && existing.EndedAt != nil {
			return nil // already final; a second end changes nothing
		}
		now := time.Now()
		if existing == nil {
			// The start was never recorded (e.g. a very short call); create a
			// finalized row so nothing is lost.
			id := actorID
			seconds := clampSeconds(req.DurationSeconds, maxUnstartedCall)
			log := &models.CallLog{
				CallID:          req.CallID,
				UserID:          &id,
				Direction:       directionOr(req.Direction, "outbound"),
				PeerNumber:      strings.TrimSpace(req.Peer),
				PeerKey:         phone.Key(req.Peer),
				Disposition:     dispositionOr(req.Disposition, nil),
				StartedAt:       now.Add(-time.Duration(seconds) * time.Second),
				EndedAt:         &now,
				DurationSeconds: seconds,
			}
			if err := s.repo.Create(ctx, log); err != nil {
				return errs.Internal(err)
			}
			return s.finish(ctx, *log)
		}
		final := *existing
		final.EndedAt = &now
		final.Disposition = dispositionOr(req.Disposition, existing)
		final.DurationSeconds = clampSeconds(req.DurationSeconds, now.Sub(existing.StartedAt)+5*time.Second)
		if err := s.repo.Update(ctx, existing.ID, map[string]any{
			"ended_at":         now,
			"disposition":      final.Disposition,
			"duration_seconds": final.DurationSeconds,
		}); err != nil {
			return errs.Internal(err)
		}
		return s.finish(ctx, final)
	}
	return nil
}

// maxUnstartedCall caps the length of a call whose start the panel never
// heard about.
const maxUnstartedCall = 4 * time.Hour

// clampSeconds keeps a reported length between zero and limit.
func clampSeconds(seconds int, limit time.Duration) int {
	if seconds < 0 {
		return 0
	}
	if most := int(limit / time.Second); seconds > most {
		return most
	}
	return seconds
}

// ICallVerifier finds a call in the phone system's own records.
type ICallVerifier interface {
	// CallSeen looks for a call between extension and peer that started
	// around at. It reports whether one was found, how long the two sides
	// talked and whether it was answered.
	CallSeen(ctx context.Context, extension, peer string, at time.Time) (found bool, talkSeconds int, answered bool, err error)
}

// SetVerifier makes what follows a call wait for the phone system's record
// of it. Without one (no phone system configured) it runs at once.
func (s *Service) SetVerifier(v ICallVerifier) { s.verifier = v }

// finish runs what follows a final call: at once when there is nothing to
// check against, otherwise after the phone system's record confirms it.
func (s *Service) finish(ctx context.Context, log models.CallLog) error {
	if s.verifier == nil {
		s.ended(ctx, log)
		return nil
	}
	if err := s.repo.Update(ctx, log.ID, map[string]any{"hooks_done": false}); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// verifyAfter is how long a call waits for the phone system's record before
// what follows it is dropped.
const verifyAfter = 2 * time.Hour

// StartVerifier checks waiting calls against the phone system's records every
// half minute until ctx ends.
func (s *Service) StartVerifier(ctx context.Context, g *safe.Group) {
	if s.verifier == nil {
		return
	}
	g.Loop(ctx, "call log verifier", func(ctx context.Context) {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			s.VerifyPending(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
}

// VerifyPending checks the calls waiting for the phone system's record once:
// a confirmed call gets the system's own length and runs what follows it;
// one never seen within verifyAfter is closed without it.
func (s *Service) VerifyPending(ctx context.Context) {
	logs, err := s.repo.HooksPending(ctx, 200)
	if err != nil {
		slog.WarnContext(ctx, "calls waiting for the phone record could not be read", "error", err)
		return
	}
	for _, l := range logs {
		if ctx.Err() != nil {
			return
		}
		ext := ""
		if l.UserID != nil {
			if u, err := s.users.GetByID(ctx, *l.UserID); err == nil && u.SIPExtension != nil {
				ext = *u.SIPExtension
			}
		}
		found := false
		if ext != "" {
			var talk int
			var answered bool
			found, talk, answered, err = s.verifier.CallSeen(ctx, ext, l.PeerNumber, l.StartedAt)
			if err != nil {
				slog.WarnContext(ctx, "phone record could not be checked", "call", l.CallID, "error", err)
				continue
			}
			if found {
				// The phone system's own figures win over the browser's.
				l.DurationSeconds = talk
				if !answered && l.Disposition == "answered" {
					l.Disposition = "no_answer"
				}
			}
		}
		switch {
		case found:
			if err := s.repo.Update(ctx, l.ID, map[string]any{"hooks_done": true, "duration_seconds": l.DurationSeconds, "disposition": l.Disposition}); err != nil {
				slog.WarnContext(ctx, "verified call could not be stored", "call", l.CallID, "error", err)
				continue
			}
			s.ended(ctx, l)
		case l.EndedAt != nil && time.Since(*l.EndedAt) > verifyAfter:
			slog.WarnContext(ctx, "call never appeared in the phone records; nothing follows it", "call", l.CallID)
			if err := s.repo.Update(ctx, l.ID, map[string]any{"hooks_done": true}); err != nil {
				slog.WarnContext(ctx, "unverified call could not be closed", "call", l.CallID, "error", err)
			}
		}
	}
}

// ended hands a final call to the hook, outliving the request.
func (s *Service) ended(ctx context.Context, log models.CallLog) {
	if s.OnEnded == nil {
		return
	}
	bg := context.WithoutCancel(ctx)
	safe.Go(bg, "call ended hooks", func() { s.OnEnded(bg, log) })
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
		return nil, errs.Forbidden("Çağrı geçmişini görme yetkin yok.")
	}
	key := phone.Key(number)
	if len(key) < 3 {
		return nil, errs.Invalid("Aramak için en az üç rakam gir.", nil)
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
		item := LookupItem{UUID: l.CallID, Direction: l.Direction, Disposition: l.Disposition, AgentID: uid, AgentName: names[uid], StartedAt: l.StartedAt.In(tz.Istanbul).Format(time.RFC3339), DurationSeconds: l.DurationSeconds}
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
		return nil, errs.Forbidden("Çağrı kayıtlarını görme yetkin yok.")
	}
	now := time.Now().In(tz.Istanbul)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz.Istanbul)
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
