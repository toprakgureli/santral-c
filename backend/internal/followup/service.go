// Package followup keeps track of the customers the team still owes a call.
// A call that did not get through (no answer, busy, cancelled while ringing,
// or only the phone system's announcement) puts the number on its caller's
// list. The row closes by itself the moment anyone has a real conversation
// with that number, a colleague calling it or the customer calling in, and
// whoever could not get through is told who reached them. While a call is
// ringing or going on, the panel shows what the team knows about the number:
// how often it called today, who last spoke with it, and who could not
// reach it.
package followup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
	"github.com/toprakgureli/santral-c/backend/pkg/tz"
)

const (
	// contactSeconds: a connected call shorter than this was the phone
	// system playing an announcement, not a conversation (the same
	// threshold as the escalation card).
	contactSeconds = 8
	// openFor is how long a number stays on the list without a new attempt.
	openFor = 7 * 24 * time.Hour
	// claimFor is how long "I am calling back" holds a row.
	claimFor = 30 * time.Minute
	// lastTalkWithin is how far back the last conversation is looked for.
	lastTalkWithin = 30 * 24 * time.Hour
	// freshCall leaves the current call out of today's count: the phone
	// system's copy may already hold it.
	freshCall = time.Minute
)

// IActors loads the acting user.
type IActors interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// INotices leaves a notice for a person.
type INotices interface {
	Add(ctx context.Context, userID uint, kind, text, link string)
}

// Service is the follow-up module.
type Service struct {
	repo    *Repository
	actors  IActors
	notices INotices
	now     func() time.Time
}

// NewService builds the follow-up service.
func NewService(repo *Repository, actors IActors, notices INotices) *Service {
	return &Service{repo: repo, actors: actors, notices: notices, now: time.Now}
}

// Person is a colleague as the lists name them.
type Person struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// Reached tells who had the conversation that closed a row.
type Reached struct {
	By        Person    `json:"by"`
	Direction string    `json:"direction"`
	Seconds   int       `json:"seconds"`
	At        time.Time `json:"at"`
}

// Claim is a colleague calling the number back right now.
type Claim struct {
	By    Person    `json:"by"`
	Until time.Time `json:"until"`
	Mine  bool      `json:"mine"`
}

// Unreached is one number someone could not reach.
type Unreached struct {
	ID        uint       `json:"id"`
	Number    string     `json:"number"`
	User      Person     `json:"user"`
	Mine      bool       `json:"mine"`
	Attempts  int        `json:"attempts"`
	FirstAt   time.Time  `json:"firstAt"`
	LastAt    time.Time  `json:"lastAt"`
	Reason    string     `json:"reason"`
	Status    string     `json:"status"`
	Reached   *Reached   `json:"reached,omitempty"`
	Claim     *Claim     `json:"claim,omitempty"`
	DroppedBy *Person    `json:"droppedBy,omitempty"`
	DroppedAt *time.Time `json:"droppedAt,omitempty"`
}

// LastTalk is the last real conversation with a number.
type LastTalk struct {
	By        Person    `json:"by"`
	Mine      bool      `json:"mine"`
	At        time.Time `json:"at"`
	Seconds   int       `json:"seconds"`
	Direction string    `json:"direction"`
}

// PeerContext is what the team knows about a number during a call.
type PeerContext struct {
	// InboundBefore counts the calls the number made today before the
	// current one.
	InboundBefore int         `json:"inboundBefore"`
	LastTalk      *LastTalk   `json:"lastTalk,omitempty"`
	Unreached     []Unreached `json:"unreached"`
	// Reminders are the call backs planned to the number.
	Reminders []Reminder `json:"reminders"`
}

// OnCallEnded runs after a call is final: a call out that did not get
// through puts the number on its caller's list; a real conversation, in
// either direction, closes every open row of that number and tells the
// others who reached it.
func (s *Service) OnCallEnded(ctx context.Context, log models.CallLog) {
	key := log.PeerKey
	if log.UserID == nil || log.Direction == "internal" || len(key) <= 5 {
		return
	}
	at := s.now()
	if log.EndedAt != nil {
		at = *log.EndedAt
	}
	talked := log.Disposition == "answered" && (log.DurationUnknown || log.DurationSeconds >= contactSeconds)
	switch {
	case talked:
		who := s.nameOf(ctx, *log.UserID)
		rows, err := s.repo.Reach(ctx, key, *log.UserID, log.Direction, log.DurationSeconds, log.CallID, at)
		if err != nil {
			slog.WarnContext(ctx, "unreached calls could not be closed", "call", log.CallID, "error", err)
		} else {
			s.tellReached(ctx, rows, log, who, at)
		}
		planned, err := s.repo.reachReminders(ctx, key, *log.UserID, at)
		if err != nil {
			slog.WarnContext(ctx, "call backs could not be closed", "call", log.CallID, "error", err)
		} else {
			s.tellRemindersReached(ctx, planned, log, who, at)
		}
	case log.Direction == "outbound":
		if err := s.repo.AddAttempt(ctx, key, strings.TrimSpace(log.PeerNumber), *log.UserID, at, log.CallID, reasonOf(log)); err != nil {
			slog.WarnContext(ctx, "unreached call could not be recorded", "call", log.CallID, "error", err)
		}
	}
}

// reasonOf names why a call out did not get through.
func reasonOf(log models.CallLog) string {
	switch log.Disposition {
	case "answered":
		return "short" // the phone system's announcement
	case "busy", "canceled", "failed":
		return log.Disposition
	}
	return "no_answer"
}

// nameOf names a colleague for a notice.
func (s *Service) nameOf(ctx context.Context, id uint) string {
	if u, err := s.actors.GetByID(ctx, id); err == nil && u != nil {
		return u.Name
	}
	return "Bir çalışma arkadaşın"
}

// tellReached tells everyone who could not reach the number who did.
func (s *Service) tellReached(ctx context.Context, rows []models.CallUnreached, log models.CallLog, who string, at time.Time) {
	for _, r := range rows {
		if r.UserID == *log.UserID {
			continue // they reached the number themselves
		}
		var text string
		if log.Direction == "inbound" {
			text = fmt.Sprintf("Ulaşamadığın %s geri aradı; %s ile %s görüştü (%s).", r.PeerNumber, who, talkLabel(log.DurationSeconds), clock(at))
		} else {
			text = fmt.Sprintf("Ulaşamadığın %s numarasına %s ulaştı, %s görüştü (%s).", r.PeerNumber, who, talkLabel(log.DurationSeconds), clock(at))
		}
		s.notices.Add(ctx, r.UserID, "unreached_reached", text, "/followups")
	}
}

// scope reads who the actor may see: everyone's rows with call.view_all,
// their own with call.view_own or a phone line.
func (s *Service) scope(ctx context.Context, actorID uint, all bool) (*models.User, *uint, error) {
	actor, err := s.actors.GetByID(ctx, actorID)
	if err != nil {
		return nil, nil, err
	}
	seesAll := actor.Can(enums.CallViewAll) || actor.Can(enums.CDRViewAll)
	if all {
		if !seesAll {
			return nil, nil, errs.Forbidden("Ekibin geri dönüş listesini görme yetkin yok.")
		}
		return actor, nil, nil
	}
	if !seesAll && !actor.Can(enums.CallViewOwn) && !actor.Can(enums.CDRViewOwn) && !actor.Can(enums.CallOriginate) {
		return nil, nil, errs.Forbidden("Geri dönüş listesini görme yetkin yok.")
	}
	return actor, &actor.ID, nil
}

// Unreached lists the numbers still owed a call (open) or those closed in
// the last week, the actor's own or, with all, everyone's.
func (s *Service) Unreached(ctx context.Context, actorID uint, all, open bool) ([]Unreached, error) {
	actor, only, err := s.scope(ctx, actorID, all)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, only, open, s.now().Add(-openFor))
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]Unreached, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(r, actor.ID))
	}
	return out, nil
}

func (s *Service) view(r row, actorID uint) Unreached {
	v := Unreached{ID: r.ID, Number: r.PeerNumber, User: Person{r.UserID, r.UserName}, Mine: r.UserID == actorID,
		Attempts: r.Attempts, FirstAt: r.FirstAt, LastAt: r.LastAt, Reason: r.LastReason, Status: r.Status, DroppedAt: r.DroppedAt}
	if r.ReachedBy != nil && r.ReachedAt != nil {
		v.Reached = &Reached{By: Person{*r.ReachedBy, r.ReachedByName}, At: *r.ReachedAt}
		if r.ReachedDirection != nil {
			v.Reached.Direction = *r.ReachedDirection
		}
		if r.ReachedSeconds != nil {
			v.Reached.Seconds = *r.ReachedSeconds
		}
	}
	if r.ClaimedBy != nil && r.ClaimedUntil != nil && r.ClaimedUntil.After(s.now()) {
		v.Claim = &Claim{By: Person{*r.ClaimedBy, r.ClaimedByName}, Until: *r.ClaimedUntil, Mine: *r.ClaimedBy == actorID}
	}
	if r.DroppedBy != nil {
		v.DroppedBy = &Person{*r.DroppedBy, r.DroppedByName}
	}
	return v
}

// mayTouch loads a row the actor may act on: their own, or anyone's with
// call.view_all.
func (s *Service) mayTouch(ctx context.Context, actorID, id uint) (*models.CallUnreached, error) {
	actor, only, err := s.scope(ctx, actorID, false)
	if err != nil {
		return nil, err
	}
	r, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	seesAll := actor.Can(enums.CallViewAll) || actor.Can(enums.CDRViewAll)
	if r == nil || (!seesAll && only != nil && r.UserID != *only) {
		return nil, errs.NotFound("Kayıt bulunamadı.")
	}
	return r, nil
}

// Claim marks a number as being called back by the actor for half an hour,
// so nobody else calls it at the same time.
func (s *Service) Claim(ctx context.Context, actorID, id uint) error {
	if _, err := s.mayTouch(ctx, actorID, id); err != nil {
		return err
	}
	ok, err := s.repo.Claim(ctx, id, actorID, s.now().Add(claimFor))
	if err != nil {
		return errs.Internal(err)
	}
	if !ok {
		return errs.Conflict("Bu numarayı şu an başka biri arıyor ya da kayıt kapandı.", nil)
	}
	return nil
}

// Unclaim lets go of a number the actor said they would call.
func (s *Service) Unclaim(ctx context.Context, actorID, id uint) error {
	if _, err := s.mayTouch(ctx, actorID, id); err != nil {
		return err
	}
	if err := s.repo.Unclaim(ctx, id, actorID); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// Drop takes a number off the list by hand: no call back is needed.
func (s *Service) Drop(ctx context.Context, actorID, id uint) error {
	if _, err := s.mayTouch(ctx, actorID, id); err != nil {
		return err
	}
	ok, err := s.repo.Drop(ctx, id, actorID)
	if err != nil {
		return errs.Internal(err)
	}
	if !ok {
		return errs.NotFound("Kayıt zaten kapanmış.")
	}
	return nil
}

// Peer tells what the team knows about a number while a call with it
// rings or goes on.
func (s *Service) Peer(ctx context.Context, actorID uint, number string) (*PeerContext, error) {
	actor, _, err := s.scope(ctx, actorID, false)
	if err != nil {
		return nil, err
	}
	out := &PeerContext{Unreached: []Unreached{}, Reminders: []Reminder{}}
	key := phone.Key(number)
	if len(key) <= 5 {
		return out, nil // a colleague's extension
	}
	now := s.now()
	local := now.In(tz.Istanbul)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, tz.Istanbul)
	if out.InboundBefore, err = s.repo.InboundBetween(ctx, key, midnight, now.Add(-freshCall)); err != nil {
		return nil, errs.Internal(err)
	}
	last, err := s.repo.LastTalk(ctx, key, now.Add(-lastTalkWithin), contactSeconds)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if last != nil {
		out.LastTalk = &LastTalk{By: Person{last.UserID, last.Name}, Mine: last.UserID == actor.ID, At: last.StartedAt,
			Seconds: last.DurationSeconds, Direction: last.Direction}
	}
	rows, err := s.repo.OpenByPeer(ctx, key, now.Add(-openFor))
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, r := range rows {
		out.Unreached = append(out.Unreached, s.view(r, actor.ID))
	}
	planned, err := s.repo.remindersOfPeer(ctx, key)
	if err != nil {
		return nil, errs.Internal(err)
	}
	for _, r := range planned {
		out.Reminders = append(out.Reminders, reminderView(r, actor.ID))
	}
	return out, nil
}

// talkLabel writes a length as minutes and seconds.
func talkLabel(seconds int) string {
	m, sec := seconds/60, seconds%60
	switch {
	case m == 0:
		return fmt.Sprintf("%d sn", sec)
	case sec == 0:
		return fmt.Sprintf("%d dk", m)
	}
	return fmt.Sprintf("%d dk %d sn", m, sec)
}

// clock writes a time of day in Turkey.
func clock(t time.Time) string {
	return t.In(tz.Istanbul).Format("15:04")
}
