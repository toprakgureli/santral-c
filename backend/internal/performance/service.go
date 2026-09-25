// Package performance serves the team page: every agent's live status and
// today's call figures, scoped by the viewer's permission.
package performance

import (
	"context"
	"sort"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// shortLongSeconds mirrors the call-log boundary between a short and a long
// conversation.
const shortLongSeconds = 30

// istanbul is the tenant's timezone (UTC+3, no DST).
var istanbul = time.FixedZone("+03", 3*3600)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// ILiveStatus reports the hosted PBX's live extension statuses (AVAILABLE,
// TALKING, UNREGISTERED and the presence overlays), keyed by extension.
type ILiveStatus interface {
	ExtensionStatuses(ctx context.Context) map[string]string
}

// IContactNames resolves a phone number to a contact name, or "".
type IContactNames interface {
	NameByNumber(ctx context.Context, e164 string) string
}

// Service builds the team page.
type Service struct {
	repo     *Repository
	users    IActorResolver
	live     ILiveStatus
	contacts IContactNames
}

// NewService builds a performance service. live and contacts are optional.
func NewService(repo *Repository, users IActorResolver, contacts IContactNames) *Service {
	return &Service{repo: repo, users: users, contacts: contacts}
}

// SetLive wires the hosted PBX status source (absent when telephony is off).
func (s *Service) SetLive(l ILiveStatus) { s.live = l }

// CurrentCall is the call an agent is on right now.
type CurrentCall struct {
	Peer      string    `json:"peer"`
	PeerName  string    `json:"peerName,omitempty"`
	Direction string    `json:"direction"`
	StartedAt time.Time `json:"startedAt"`
}

// RecentCall is one of an agent's latest calls, for the card's detail fold.
type RecentCall struct {
	Peer            string    `json:"peer"`
	PeerName        string    `json:"peerName,omitempty"`
	Direction       string    `json:"direction"`
	Disposition     string    `json:"disposition"`
	StartedAt       time.Time `json:"startedAt"`
	DurationSeconds int       `json:"durationSeconds"`
}

// Row is one agent on the team page.
type Row struct {
	UserID    uint         `json:"userId"`
	Name      string       `json:"name"`
	Extension string       `json:"extension"`
	Roles     []string     `json:"roles"`
	Status    string       `json:"status"` // talking, available, break, backoffice, dnd, off, unregistered
	Since     *time.Time   `json:"since,omitempty"`
	Call      *CurrentCall `json:"call,omitempty"`
	Shift     ShiftInfo    `json:"shift"`
	Calls     Counts       `json:"calls"`
	// Escalations the agent recorded and seconds spent on break in the window.
	Escalations  int64 `json:"escalations"`
	BreakSeconds int64 `json:"breakSeconds"`
	// Recent are the latest calls in the window, newest first.
	Recent []RecentCall `json:"recent"`
}

// Team is the page payload.
type Team struct {
	Scope string `json:"scope"` // "all" or "role"
	From  string `json:"from"`  // local YYYY-MM-DD, inclusive
	To    string `json:"to"`    // local YYYY-MM-DD, inclusive
	Items []Row  `json:"items"`
}

// Today returns the agents the actor may see with their live status and
// today's figures. Figures reset at 00:00 Istanbul.
func (s *Service) Today(ctx context.Context, actorID uint) (*Team, error) {
	day := todayLocal()
	return s.Range(ctx, actorID, day, day)
}

// Range is Today over an inclusive local day range (YYYY-MM-DD). Live status
// is always current; the figures cover the requested days.
func (s *Service) Range(ctx context.Context, actorID uint, fromDay, toDay string) (*Team, error) {
	from, err := time.ParseInLocation("2006-01-02", fromDay, istanbul)
	if err != nil {
		return nil, errs.Invalid("Başlangıç tarihi geçersiz.", err)
	}
	toStart, err := time.ParseInLocation("2006-01-02", toDay, istanbul)
	if err != nil {
		return nil, errs.Invalid("Bitiş tarihi geçersiz.", err)
	}
	if toStart.Before(from) {
		return nil, errs.Invalid("Bitiş tarihi başlangıçtan önce olamaz.", nil)
	}
	if toStart.Sub(from) > 366*24*time.Hour {
		return nil, errs.Invalid("Aralık en fazla bir yıl olabilir.", nil)
	}
	to := toStart.AddDate(0, 0, 1)
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	scope := ""
	var roleIDs []uint
	switch {
	case actor.Can(enums.PerformanceViewAll):
		scope = "all"
	case actor.Can(enums.PerformanceViewRole):
		scope = "role"
		for _, r := range actor.Roles {
			roleIDs = append(roleIDs, r.ID)
		}
		if len(roleIDs) == 0 {
			return &Team{Scope: scope, From: fromDay, To: toDay, Items: []Row{}}, nil
		}
	default:
		return nil, errs.Forbidden("Ekip performansını görme yetkiniz yok.")
	}

	agents, err := s.repo.Agents(ctx, roleIDs)
	if err != nil {
		return nil, errs.Internal(err)
	}
	counts, err := s.repo.CallCounts(ctx, from, to, shortLongSeconds)
	if err != nil {
		return nil, errs.Internal(err)
	}
	open, err := s.repo.OpenCalls(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	presence, err := s.repo.Presence(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	shifts, err := s.repo.Shifts(ctx, from, to)
	if err != nil {
		return nil, errs.Internal(err)
	}
	escalations, err := s.repo.Escalations(ctx, from, to)
	if err != nil {
		return nil, errs.Internal(err)
	}
	breaks, err := s.repo.BreakSeconds(ctx, from, to)
	if err != nil {
		return nil, errs.Internal(err)
	}
	lastCalls, err := s.repo.LastCallEnds(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	recent, err := s.repo.RecentCalls(ctx, from, to, 6)
	if err != nil {
		return nil, errs.Internal(err)
	}
	live := map[string]string{}
	if s.live != nil {
		live = s.live.ExtensionStatuses(ctx)
	}

	rows := make([]Row, 0, len(agents))
	for i := range agents {
		u := agents[i]
		ext := ""
		if u.SIPExtension != nil {
			ext = *u.SIPExtension
		}
		row := Row{UserID: u.ID, Name: u.Name, Extension: ext, Roles: roleNames(u), Shift: shifts[u.ID], Calls: counts[u.ID], Escalations: escalations[u.ID], BreakSeconds: breaks[u.ID]}
		_, onCall := open[u.ID]
		row.Status, row.Since = s.status(u.ID, ext, onCall, presence, shifts, live, lastCalls)
		if c, ok := open[u.ID]; ok && row.Status == "talking" {
			row.Call = &CurrentCall{Peer: c.PeerNumber, Direction: c.Direction, StartedAt: c.StartedAt}
			if s.contacts != nil {
				if e164, err := phone.Normalize(c.PeerNumber); err == nil {
					row.Call.PeerName = s.contacts.NameByNumber(ctx, e164)
				}
			}
		}
		row.Recent = []RecentCall{}
		for _, l := range recent[u.ID] {
			rc := RecentCall{Peer: l.PeerNumber, Direction: l.Direction, Disposition: l.Disposition, StartedAt: l.StartedAt, DurationSeconds: l.DurationSeconds}
			if s.contacts != nil {
				if e164, err := phone.Normalize(l.PeerNumber); err == nil {
					rc.PeerName = s.contacts.NameByNumber(ctx, e164)
				}
			}
			row.Recent = append(row.Recent, rc)
		}
		rows = append(rows, row)
	}
	// On-shift agents first, then by valid (30s+) calls, then by name.
	sort.SliceStable(rows, func(i, j int) bool {
		oi, oj := rows[i].Status != "off", rows[j].Status != "off"
		if oi != oj {
			return oi
		}
		if rows[i].Calls.Long != rows[j].Calls.Long {
			return rows[i].Calls.Long > rows[j].Calls.Long
		}
		return rows[i].Name < rows[j].Name
	})
	return &Team{Scope: scope, From: fromDay, To: toDay, Items: rows}, nil
}

// status combines the stored presence, the shift, the panel's own call log and
// the hosted PBX's live view into one label. A call in progress (seen by either
// the PBX or our log) always wins; off shift always reads as off. "Since" is
// how long the label has been true: a state change, the shift start or the
// end of the last call, whichever is latest. So "Boşta · 12 dk" means twelve
// minutes without a call, never the hours since the agent last touched the
// status menu.
func (s *Service) status(userID uint, ext string, onCall bool, presence map[uint]models.AgentPresence, shifts map[uint]ShiftInfo, live map[string]string, lastCalls map[uint]time.Time) (string, *time.Time) {
	if shifts[userID].StartedAt == nil {
		return "off", nil
	}
	if onCall || live[ext] == "TALKING" {
		return "talking", nil
	}
	state := "available"
	var since *time.Time
	if p, ok := presence[userID]; ok {
		state = p.State
		t := p.UpdatedAt
		since = &t
	}
	if state == "off" {
		// A shift is open but the presence row still says off (shift start could
		// not write it); treat as available so the agent is not hidden.
		state = "available"
	}
	// Without a presence row the state has held since the shift began.
	if since == nil {
		since = shifts[userID].StartedAt
	}
	// Nothing holds from before the shift began.
	if start := shifts[userID].StartedAt; start != nil && since.Before(*start) {
		since = start
	}
	// Idle time is counted from the last call, not the last click.
	if state == "available" {
		if last, ok := lastCalls[userID]; ok && last.After(*since) {
			t := last
			since = &t
		}
	}
	if state == "available" && live[ext] == "UNREGISTERED" {
		return "unregistered", since
	}
	return state, since
}

func roleNames(u models.User) []string {
	out := make([]string, 0, len(u.Roles))
	for _, r := range u.Roles {
		if enums.Role(r.Name) == enums.RoleInvisibleAdmin {
			continue
		}
		out = append(out, r.DisplayName)
	}
	return out
}

func todayLocal() string {
	return time.Now().In(istanbul).Format("2006-01-02")
}
