package verimor

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
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
	client       *Client
	users        IActorResolver
	webphoneBase string

	mu    sync.Mutex
	cache map[string]cdrCacheEntry
}

type cdrCacheEntry struct {
	list *CallList
	at   time.Time
}

// NewService builds a Verimor service.
func NewService(client *Client, users IActorResolver, webphoneBase string) *Service {
	if webphoneBase == "" {
		webphoneBase = "https://oim.verimor.com.tr/webphone"
	}
	return &Service{client: client, users: users, webphoneBase: webphoneBase, cache: make(map[string]cdrCacheEntry)}
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
		URL:       s.webphoneBase + "?token=" + url.QueryEscape(token),
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
