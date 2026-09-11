package verimor

import (
	"context"
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
)

// cdrTTL is how long a CDR page is cached; the hosted API is rate limited, so
// repeated panel loads must not each hit it.
const cdrTTL = 30 * time.Second

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// Service exposes the hosted PBX to the panel.
type Service struct {
	client *Client
	users  IActorResolver
	repo   *Repository
	cfg    configs.Bulutsantralim

	mu      sync.Mutex
	cache   map[string]cdrCacheEntry
	cdrCool time.Time // until when on-demand CDR calls back off after a 429/timeout
	scan    []CDR     // recent CDR window for in-process extension filtering
	scanAt  time.Time

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
	calls  *CallList
	exts   []PBXExtension
	queues []PBXQueue
	stats  *Stats
}

type cdrCacheEntry struct {
	list *CallList
	at   time.Time
}

// NewService builds a Verimor service.
func NewService(client *Client, users IActorResolver, repo *Repository, cfg configs.Bulutsantralim) *Service {
	if cfg.WebphoneBase == "" {
		cfg.WebphoneBase = "https://oim.verimor.com.tr/webphone"
	}
	return &Service{client: client, users: users, repo: repo, cfg: cfg, cache: make(map[string]cdrCacheEntry)}
}

// snapshotLimit is the page size the background poller keeps warm for the
// panel's call history.
const snapshotLimit = 20

// Start launches the background poller that keeps a snapshot of call history,
// extensions, queues and daily stats warm. The hosted API is rate limited
// (roughly 10 requests/minute, and 2/minute on the status endpoint), so the
// panel must read from this snapshot instead of hitting the API on every load.
func (s *Service) Start(ctx context.Context) {
	go s.poll(ctx)
}

func (s *Service) poll(ctx context.Context) {
	// Prime the snapshot in sequence with gaps, so the startup burst stays well
	// under the per-minute budget and does not throttle itself. Extensions come
	// first (and are slow, ~20s) so the agent list populates as early as possible.
	s.refreshExtensions(ctx)
	if !sleepCtx(ctx, 3*time.Second) {
		return
	}
	s.refreshCalls(ctx)
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
			s.refreshCalls(ctx)
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

func (s *Service) refreshCalls(ctx context.Context) {
	params := url.Values{}
	params.Set("page", "1")
	params.Set("limit", strconv.Itoa(snapshotLimit))
	cdrs, pg, err := s.client.CDRs(ctx, params)
	if err != nil {
		return // keep the last good snapshot
	}
	items := make([]Call, 0, len(cdrs))
	for i := range cdrs {
		items = append(items, mapCDR(cdrs[i]))
	}
	list := &CallList{Items: items, Page: pg.Page, Total: pg.TotalCount, TotalPages: pg.TotalPages}
	fillPaging(list, 1, snapshotLimit, len(items))
	s.snapMu.Lock()
	s.snap.calls = list
	s.snapMu.Unlock()
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

func (s *Service) snapCalls() *CallList {
	s.snapMu.RLock()
	defer s.snapMu.RUnlock()
	return s.snap.calls
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
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.UserUpdate) {
		return errs.Forbidden("Bu işlem için yetkiniz yok.")
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
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.UserUpdate) {
		return errs.Forbidden("Bu işlem için yetkiniz yok.")
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
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return 0, nil, err
	}
	if !actor.Can(enums.UserUpdate) {
		return 0, nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
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

// Calls returns a page of tenant call records.
func (s *Service) Calls(ctx context.Context, actorID uint, filter Filter) (*CallList, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !canViewCalls(actor) {
		return nil, errs.Forbidden("Çağrı kayıtlarını görme yetkiniz yok.")
	}

	// Scope resolution. Verimor's `number` filter does NOT work for short internal
	// extensions (it returns nearly the whole santral), so scoping to one extension
	// is done in-process over a recent window. A real phone-number search still uses
	// Verimor's own filter.
	canAll := actor.Can(enums.CDRViewAll) || actor.Can(enums.CallViewAll)
	ext := ""
	if actor.SIPExtension != nil {
		ext = *actor.SIPExtension
	}
	extFilter, phoneSearch := "", ""
	switch {
	case filter.Scope != "all" || !canAll:
		// Own scope (or an agent limited to their own calls).
		if ext == "" {
			return &CallList{Items: []Call{}, Page: 1}, nil // no extension -> no own calls
		}
		extFilter = ext
		if !isExtension(filter.Number) {
			phoneSearch = strings.TrimSpace(filter.Number) // narrow own calls by a number
		}
	case isExtension(filter.Number):
		extFilter = strings.TrimSpace(filter.Number) // "belirli dahili" mode
	default:
		phoneSearch = strings.TrimSpace(filter.Number) // real phone-number search (or none)
	}

	if extFilter != "" {
		return s.extScopedCalls(ctx, extFilter, phoneSearch, filter)
	}
	filter.Number = phoneSearch

	// The unfiltered first page is kept warm by the background poller; serve it
	// straight from the snapshot so the panel never touches the rate-limited API
	// on its hot path. While the snapshot is still warming, return an empty page
	// (200) rather than generating upstream load that would only deepen the
	// throttle; the poller fills it within one interval.
	if filter.Number == "" && apiDirection(filter.Direction) == "" && filter.Page <= 1 {
		if snap := s.snapCalls(); snap != nil {
			return snap, nil
		}
		return &CallList{Items: []Call{}, Page: 1}, nil
	}

	params := url.Values{}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 10 || filter.Limit > 100 {
		filter.Limit = 20
	}
	params.Set("page", strconv.Itoa(filter.Page))
	params.Set("limit", strconv.Itoa(filter.Limit))
	if d := apiDirection(filter.Direction); d != "" {
		params.Set("direction", d)
	}
	if filter.Number != "" {
		// `number` matches either party and does partial matching, unlike
		// `caller_id_number` which is an exact caller-only match.
		params.Set("number", filter.Number)
	}

	key := params.Encode()
	if cached, ok := s.cachedCalls(key); ok {
		return cached, nil
	}

	// After a throttle (429) or timeout, stop hammering the hosted API for a
	// short cooldown: rapid filtered/paged CDR queries would otherwise keep
	// tripping the shared rate limit and starve user_statuses and SIP sync. Serve
	// a stale page if we have one, else an empty page (200) so the UI stays calm.
	if s.cdrCooling() {
		if stale, ok := s.staleCalls(key); ok {
			return stale, nil
		}
		return &CallList{Items: []Call{}, Page: filter.Page, TotalPages: filter.Page}, nil
	}

	cdrs, pg, err := s.client.CDRs(ctx, params)
	if err != nil {
		s.startCDRCooldown()
		// Serve a stale page rather than fail when the hosted API is
		// throttled or slow.
		if stale, ok := s.staleCalls(key); ok {
			return stale, nil
		}
		return nil, errs.New(errs.CodeConflict, 502, "Çağrı kayıtları şu an alınamıyor (santral yoğun). Birazdan tekrar deneyin.", err)
	}
	items := make([]Call, 0, len(cdrs))
	for i := range cdrs {
		items = append(items, mapCDR(cdrs[i]))
	}
	list := &CallList{Items: items, Page: pg.Page, Total: pg.TotalCount, TotalPages: pg.TotalPages}
	fillPaging(list, filter.Page, filter.Limit, len(items))
	s.storeCalls(key, list)
	return list, nil
}

// extRe matches a short internal extension (3-4 digits) as opposed to a full
// external phone number.
var extRe = regexp.MustCompile(`^\d{3,4}$`)

func isExtension(s string) bool { return extRe.MatchString(strings.TrimSpace(s)) }

// extScopeScan is how many recent CDRs are scanned when filtering to one
// extension in-process (Verimor cannot filter CDRs by a short extension).
const extScopeScan = 200

// extScopedCalls returns a page of calls where `ext` is an internal party (its
// outbound and internal calls), optionally narrowed by a phone substring. It
// scans a recent window and paginates in-process.
func (s *Service) extScopedCalls(ctx context.Context, ext, phone string, filter Filter) (*CallList, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	limit := filter.Limit
	if limit < 10 || limit > 100 {
		limit = 20
	}
	scan, err := s.recentScan(ctx)
	if err != nil {
		return nil, err
	}
	dir := apiDirection(filter.Direction)
	phone = strings.TrimSpace(phone)
	matched := make([]Call, 0, 32)
	for i := range scan {
		if !extIsParty(scan[i], ext) {
			continue
		}
		call := mapCDR(scan[i])
		if dir != "" && call.Direction != dir {
			continue
		}
		if phone != "" && !strings.Contains(call.FromNumber, phone) && !strings.Contains(call.ToNumber, phone) {
			continue
		}
		matched = append(matched, call)
	}
	total := len(matched)
	pages := (total + limit - 1) / limit
	if pages < 1 {
		pages = 1
	}
	start := (filter.Page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return &CallList{Items: matched[start:end], Page: filter.Page, Total: total, TotalPages: pages}, nil
}

// extIsParty reports whether the extension is a party on the call (as caller or
// callee). An extension shows as "1014 (902...)" in the caller id, or as the bare
// number in an internal call.
func extIsParty(c CDR, ext string) bool {
	return partyIsExt(c.CallerIDNumber, ext) || partyIsExt(c.DestinationNumber, ext)
}

func partyIsExt(field, ext string) bool {
	field = strings.TrimSpace(field)
	return field == ext || strings.HasPrefix(field, ext+" ") || strings.HasPrefix(field, ext+"(")
}

// recentScan returns a cached window of the most recent CDRs (santral-wide) for
// in-process extension filtering. It fetches a few pages, caps the API calls, and
// honours the throttle cooldown.
func (s *Service) recentScan(ctx context.Context) ([]CDR, error) {
	if c, ok := s.cachedScan(); ok {
		return c, nil
	}
	if s.cdrCooling() {
		if c, ok := s.staleScan(); ok {
			return c, nil
		}
		return nil, errs.New(errs.CodeConflict, 502, "Çağrı kayıtları şu an alınamıyor (santral yoğun). Birazdan tekrar deneyin.", nil)
	}
	const pageSize = 50
	out := make([]CDR, 0, extScopeScan)
	for page := 1; len(out) < extScopeScan && page <= 4; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("limit", strconv.Itoa(pageSize))
		cdrs, pg, err := s.client.CDRs(ctx, params)
		if err != nil {
			s.startCDRCooldown()
			if len(out) > 0 {
				break // use what we already have
			}
			if stale, ok := s.staleScan(); ok {
				return stale, nil
			}
			return nil, errs.New(errs.CodeConflict, 502, "Çağrı kayıtları şu an alınamıyor (santral yoğun). Birazdan tekrar deneyin.", err)
		}
		out = append(out, cdrs...)
		if len(cdrs) < pageSize || pg.Page >= pg.TotalPages {
			break
		}
	}
	s.storeScan(out)
	return out, nil
}

func (s *Service) cachedScan() ([]CDR, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scan != nil && time.Since(s.scanAt) < cdrTTL {
		return s.scan, true
	}
	return nil, false
}

func (s *Service) staleScan() ([]CDR, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scan, s.scan != nil
}

func (s *Service) storeScan(c []CDR) {
	s.mu.Lock()
	s.scan = c
	s.scanAt = time.Now()
	s.mu.Unlock()
}

// fillPaging backfills the page envelope when the hosted API omits or zeroes the
// total count/pages, so the panel's Next/Previous controls still work. It derives
// pages from the total when known, otherwise assumes a full page means there is at
// least one more.
func fillPaging(list *CallList, page, limit, got int) {
	if list.Page < 1 {
		if page < 1 {
			page = 1
		}
		list.Page = page
	}
	if limit < 1 {
		limit = snapshotLimit
	}
	if list.TotalPages >= 1 {
		return
	}
	switch {
	case list.Total > 0:
		list.TotalPages = (list.Total + limit - 1) / limit
	case got >= limit:
		list.TotalPages = list.Page + 1 // a full page likely has more
	default:
		list.TotalPages = list.Page
	}
}

// cdrCooldown is how long on-demand CDR calls back off after the hosted API
// throttles or times out, to keep a burst from starving the shared rate limit.
const cdrCooldown = 20 * time.Second

func (s *Service) cdrCooling() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.cdrCool)
}

func (s *Service) startCDRCooldown() {
	s.mu.Lock()
	s.cdrCool = time.Now().Add(cdrCooldown)
	s.mu.Unlock()
}

func (s *Service) cachedCalls(key string) (*CallList, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[key]
	if ok && time.Since(e.at) < cdrTTL {
		return e.list, true
	}
	return nil, false
}

func (s *Service) staleCalls(key string) (*CallList, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[key]
	return e.list, ok
}

func (s *Service) storeCalls(key string, list *CallList) {
	s.mu.Lock()
	s.cache[key] = cdrCacheEntry{list: list, at: time.Now()}
	s.mu.Unlock()
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
	uuid, err := s.client.Originate(ctx, *actor.SIPExtension, destination)
	if err != nil {
		return "", errs.Internal(err)
	}
	return uuid, nil
}

// PBXExtension is an extension and its live status.
type PBXExtension struct {
	Extension string `json:"extension"`
	Status    string `json:"status"`
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
	if err := s.authorizeTransfer(ctx, actorID); err != nil {
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
	if err != nil || len(presence) == 0 {
		return snap
	}
	// Copy so the shared snapshot is never mutated; overlay presence only over
	// an idle (AVAILABLE) extension, so a live call (TALKING) still wins.
	out := make([]PBXExtension, len(snap))
	copy(out, snap)
	for i := range out {
		if out[i].Status != "AVAILABLE" {
			continue
		}
		if state, ok := presence[out[i].Extension]; ok {
			out[i].Status = presenceStatus(state)
		}
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
	if err := s.authorizeTransfer(ctx, actorID); err != nil {
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
	default:
		return "AVAILABLE"
	}
}

// Queues lists call queues from the warm snapshot (poller-owned, never blocks).
func (s *Service) Queues(ctx context.Context, actorID uint) ([]PBXQueue, error) {
	if err := s.authorizeTransfer(ctx, actorID); err != nil {
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
}

// Status returns the actor's presence, when the current state started, and
// today's accumulated durations per state plus total talk time.
func (s *Service) Status(ctx context.Context, actorID uint) (*Presence, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		return nil, err
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

	from := todayStart()
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

	out := &Presence{State: state, Totals: totals, Talk: call, Online: online}
	if !since.IsZero() {
		out.Since = since.UTC().Format(time.RFC3339)
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
