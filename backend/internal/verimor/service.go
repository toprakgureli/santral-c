package verimor

import (
	"context"
	"net/url"
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

	mu    sync.Mutex
	cache map[string]cdrCacheEntry

	dirMu      sync.Mutex
	extCache   []PBXExtension
	extAt      time.Time
	queueCache []PBXQueue
	queueAt    time.Time
	statsCache *Stats
	statsAt    time.Time
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
		params.Set("caller_id_number", filter.Number)
	}

	key := params.Encode()
	if cached, ok := s.cachedCalls(key); ok {
		return cached, nil
	}

	cdrs, pg, err := s.client.CDRs(ctx, params)
	if err != nil {
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
	s.storeCalls(key, list)
	return list, nil
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

const (
	extTTL   = 60 * time.Second
	queueTTL = 300 * time.Second
)

// Extensions lists extensions with status, cached to respect the hosted API's
// rate limit (2 requests/minute on this endpoint).
func (s *Service) Extensions(ctx context.Context, actorID uint) ([]PBXExtension, error) {
	if err := s.authorizeTransfer(ctx, actorID); err != nil {
		return nil, err
	}
	s.dirMu.Lock()
	if s.extCache != nil && time.Since(s.extAt) < extTTL {
		out := s.extCache
		s.dirMu.Unlock()
		return out, nil
	}
	s.dirMu.Unlock()

	raw, err := s.client.UserStatuses(ctx)
	if err != nil {
		s.dirMu.Lock()
		stale := s.extCache
		s.dirMu.Unlock()
		if stale != nil {
			return stale, nil
		}
		return nil, errs.New(errs.CodeConflict, 502, "Dahili listesi alınamadı (santral yoğun).", err)
	}
	out := make([]PBXExtension, 0, len(raw))
	for _, e := range raw {
		out = append(out, PBXExtension{Extension: strconv.Itoa(e.User), Status: e.Status})
	}
	s.dirMu.Lock()
	s.extCache = out
	s.extAt = time.Now()
	s.dirMu.Unlock()
	return out, nil
}

// Queues lists call queues, cached to respect the rate limit.
func (s *Service) Queues(ctx context.Context, actorID uint) ([]PBXQueue, error) {
	if err := s.authorizeTransfer(ctx, actorID); err != nil {
		return nil, err
	}
	s.dirMu.Lock()
	if s.queueCache != nil && time.Since(s.queueAt) < queueTTL {
		out := s.queueCache
		s.dirMu.Unlock()
		return out, nil
	}
	s.dirMu.Unlock()

	raw, err := s.client.Queues(ctx)
	if err != nil {
		s.dirMu.Lock()
		stale := s.queueCache
		s.dirMu.Unlock()
		if stale != nil {
			return stale, nil
		}
		return nil, errs.New(errs.CodeConflict, 502, "Kuyruk listesi alınamadı (santral yoğun).", err)
	}
	out := make([]PBXQueue, 0, len(raw))
	for _, q := range raw {
		out = append(out, PBXQueue{Number: strconv.Itoa(q.Number), Name: q.Name})
	}
	s.dirMu.Lock()
	s.queueCache = out
	s.queueAt = time.Now()
	s.dirMu.Unlock()
	return out, nil
}

// Stats is a small daily call summary.
type Stats struct {
	Total  int `json:"total"`
	Missed int `json:"missed"`
}

// SetStatus sets the actor's do-not-disturb state (true = no incoming calls).
func (s *Service) SetStatus(ctx context.Context, actorID uint, dnd bool) error {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if actor.SIPExtension == nil || *actor.SIPExtension == "" {
		return errs.Invalid("Hesabınızda tanımlı bir dahili numara yok.", nil)
	}
	if err := s.client.SetDND(ctx, *actor.SIPExtension, dnd); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// Stats returns today's tenant call totals, cached to respect the rate limit.
func (s *Service) Stats(ctx context.Context, actorID uint) (*Stats, error) {
	if _, err := s.users.GetByID(ctx, actorID); err != nil {
		return nil, err
	}
	s.dirMu.Lock()
	if s.statsCache != nil && time.Since(s.statsAt) < 60*time.Second {
		out := s.statsCache
		s.dirMu.Unlock()
		return out, nil
	}
	s.dirMu.Unlock()

	from := time.Now().UTC().Format("2006-01-02") + " 00:00:00 UTC"
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
		s.dirMu.Lock()
		stale := s.statsCache
		s.dirMu.Unlock()
		if stale != nil {
			return stale, nil
		}
		return nil, errs.New(errs.CodeConflict, 502, "İstatistik alınamadı (santral yoğun).", err)
	}
	missed, err := s.client.CDRCount(ctx, base(true))
	if err != nil {
		missed = 0
	}
	out := &Stats{Total: total, Missed: missed}
	s.dirMu.Lock()
	s.statsCache = out
	s.statsAt = time.Now()
	s.dirMu.Unlock()
	return out, nil
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
		Disposition:     normalizeResult(c.Result, c.Missed),
		FromNumber:      c.CallerIDNumber,
		ToNumber:        c.DestinationNumber,
		StartedAt:       c.StartStamp,
		DurationSeconds: parseDuration(c.Duration),
		Recording:       c.RecordingPresent,
	}
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

func normalizeResult(result string, missed bool) string {
	switch strings.ToLower(result) {
	case "cevaplandı", "cevaplandi", "answered":
		return "answered"
	case "meşgul", "mesgul", "busy":
		return "busy"
	case "cevapsız", "cevapsiz", "cevaplanmadı", "no answer", "noanswer":
		return "no_answer"
	}
	if missed {
		return "no_answer"
	}
	return "answered"
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
