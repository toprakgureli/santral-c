package whatsapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/store"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/safe"
)

// IUsers loads a user with roles and permissions.
type IUsers interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IPusher delivers live events to the panel and says who has it open.
type IPusher interface {
	Push(ids []uint, event any)
	OnlineUsers(ids []uint) map[uint]bool
	SubscribeRaw(userID uint) (chan []byte, func())
}

// IStorage keeps media files (the Drive connected for chat attachments).
type IStorage interface {
	Connected(ctx context.Context) bool
	Folder(ctx context.Context) (string, error)
	EnsureFolder(ctx context.Context, name, parent string) (string, error)
	Put(ctx context.Context, folder, name, mime string, data []byte) (string, error)
	Open(ctx context.Context, id, rangeHeader string) (*http.Response, error)
}

// IAudit records who changed what.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// Service is the WhatsApp module. It reaches the database only through
// repo.
type Service struct {
	repo    *store.Repository
	users   IUsers
	push    IPusher
	storage IStorage
	audit   IAudit
	// ring encrypts the secrets kept in the database; secret only signs
	// survey links.
	ring   *crypt.Keyring
	secret string

	wakeWebhook chan struct{}
	wakeOutbox  chan struct{}
	// wakeInbound tells a follow-up worker that a message's work is waiting.
	wakeInbound chan struct{}

	mu        sync.Mutex
	viewers   map[uint]*viewer
	viewersAt time.Time
	folders   map[uint]string
	// uploaded files already on Meta, by device and file
	metaFiles map[string]metaFile
}

// NewService builds the module. ring encrypts stored secrets; secret signs
// the links sent in surveys.
func NewService(db *gorm.DB, users IUsers, push IPusher, storage IStorage, auditor IAudit, ring *crypt.Keyring, secret string) *Service {
	return &Service{
		repo:        store.New(db),
		users:       users,
		push:        push,
		storage:     storage,
		audit:       auditor,
		ring:        ring,
		secret:      "wa:" + secret,
		wakeWebhook: make(chan struct{}, 1),
		wakeOutbox:  make(chan struct{}, 1),
		wakeInbound: make(chan struct{}, inboundWorkers),
		folders:     map[uint]string{},
		metaFiles:   map[string]metaFile{},
	}
}

// Start runs the background workers until ctx ends.
func (s *Service) Start(ctx context.Context, g *safe.Group) {
	g.Loop(ctx, "whatsapp webhook worker", s.webhookWorker)
	g.Loop(ctx, "whatsapp outbox worker", s.outboxWorker)
	g.Loop(ctx, "whatsapp clock", s.clock)
	for i := 0; i < inboundWorkers; i++ {
		g.Loop(ctx, "whatsapp follow-up worker", s.inboundWorker)
	}
	g.Loop(ctx, "whatsapp media", s.mediaLoop)
}

func wake(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// ---------------------------------------------------------------- secrets

// SealPurpose is the keyring label of WhatsApp's stored secrets.
const SealPurpose = "whatsapp"

func (s *Service) seal(plain string) (string, error) {
	return s.ring.Seal(SealPurpose, plain)
}

// open decrypts a stored secret. A value that cannot be opened reads as
// not set, and the failure is logged so a wrong data key is noticed.
func (s *Service) open(enc string) string {
	out, err := s.ring.Open(SealPurpose, enc)
	if err != nil {
		slog.Error("stored whatsapp secret could not be decrypted", "error", err)
		return ""
	}
	return out
}

func randomKey(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------- who sees what

// viewer is one person's reach in the module: permissions, devices and
// teams. It is cached briefly so events can be addressed quickly.
type viewer struct {
	user     *models.User
	channels map[uint]bool
	teams    map[uint]bool
}

func (v *viewer) can(p enums.Permission) bool { return v.user.Can(p) }

func (v *viewer) seesChannel(id uint) bool {
	return v.can(enums.WAView) && (v.can(enums.WAViewAll) || v.channels[id])
}

// seesTicket decides whether a person sees a ticket at all.
func (v *viewer) seesTicket(t *models.WATicket, participants map[uint]bool) bool {
	if t == nil || !v.seesChannel(t.ChannelID) {
		return false
	}
	uid := v.user.ID
	switch {
	case v.can(enums.WAViewAll):
		return true
	case t.OwnerID != nil && *t.OwnerID == uid:
		return true
	case participants[uid]:
		return true
	case v.can(enums.WAViewTeam) && t.TeamID != nil && v.teams[*t.TeamID]:
		return true
	case t.Status != "resolved" && t.Status != "bot" && t.OwnerID == nil && v.can(enums.WAPool):
		return true
	case t.WaitingListedAt != nil && t.Status != "resolved" && v.can(enums.WAWaiting):
		return true
	}
	return false
}

// reach is seesTicket for the database: which tickets the viewer sees,
// for narrowing a query before its LIMIT, so a person with a narrow view
// still gets a full page of what they may see.
func (v *viewer) reach() store.Reach {
	if !v.can(enums.WAView) {
		return store.Reach{None: true}
	}
	if v.can(enums.WAViewAll) {
		return store.Reach{All: true}
	}
	r := store.Reach{
		Channels: make([]uint, 0, len(v.channels)),
		UserID:   v.user.ID,
		Pool:     v.can(enums.WAPool),
		Waiting:  v.can(enums.WAWaiting),
	}
	for id := range v.channels {
		r.Channels = append(r.Channels, id)
	}
	if v.can(enums.WAViewTeam) {
		for id := range v.teams {
			r.Teams = append(r.Teams, id)
		}
	}
	return r
}

// likeEscape makes typed text match itself in a LIKE pattern: % and _
// are not wildcards there.
var likeEscape = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

// loadViewers reads everyone who may use the module.
func (s *Service) loadViewers(ctx context.Context) (map[uint]*viewer, error) {
	s.mu.Lock()
	if s.viewers != nil && time.Since(s.viewersAt) < 30*time.Second {
		v := s.viewers
		s.mu.Unlock()
		return v, nil
	}
	s.mu.Unlock()

	ids, err := s.repo.ActiveUsersWithPermission(ctx, string(enums.WAView))
	if err != nil {
		return nil, err
	}
	members, err := s.repo.ChannelMembers(ctx)
	if err != nil {
		return nil, err
	}
	teams, err := s.repo.TeamMembers(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uint]*viewer, len(ids))
	for _, id := range ids {
		u, err := s.users.GetByID(ctx, id)
		if err != nil || u == nil {
			continue
		}
		out[id] = &viewer{user: u, channels: map[uint]bool{}, teams: map[uint]bool{}}
	}
	for _, m := range members {
		if v := out[m.UserID]; v != nil {
			v.channels[m.ChannelID] = true
		}
	}
	for _, t := range teams {
		if v := out[t.UserID]; v != nil {
			v.teams[t.TeamID] = true
		}
	}
	s.mu.Lock()
	s.viewers = out
	s.viewersAt = time.Now()
	s.mu.Unlock()
	return out, nil
}

// forget drops the cache after a change of members or teams.
func (s *Service) forget() {
	s.mu.Lock()
	s.viewers = nil
	s.mu.Unlock()
}

// viewerOf loads one person fresh, for a request they make.
func (s *Service) viewerOf(ctx context.Context, userID uint) (*viewer, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.Can(enums.WAView) {
		return nil, errs.Forbidden("WhatsApp'ı kullanma yetkin yok.")
	}
	v := &viewer{user: u, channels: map[uint]bool{}, teams: map[uint]bool{}}
	chans, err := s.repo.ChannelsOfUser(ctx, userID)
	warnDB(ctx, err)
	for _, c := range chans {
		v.channels[c] = true
	}
	teams, err := s.repo.TeamsOfUser(ctx, userID)
	warnDB(ctx, err)
	for _, t := range teams {
		v.teams[t] = true
	}
	return v, nil
}

// require loads the actor and checks one permission.
func (s *Service) require(ctx context.Context, userID uint, p enums.Permission, msg string) (*models.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// Everything in the module starts from using WhatsApp at all; the
	// panel guards its WhatsApp pages the same way.
	if !u.Can(enums.WAView) {
		return nil, errs.Forbidden("WhatsApp'ı kullanma yetkin yok.")
	}
	if !u.Can(p) {
		return nil, errs.Forbidden(msg)
	}
	return u, nil
}

// audience lists who sees a ticket right now.
func (s *Service) audience(ctx context.Context, t *models.WATicket) []uint {
	viewers, err := s.loadViewers(ctx)
	if err != nil || t == nil {
		return nil
	}
	parts := s.repo.Participants(ctx, t.ID)
	out := make([]uint, 0, len(viewers))
	for id, v := range viewers {
		if v.seesTicket(t, parts) {
			out = append(out, id)
		}
	}
	return out
}

// managers lists who is alerted about a device's problems.
func (s *Service) managers(ctx context.Context, channelID uint) []uint {
	viewers, err := s.loadViewers(ctx)
	if err != nil {
		return nil
	}
	var out []uint
	for id, v := range viewers {
		if v.can(enums.WAChannelManage) || (v.can(enums.WAViewAll) && v.seesChannel(channelID)) {
			out = append(out, id)
		}
	}
	return out
}

// ---------------------------------------------------------------- events

// Event is what the panel receives on the live stream for this module.
type Event struct {
	Type           string `json:"type"`
	ConversationID uint   `json:"conversationId,omitempty"`
	Conversation   any    `json:"conversation,omitempty"`
	Message        any    `json:"message,omitempty"`
	UserID         uint   `json:"userId,omitempty"`
	Name           string `json:"name,omitempty"`
	Text           string `json:"text,omitempty"`
	Level          string `json:"level,omitempty"`
}

// bump moves a conversation's version forward so reconnecting panels see
// the change.
func (s *Service) bump(ctx context.Context, conversationID uint) {
	warnDB(ctx, s.repo.BumpConversation(ctx, conversationID))
}

// publish sends a conversation's fresh summary (and optionally a message)
// to everyone who sees it, and tells those who lost sight of it.
func (s *Service) publish(ctx context.Context, conversationID uint, msg *models.WAMessage, before []uint) {
	s.bump(ctx, conversationID)
	conv, ticket, err := s.repo.Conversation(ctx, conversationID)
	if err != nil {
		slog.WarnContext(ctx, "whatsapp publish failed", "conversation", conversationID, "error", err)
		return
	}
	now := s.audience(ctx, ticket)
	if ticket == nil {
		now = nil
	}
	in := make(map[uint]bool, len(now))
	for _, id := range now {
		in[id] = true
	}
	var gone []uint
	for _, id := range before {
		if !in[id] {
			gone = append(gone, id)
		}
	}
	if len(gone) > 0 {
		s.push.Push(gone, Event{Type: "wa.gone", ConversationID: conversationID})
	}
	if len(now) == 0 {
		return
	}
	views, err := s.summaries(ctx, []models.WAConversation{*conv})
	if err != nil || len(views) == 0 {
		return
	}
	ev := Event{Type: "wa.conv", ConversationID: conversationID, Conversation: views[0]}
	if msg != nil {
		mv, err := s.messageViews(ctx, []models.WAMessage{*msg})
		if err == nil && len(mv) > 0 {
			ev.Message = mv[0]
		}
	}
	s.push.Push(now, ev)
}

// alert tells a device's managers about a problem.
func (s *Service) alert(ctx context.Context, channelID uint, text string) {
	ids := s.managers(ctx, channelID)
	if len(ids) == 0 {
		return
	}
	s.push.Push(ids, Event{Type: "wa.alert", Text: text, Level: "warning"})
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func strPtr(s string) *string { return &s }

func uintPtr(u uint) *uint { return &u }

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}
