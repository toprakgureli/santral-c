package verimor

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IShiftReader reports the user's open shift. Placing calls and changing
// presence are only allowed on shift, and the presence clocks run per shift.
type IShiftReader interface {
	// OpenSince returns when the user's current shift started; ok is false
	// when the user is off shift.
	OpenSince(ctx context.Context, userID uint) (start time.Time, ok bool, err error)
}

// Service exposes the hosted PBX to the panel.
type Service struct {
	client *Client
	users  IActorResolver
	repo   *Repository
	cfg    configs.Bulutsantralim
	shifts IShiftReader

	breakLimit IBreakLimit
	contacts   IContactNames

	// snap holds the last good snapshot the background poller maintains, so the
	// panel's hot paths never touch the rate-limited API directly.
	snapMu sync.RWMutex
	snap   snapshot

	// hub fans real-time agent-list updates out to SSE subscribers.
	hubMu sync.Mutex
	subs  map[chan []byte]struct{}
}

// snapshot is the last-good view the poller keeps warm.
type snapshot struct {
	exts   []PBXExtension
	queues []PBXQueue
	stats  *Stats
}

// NewService builds a Verimor service.
func NewService(client *Client, users IActorResolver, repo *Repository, cfg configs.Bulutsantralim) *Service {
	if cfg.WebphoneBase == "" {
		cfg.WebphoneBase = "https://oim.verimor.com.tr/webphone"
	}
	return &Service{client: client, users: users, repo: repo, cfg: cfg}
}

// SetShifts wires the shift reader that gates outbound calls and presence.
func (s *Service) SetShifts(r IShiftReader) { s.shifts = r }

// IBreakLimit reads the daily break allowance.
type IBreakLimit interface {
	BreakLimitMinutes(ctx context.Context) int
}

// SetBreakLimit wires the settings reader for the daily break allowance.
func (s *Service) SetBreakLimit(r IBreakLimit) { s.breakLimit = r }

// IContactNames resolves a phone number to a contact name, or "".
type IContactNames interface {
	NameByNumber(ctx context.Context, e164 string) string
}

// SetContacts wires the contact lookup used to name a talking agent's peer.
func (s *Service) SetContacts(r IContactNames) { s.contacts = r }

// onShift reports whether the user may place calls and change presence. With
// no shift reader wired, everything is allowed.
func (s *Service) onShift(ctx context.Context, userID uint) bool {
	_, ok := s.shiftStart(ctx, userID)
	return ok
}

// shiftStart returns when the user's current shift began. With no shift reader
// wired, the day is the window (the pre-shift behaviour).
func (s *Service) shiftStart(ctx context.Context, userID uint) (time.Time, bool) {
	if s.shifts == nil {
		return todayStart(), true
	}
	start, ok, err := s.shifts.OpenSince(ctx, userID)
	if err != nil {
		slog.WarnContext(ctx, "shift lookup failed", "user", userID, "error", err)
		return time.Time{}, false
	}
	return start, ok
}

// ShiftStarted puts the agent back on the floor: presence becomes available
// and do-not-disturb is lifted on the hosted PBX. Users without an extension
// only get the presence row.
func (s *Service) ShiftStarted(ctx context.Context, userID uint) {
	if err := s.repo.SetPresence(ctx, userID, "available"); err != nil {
		slog.WarnContext(ctx, "presence could not be reset at shift start", "user", userID, "error", err)
		return
	}
	_ = s.repo.RecordTransition(ctx, userID, "available")
	s.broadcastExtensions(ctx)
	if ext := s.extensionOf(ctx, userID); ext != "" {
		if err := s.client.SetDND(ctx, ext, false); err != nil {
			slog.WarnContext(ctx, "dnd could not be lifted at shift start", "user", userID, "error", err)
		}
	}
}

// ShiftEnded takes the agent off the floor: the open presence stretch is
// closed so no more time accrues, the agent list shows "off shift", and the
// hosted PBX stops routing calls to the extension.
func (s *Service) ShiftEnded(ctx context.Context, userID uint) {
	if err := s.repo.SetPresence(ctx, userID, "off"); err != nil {
		slog.WarnContext(ctx, "presence could not be set at shift end", "user", userID, "error", err)
		return
	}
	_ = s.repo.CloseOpenEvent(ctx, userID)
	s.broadcastExtensions(ctx)
	if ext := s.extensionOf(ctx, userID); ext != "" {
		if err := s.client.SetDND(ctx, ext, true); err != nil {
			slog.WarnContext(ctx, "dnd could not be engaged at shift end", "user", userID, "error", err)
		}
	}
}

func (s *Service) extensionOf(ctx context.Context, userID uint) string {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil || u.SIPExtension == nil {
		return ""
	}
	return *u.SIPExtension
}

// Start launches the background poller that keeps a snapshot of extensions,
// queues and daily stats warm and copies new call records into the mirror, plus
// the one-off history backfill. The hosted API is rate limited (roughly 10
// requests/minute, and 2/minute on the status endpoint), so the panel must read
// from the snapshot and the mirror instead of hitting the API on every load.
func (s *Service) Start(ctx context.Context) {
	go s.poll(ctx)
	go s.runMirror(ctx)
}

func (s *Service) poll(ctx context.Context) {
	// Prime the snapshot in sequence with gaps, so the startup burst stays well
	// under the per-minute budget and does not throttle itself. Extensions come
	// first (and are slow, ~20s) so the agent list populates as early as possible.
	s.refreshExtensions(ctx)
	if !sleepCtx(ctx, 3*time.Second) {
		return
	}
	s.refreshHead(ctx)
	if !sleepCtx(ctx, 3*time.Second) {
		return
	}
	s.refreshStats(ctx)
	if !sleepCtx(ctx, 3*time.Second) {
		return
	}
	s.refreshQueues(ctx)
	s.finalizeStaleCalls(ctx)

	callsT := time.NewTicker(30 * time.Second)
	extT := time.NewTicker(40 * time.Second)
	statsT := time.NewTicker(60 * time.Second)
	queueT := time.NewTicker(5 * time.Minute)
	staleT := time.NewTicker(2 * time.Minute)
	defer callsT.Stop()
	defer extT.Stop()
	defer statsT.Stop()
	defer queueT.Stop()
	defer staleT.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-callsT.C:
			s.refreshHead(ctx)
		case <-extT.C:
			s.refreshExtensions(ctx)
		case <-statsT.C:
			s.refreshStats(ctx)
		case <-queueT.C:
			s.refreshQueues(ctx)
		case <-staleT.C:
			s.finalizeStaleCalls(ctx)
		}
	}
}

// finalizeStaleCalls closes call logs left open past the cap, so an unclosed row
// (its hangup was never recorded) stops reading as in-progress and stops
// inflating call time.
func (s *Service) finalizeStaleCalls(ctx context.Context) {
	if _, err := s.repo.FinalizeStaleCalls(ctx); err != nil {
		slog.WarnContext(ctx, "stale call finalize failed", "error", err)
	}
}

// sleepCtx waits for d or until ctx is cancelled; it returns false if the
// context ended (the caller should stop).
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// refreshHead copies the newest page of call records into the mirror. It is
// cheap (one API call) and runs every 30 seconds, so a finished call is
// searchable within half a minute.
func (s *Service) refreshHead(ctx context.Context) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("limit", strconv.Itoa(headPageSize))
	cdrs, _, err := s.client.CDRs(ctx, params)
	if err != nil {
		return // the next tick retries
	}
	if _, err := s.repo.UpsertCDRs(ctx, cdrs); err != nil {
		slog.WarnContext(ctx, "call records could not be mirrored", "error", err)
	}
}

func (s *Service) refreshExtensions(ctx context.Context) {
	raw, err := s.client.UserStatuses(ctx)
	if err != nil {
		return
	}
	out := make([]PBXExtension, 0, len(raw))
	for _, e := range raw {
		out = append(out, PBXExtension{Extension: strconv.Itoa(e.User), Status: e.Status})
	}
	s.snapMu.Lock()
	s.snap.exts = out
	s.snapMu.Unlock()
	// Push the fresh agent list to any live SSE subscribers immediately.
	s.broadcastExtensions(ctx)
}

func (s *Service) refreshQueues(ctx context.Context) {
	raw, err := s.client.Queues(ctx)
	if err != nil {
		return
	}
	out := make([]PBXQueue, 0, len(raw))
	for _, q := range raw {
		out = append(out, PBXQueue{Number: strconv.Itoa(q.Number), Name: q.Name})
	}
	s.snapMu.Lock()
	s.snap.queues = out
	s.snapMu.Unlock()
}

func (s *Service) refreshStats(ctx context.Context) {
	out, err := s.computeStats(ctx)
	if err != nil {
		return
	}
	s.snapMu.Lock()
	s.snap.stats = out
	s.snapMu.Unlock()
}

func (s *Service) snapExtensions() []PBXExtension {
	s.snapMu.RLock()
	defer s.snapMu.RUnlock()
	return s.snap.exts
}

func (s *Service) snapQueues() []PBXQueue {
	s.snapMu.RLock()
	defer s.snapMu.RUnlock()
	return s.snap.queues
}

func (s *Service) snapStats() *Stats {
	s.snapMu.RLock()
	defer s.snapMu.RUnlock()
	return s.snap.stats
}

// SIPCredentials is a softphone's WebRTC registration data.
type SIPCredentials struct {
	Extension    string `json:"extension"`
	Password     string `json:"password"`
	Domain       string `json:"domain"`
	WebSocketURL string `json:"webSocketUrl"`
	StunURL      string `json:"stunUrl,omitempty"`
	TurnURL      string `json:"turnUrl,omitempty"`
	TurnUser     string `json:"turnUser,omitempty"`
	TurnPass     string `json:"turnPass,omitempty"`
}

// Credentials returns the logged-in agent's own SIP registration data.
func (s *Service) Credentials(ctx context.Context, actorID uint) (*SIPCredentials, error) {
	u, err := s.repo.GetUser(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if u == nil {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	if u.SIPExtension == nil || *u.SIPExtension == "" || u.SIPSecret == nil || *u.SIPSecret == "" {
		return nil, errs.NotFound("Hesabınız için SIP bilgisi tanımlı değil.")
	}
	password, err := crypt.Decrypt(s.cfg.SIPKey, *u.SIPSecret)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return &SIPCredentials{
		Extension:    *u.SIPExtension,
		Password:     password,
		Domain:       s.cfg.SIPDomain,
		WebSocketURL: s.cfg.SIPWssURL,
		StunURL:      s.cfg.StunURL,
		TurnURL:      s.cfg.TurnURL,
		TurnUser:     s.cfg.TurnUser,
		TurnPass:     s.cfg.TurnPass,
	}, nil
}

// SetCredentials stores a user's SIP extension and password (encrypted).
func (s *Service) SetCredentials(ctx context.Context, actorID, targetID uint, extension, password string) error {
	if _, err := s.authorizeAny(ctx, actorID, enums.UserUpdate, enums.AgentManage); err != nil {
		return err
	}
	enc, err := crypt.Encrypt(s.cfg.SIPKey, password)
	if err != nil {
		return errs.Internal(err)
	}
	if err := s.repo.SetSIP(ctx, targetID, extension, enc); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// ProvisionSIP pulls a single extension's SIP password from Verimor and stores
// it (encrypted) for the target user, so the admin only enters the extension.
func (s *Service) ProvisionSIP(ctx context.Context, actorID, targetID uint, extension string) error {
	if _, err := s.authorizeAny(ctx, actorID, enums.UserUpdate, enums.AgentManage); err != nil {
		return err
	}
	if extension == "" {
		return errs.Invalid("Dahili numarası zorunlu.", nil)
	}
	pw, err := s.client.WebphoneSIP(ctx, s.cfg.WebphoneBase, extension)
	if err != nil {
		return errs.New(errs.CodeConflict, 502, "SIP şifresi alınamadı. Bu dahiliye Verimor'da bir personel tanımlı olmalı (webphone).", err)
	}
	enc, err := crypt.Encrypt(s.cfg.SIPKey, pw)
	if err != nil {
		return errs.Internal(err)
	}
	if err := s.repo.SetSIP(ctx, targetID, extension, enc); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// SIPSyncFailure is one extension that could not be synced, with why.
type SIPSyncFailure struct {
	Extension string `json:"extension"`
	Reason    string `json:"reason"`
}

// SyncAllSIP pulls the SIP password from Verimor for every user that has an
// extension and stores it, returning how many succeeded and which extensions
// failed with the reason, so a missing employee can be told apart from a
// throttle or a changed webphone page.
func (s *Service) SyncAllSIP(ctx context.Context, actorID uint) (int, []SIPSyncFailure, error) {
	if _, err := s.authorizeAny(ctx, actorID, enums.UserUpdate, enums.AgentManage); err != nil {
		return 0, nil, err
	}
	users, err := s.repo.UsersWithExtension(ctx)
	if err != nil {
		return 0, nil, errs.Internal(err)
	}
	ok := 0
	failed := make([]SIPSyncFailure, 0)
	for i, u := range users {
		// Each extension costs two API calls (token + page); space them out so a
		// bulk sync does not trip the hosted rate limit and fail every extension.
		if i > 0 {
			if !sleepCtx(ctx, 1500*time.Millisecond) {
				break
			}
		}
		pw, err := s.client.WebphoneSIP(ctx, s.cfg.WebphoneBase, u.Extension)
		if err != nil {
			failed = append(failed, SIPSyncFailure{Extension: u.Extension, Reason: err.Error()})
			continue
		}
		enc, err := crypt.Encrypt(s.cfg.SIPKey, pw)
		if err != nil {
			failed = append(failed, SIPSyncFailure{Extension: u.Extension, Reason: "şifre şifrelenemedi"})
			continue
		}
		if err := s.repo.SetSIP(ctx, u.ID, u.Extension, enc); err != nil {
			failed = append(failed, SIPSyncFailure{Extension: u.Extension, Reason: "kaydedilemedi"})
			continue
		}
		ok++
	}
	return ok, failed, nil
}

// Webphone is the embedded softphone descriptor for one agent.
type Webphone struct {
	Extension string `json:"extension"`
	URL       string `json:"url"`
}

// Call is the panel view of a call record.
type Call struct {
	UUID            string `json:"uuid"`
	Direction       string `json:"direction"`
	Disposition     string `json:"disposition"`
	FromNumber      string `json:"fromNumber"`
	ToNumber        string `json:"toNumber"`
	StartedAt       string `json:"startedAt"`
	DurationSeconds int    `json:"durationSeconds"`
	Recording       bool   `json:"recording"`
}

// CallList is a page of call records.
type CallList struct {
	Items      []Call `json:"items"`
	Page       int    `json:"page"`
	Total      int    `json:"total"`
	TotalPages int    `json:"totalPages"`
}

// Filter narrows a call-record query.
type Filter struct {
	Direction string
	Number    string
	Scope     string // "own" (default) or "all"
	From      string // inclusive start date, local "YYYY-MM-DD" (optional)
	To        string // inclusive end date, local "YYYY-MM-DD" (optional)
	Page      int
	Limit     int
}

// WebphoneURL returns the tokenized iframe URL for the actor's extension.
func (s *Service) WebphoneURL(ctx context.Context, actorID uint) (*Webphone, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if actor.SIPExtension == nil || *actor.SIPExtension == "" {
		return nil, errs.NotFound("Hesabınıza bir dahili numara atanmamış.")
	}
	token, err := s.client.WebphoneToken(ctx, *actor.SIPExtension)
	if err != nil {
		return nil, errs.New(errs.CodeConflict, 502, "Softphone jetonu alınamadı. Dahili için Verimor'da personel hesabı olmalı.", err)
	}
	return &Webphone{
		Extension: *actor.SIPExtension,
		URL:       s.cfg.WebphoneBase + "?token=" + url.QueryEscape(token),
	}, nil
}

// Export bounds: pages of 100 (the largest page Verimor serves reliably) and at
// most exportPages of them, so a date-backed export stays under a minute.
const (
	exportPageSize = 100
	exportPages    = 50
)

// ExportCalls returns the calls matching filter as UTF-8 CSV (with a BOM so
// Excel reads Turkish text). It pages through Calls so the export holds exactly
// what the list would show, bounded by exportPages.
func (s *Service) ExportCalls(ctx context.Context, actorID uint, filter Filter) ([]byte, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.CDRExport) {
		return nil, errs.Forbidden("Çağrı kayıtlarını dışa aktarma yetkiniz yok.")
	}
	var buf bytes.Buffer
	buf.WriteString("ï»¿") // UTF-8 BOM so Excel detects the encoding
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	_ = w.Write([]string{"Zaman", "Yön", "Kimden", "Kime", "Durum", "Süre (sn)", "Kayıt", "UUID"})
	filter.Limit = exportPageSize
	for page := 1; page <= exportPages; page++ {
		filter.Page = page
		list, err := s.Calls(ctx, actorID, filter)
		if err != nil {
			return nil, err
		}
		for _, c := range list.Items {
			rec := "hayır"
			if c.Recording {
				rec = "evet"
			}
			_ = w.Write([]string{c.StartedAt, c.Direction, c.FromNumber, c.ToNumber, c.Disposition, strconv.Itoa(c.DurationSeconds), rec, c.UUID})
		}
		if len(list.Items) < exportPageSize || page >= list.TotalPages {
			break
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, errs.Internal(err)
	}
	return buf.Bytes(), nil
}

// Calls returns a page of call records from the local mirror. Verimor's own
// filters cannot search by number or extension, so every query runs here:
// scope, number, direction and date range all become SQL.
func (s *Service) Calls(ctx context.Context, actorID uint, filter Filter) (*CallList, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !canViewCalls(actor) {
		return nil, errs.Forbidden("Çağrı kayıtlarını görme yetkiniz yok.")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 10 || filter.Limit > 100 {
		filter.Limit = 20
	}

	canAll := actor.Can(enums.CDRViewAll) || actor.Can(enums.CallViewAll)
	ext := ""
	if actor.SIPExtension != nil {
		ext = *actor.SIPExtension
	}
	number := strings.TrimSpace(filter.Number)
	q := CDRQuery{Direction: apiDirection(filter.Direction), Page: filter.Page, Limit: filter.Limit}
	switch {
	case filter.Scope != "all" || !canAll:
		// Own scope (or an agent limited to their own calls).
		if ext == "" {
			return &CallList{Items: []Call{}, Page: 1, TotalPages: 1}, nil // no extension -> no own calls
		}
		q.Ext = ext
		if !isExtension(number) {
			q.Phone = phoneQuery(number) // narrow own calls by a number
		}
	case isExtension(number):
		q.Ext = number // "belirli dahili" mode
	default:
		q.Phone = phoneQuery(number) // phone-number search (or none)
	}
	if day := dateOnly(filter.From); day != "" {
		if from, _, err := dayBounds(day); err == nil {
			q.From = from
		}
	}
	if day := dateOnly(filter.To); day != "" {
		if _, to, err := dayBounds(day); err == nil {
			q.To = to
		}
	}

	rows, total, err := s.repo.QueryCDRs(ctx, q)
	if err != nil {
		return nil, errs.Internal(err)
	}
	items := make([]Call, 0, len(rows))
	for i := range rows {
		items = append(items, mapCDR(rowCDR(rows[i])))
	}
	pages := int((total + int64(filter.Limit) - 1) / int64(filter.Limit))
	if pages < 1 {
		pages = 1
	}
	return &CallList{Items: items, Page: filter.Page, Total: int(total), TotalPages: pages}, nil
}

// extRe matches a short internal extension (3-4 digits) as opposed to a full
// external phone number.
var extRe = regexp.MustCompile(`^\d{3,4}$`)

func isExtension(s string) bool { return extRe.MatchString(strings.TrimSpace(s)) }

// dateOnly keeps a valid "YYYY-MM-DD" prefix and drops anything else.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

func dateOnly(s string) string {
	s = strings.TrimSpace(s)
	if m := dateRe.FindString(s); m != "" {
		return m
	}
	return ""
}

// Originate places a click-to-call from the actor's extension.
func (s *Service) Originate(ctx context.Context, actorID uint, destination string) (string, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return "", err
	}
	if !actor.Can(enums.CallOriginate) {
		return "", errs.Forbidden("Çağrı başlatma yetkiniz yok.")
	}
	if actor.SIPExtension == nil || *actor.SIPExtension == "" {
		return "", errs.Invalid("Hesabınızda tanımlı bir dahili numara yok.", nil)
	}
	if !s.onShift(ctx, actorID) {
		return "", errs.Forbidden("Çağrı başlatmak için önce mesai başlatın.")
	}
	uuid, err := s.client.Originate(ctx, *actor.SIPExtension, destination)
	if err != nil {
		return "", errs.Internal(err)
	}
	return uuid, nil
}

// PBXExtension is an extension and its live status.
type PBXExtension struct {
	Extension string   `json:"extension"`
	Status    string   `json:"status"`
	Names     []string `json:"names,omitempty"`    // active panel users on this extension
	Peer      string   `json:"peer,omitempty"`     // other party while TALKING (from the panel's own call log)
	PeerName  string   `json:"peerName,omitempty"` // contact name for Peer, when known
}

// PBXQueue is a call queue.
type PBXQueue struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

// Extensions lists extensions with live status from the warm snapshot, overlaid
// with our own persisted presence so a paused agent shows as paused rather than
// idle. All upstream fetches are owned by the background poller, so this hot
// path never touches the rate-limited API; it returns an empty list while
// warming.
func (s *Service) Extensions(ctx context.Context, actorID uint) ([]PBXExtension, error) {
	if _, err := s.authorizeAny(ctx, actorID, enums.AgentView, enums.CallTransfer); err != nil {
		return nil, err
	}
	return s.overlaidExtensions(ctx), nil
}

// overlaidExtensions builds the agent list from the warm snapshot with our
// persisted presence laid over it (a paused agent shows paused, not idle).
func (s *Service) overlaidExtensions(ctx context.Context) []PBXExtension {
	snap := s.snapExtensions()
	if snap == nil {
		return []PBXExtension{}
	}
	presence, err := s.repo.PresenceByExtension(ctx)
	if err != nil {
		presence = nil
	}
	names, err := s.repo.NamesByExtension(ctx)
	if err != nil {
		names = nil
	}
	peers, err := s.repo.OpenPeersByExtension(ctx)
	if err != nil {
		peers = nil
	}
	// Copy so the shared snapshot is never mutated; overlay presence only over
	// an idle (AVAILABLE) extension, so a live call (TALKING) still wins.
	out := make([]PBXExtension, len(snap))
	copy(out, snap)
	for i := range out {
		out[i].Names = names[out[i].Extension]
		if out[i].Status == "TALKING" {
			if peer := peers[out[i].Extension]; peer != "" {
				out[i].Peer = peer
				if s.contacts != nil {
					if e164, err := phone.Normalize(peer); err == nil {
						out[i].PeerName = s.contacts.NameByNumber(ctx, e164)
					}
				}
			}
		}
		if out[i].Status != "AVAILABLE" {
			continue
		}
		if state, ok := presence[out[i].Extension]; ok {
			out[i].Status = presenceStatus(state)
		}
	}
	return out
}

// ExtensionStatuses returns the overlaid live status of every extension, keyed
// by extension, for the team page.
func (s *Service) ExtensionStatuses(ctx context.Context) map[string]string {
	exts := s.overlaidExtensions(ctx)
	out := make(map[string]string, len(exts))
	for _, e := range exts {
		out[e.Extension] = e.Status
	}
	return out
}

// extensionsEvent is the SSE payload for a live agent-list update.
type extensionsEvent struct {
	Type  string         `json:"type"`
	Items []PBXExtension `json:"items"`
}

// extensionsJSON returns the current overlaid agent list as an SSE data payload.
func (s *Service) extensionsJSON(ctx context.Context) []byte {
	data, err := json.Marshal(extensionsEvent{Type: "extensions", Items: s.overlaidExtensions(ctx)})
	if err != nil {
		return []byte(`{"type":"extensions","items":[]}`)
	}
	return data
}

// broadcastExtensions pushes the current agent list to all SSE subscribers.
func (s *Service) broadcastExtensions(ctx context.Context) {
	s.broadcast(s.extensionsJSON(ctx))
}

// Subscribe registers an SSE subscriber and returns its channel. Authorization
// is the caller's responsibility (see StreamStart).
func (s *Service) subscribe() chan []byte {
	ch := make(chan []byte, 8)
	s.hubMu.Lock()
	if s.subs == nil {
		s.subs = make(map[chan []byte]struct{})
	}
	s.subs[ch] = struct{}{}
	s.hubMu.Unlock()
	return ch
}

func (s *Service) unsubscribe(ch chan []byte) {
	s.hubMu.Lock()
	if _, ok := s.subs[ch]; ok {
		delete(s.subs, ch)
		close(ch)
	}
	s.hubMu.Unlock()
}

func (s *Service) broadcast(msg []byte) {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- msg:
		default: // drop for a slow consumer rather than block the poller
		}
	}
}

// StreamStart authorizes an SSE client and returns the initial agent-list
// payload plus a channel of subsequent updates. Call StreamStop to release it.
func (s *Service) StreamStart(ctx context.Context, actorID uint) ([]byte, chan []byte, error) {
	if _, err := s.authorizeAny(ctx, actorID, enums.AgentView, enums.CallTransfer); err != nil {
		return nil, nil, err
	}
	return s.extensionsJSON(ctx), s.subscribe(), nil
}

// StreamStop releases an SSE subscriber channel.
func (s *Service) StreamStop(ch chan []byte) {
	s.unsubscribe(ch)
}

// presenceStatus maps a stored presence state to the status code the agent list
// renders.
func presenceStatus(state string) string {
	switch state {
	case "break":
		return "BREAK"
	case "backoffice":
		return "BACKOFFICE"
	case "dnd":
		return "SS_DND"
	case "off":
		return "OFF_SHIFT"
	default:
		return "AVAILABLE"
	}
}

// Queues lists call queues from the warm snapshot (poller-owned, never blocks).
func (s *Service) Queues(ctx context.Context, actorID uint) ([]PBXQueue, error) {
	if _, err := s.authorizeAny(ctx, actorID, enums.QueueView, enums.CallTransfer); err != nil {
		return nil, err
	}
	if snap := s.snapQueues(); snap != nil {
		return snap, nil
	}
	return []PBXQueue{}, nil
}

// Stats is a small daily call summary.
type Stats struct {
	Total  int `json:"total"`
	Missed int `json:"missed"`
}

// SetStatus records the actor's presence state and engages do-not-disturb on
// the hosted PBX for any non-available state (so a paused agent stops receiving
// calls). Presence is persisted so it survives reloads and shows in the agent
// list.
func (s *Service) SetStatus(ctx context.Context, actorID uint, state string) error {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if actor.SIPExtension == nil || *actor.SIPExtension == "" {
		return errs.Invalid("Hesabınızda tanımlı bir dahili numara yok.", nil)
	}
	if !s.onShift(ctx, actorID) {
		return errs.Forbidden("Durum değiştirmek için önce mesai başlatın.")
	}
	if state == "" {
		state = "available"
	}
	if err := s.repo.SetPresence(ctx, actorID, state); err != nil {
		return errs.Internal(err)
	}
	// Log the transition so per-state durations accumulate (best effort).
	_ = s.repo.RecordTransition(ctx, actorID, state)
	// Reflect the change on every open agent list instantly.
	s.broadcastExtensions(ctx)
	if err := s.client.SetDND(ctx, *actor.SIPExtension, state != "available"); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// Presence is the actor's current presence plus today's per-state totals and
// talk time (seconds), so the panel can show how long the agent held each state.
type Presence struct {
	State  string           `json:"state"`
	Since  string           `json:"since,omitempty"`
	Totals map[string]int64 `json:"totals"`
	Talk   int64            `json:"talk"`
	// Online is today's total time the agent has been present (seconds). The
	// per-state totals plus talk partition it: idle + talk + break + backoffice
	// + dnd == online.
	Online int64 `json:"online"`
	// Pauses are today's break / backoffice / dnd stretches, oldest first; the
	// open one has no EndedAt.
	Pauses []Pause `json:"pauses"`
	// BreakLimit is the daily break allowance in seconds.
	BreakLimit int64 `json:"breakLimit"`
}

// Pause is one non-available stretch of the day.
type Pause struct {
	State     string  `json:"state"`
	StartedAt string  `json:"startedAt"`
	EndedAt   *string `json:"endedAt,omitempty"`
}

// Status returns the actor's presence, when the current state started, and
// the current shift's accumulated durations per state plus total talk time.
// Every clock starts from zero at "Mesai Başlat" and nothing accrues off
// shift, so a state can never read as older than the shift.
func (s *Service) Status(ctx context.Context, actorID uint) (*Presence, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		return nil, err
	}
	from, onShift := s.shiftStart(ctx, actorID)
	if !onShift {
		return &Presence{State: "off", Totals: map[string]int64{}}, nil
	}
	state, since, err := s.repo.GetPresence(ctx, actorID)
	if err != nil {
		return &Presence{State: "available", Totals: map[string]int64{}}, nil
	}
	// Start the clock the first time the agent appears, so totals accumulate.
	_ = s.repo.EnsureOpenEvent(ctx, actorID, state)
	// Prefer the open stretch's start as "since" so the timer is stable across
	// page navigation (agent_presence.updated_at is absent until a manual change).
	if started, ok, _ := s.repo.OpenEventStartedAt(ctx, actorID); ok {
		since = started
	}
	// A stretch that predates the shift (left open by an old session) must not
	// leak into the timer.
	if since.Before(from) {
		since = from
	}

	totals, err := s.repo.PresenceTotals(ctx, actorID, from)
	if err != nil {
		totals = map[string]int64{}
	}
	// Online time is the sum of every presence stretch (talk time lives inside
	// the "available" stretches, so this already includes it).
	var online int64
	for _, v := range totals {
		online += v
	}
	call, _ := s.repo.CallSecondsToday(ctx, actorID, from)
	// Time on a call is talk, not idle, so carve it out of "available".
	if avail := totals["available"] - call; avail > 0 {
		totals["available"] = avail
	} else {
		delete(totals, "available")
	}
	// While available, the header timer should show the current idle streak (the
	// time since the last call ended), not the whole available stretch which
	// spans past calls. Other states time from when they were entered.
	if state == "available" {
		if last, ok, _ := s.repo.LastCallEndedAt(ctx, actorID, from); ok && last.After(since) {
			since = last
		}
	}
	out := &Presence{State: state, Totals: totals, Talk: call, Online: online, Pauses: []Pause{}, BreakLimit: 60 * 60}
	if s.breakLimit != nil {
		out.BreakLimit = int64(s.breakLimit.BreakLimitMinutes(ctx)) * 60
	}
	if !since.IsZero() {
		out.Since = since.UTC().Format(time.RFC3339)
	}
	if stretches, err := s.repo.PauseStretches(ctx, actorID, from); err == nil {
		for _, e := range stretches {
			start := e.StartedAt
			if start.Before(from) {
				start = from
			}
			p := Pause{State: e.State, StartedAt: start.UTC().Format(time.RFC3339)}
			if e.EndedAt != nil {
				ended := e.EndedAt.UTC().Format(time.RFC3339)
				p.EndedAt = &ended
			}
			out.Pauses = append(out.Pauses, p)
		}
	}
	return out, nil
}

// Stats returns today's tenant call totals from the warm snapshot (poller-owned,
// never blocks). Zero totals are returned while the snapshot is warming.
func (s *Service) Stats(ctx context.Context, actorID uint) (*Stats, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		return nil, err
	}
	if snap := s.snapStats(); snap != nil {
		return snap, nil
	}
	return &Stats{}, nil
}

// istanbul is the tenant's timezone (UTC+3, no DST). Using a fixed zone avoids
// depending on the OS tzdata being present.
var istanbul = time.FixedZone("+03", 3*3600)

// todayStart returns the UTC instant of local (Istanbul) midnight today, so
// "today" resets at 00:00 local rather than at 00:00 UTC.
func todayStart() time.Time {
	now := time.Now().In(istanbul)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, istanbul)
}

// computeStats reads today's total and missed call counts from the hosted API.
func (s *Service) computeStats(ctx context.Context) (*Stats, error) {
	from := todayStart().UTC().Format("2006-01-02 15:04:05") + " UTC"
	base := func(missed bool) url.Values {
		v := url.Values{}
		v.Set("start_stamp_from", from)
		if missed {
			v.Set("missed", "true")
		}
		return v
	}
	total, err := s.client.CDRCount(ctx, base(false))
	if err != nil {
		return nil, err
	}
	missed, err := s.client.CDRCount(ctx, base(true))
	if err != nil {
		missed = 0
	}
	return &Stats{Total: total, Missed: missed}, nil
}

func (s *Service) authorizeTransfer(ctx context.Context, actorID uint) error {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.CallTransfer) {
		return errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	return nil
}

// authorizeAny loads the actor and passes when they hold at least one of the
// permissions.
func (s *Service) authorizeAny(ctx context.Context, actorID uint, perms ...enums.Permission) (*models.User, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		if actor.Can(p) {
			return actor, nil
		}
	}
	return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
}

func canViewCalls(u *models.User) bool {
	return u.Can(enums.CDRViewAll) || u.Can(enums.CallViewAll) ||
		u.Can(enums.CDRViewOwn) || u.Can(enums.CallViewOwn)
}

func canAccessRecording(u *models.User) bool {
	return u.Can(enums.CallRecordAccess) || u.Can(enums.CDRViewAll) || u.Can(enums.CallViewAll)
}

// Recording mints a one-time URL for a call's recording and returns the live
// download response so the handler can stream it. The caller must close the
// response body.
func (s *Service) Recording(ctx context.Context, actorID uint, callUUID string) (*http.Response, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !canAccessRecording(actor) {
		return nil, errs.Forbidden("Çağrı kaydına erişim yetkiniz yok.")
	}
	mintedURL, err := s.client.RecordingURL(ctx, callUUID)
	if err != nil {
		return nil, errs.New(errs.CodeConflict, 502, "Çağrı kaydı bulunamadı veya alınamadı.", err)
	}
	res, err := s.client.OpenRecording(ctx, mintedURL)
	if err != nil {
		return nil, errs.New(errs.CodeConflict, 502, "Çağrı kaydı indirilemedi.", err)
	}
	return res, nil
}

func apiDirection(v string) string {
	switch v {
	case "inbound", "outbound", "internal":
		return v
	default:
		return ""
	}
}

func mapCDR(c CDR) Call {
	return Call{
		UUID:            c.CallUUID,
		Direction:       normalizeDirection(c.Direction),
		Disposition:     cdrDisposition(c),
		FromNumber:      c.CallerIDNumber,
		ToNumber:        c.DestinationNumber,
		StartedAt:       c.StartStamp,
		DurationSeconds: parseDuration(c.Duration),
		Recording:       bool(c.RecordingPresent),
	}
}

// cdrDisposition trusts answer_stamp: a call is answered only if it has one.
// The hosted API's `result` label alone is unreliable ("Vazgeçildi" abandoned
// calls have no answer_stamp yet were mislabeled as answered).
func cdrDisposition(c CDR) string {
	if strings.TrimSpace(c.AnswerStamp) != "" {
		return "answered"
	}
	switch strings.ToLower(strings.TrimSpace(c.Result)) {
	case "meşgul", "mesgul", "busy":
		return "busy"
	case "vazgeçildi", "vazgecildi", "iptal", "canceled", "cancelled":
		return "canceled"
	}
	return "no_answer"
}

func normalizeDirection(v string) string {
	switch strings.ToLower(v) {
	case "gelen", "inbound":
		return "inbound"
	case "giden", "outbound":
		return "outbound"
	case "iç", "ic", "dahili", "internal":
		return "internal"
	default:
		return v
	}
}

// parseDuration turns "hh:mm:ss" or "mm:ss" into seconds.
func parseDuration(v string) int {
	parts := strings.Split(strings.TrimSpace(v), ":")
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}
