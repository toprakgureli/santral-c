package whatsapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
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

// Service is the WhatsApp module.
type Service struct {
	db      *gorm.DB
	users   IUsers
	push    IPusher
	storage IStorage
	secret  string

	wakeWebhook chan struct{}
	wakeOutbox  chan struct{}

	mu        sync.Mutex
	viewers   map[uint]*viewer
	viewersAt time.Time
	folders   map[uint]string
	// uploaded files already on Meta, by device and file
	metaFiles map[string]metaFile
}

// NewService builds the module. secret encrypts tokens at rest.
func NewService(db *gorm.DB, users IUsers, push IPusher, storage IStorage, secret string) *Service {
	return &Service{
		db:          db,
		users:       users,
		push:        push,
		storage:     storage,
		secret:      "wa:" + secret,
		wakeWebhook: make(chan struct{}, 1),
		wakeOutbox:  make(chan struct{}, 1),
		folders:     map[uint]string{},
		metaFiles:   map[string]metaFile{},
	}
}

// Start runs the background workers until ctx ends.
func (s *Service) Start(ctx context.Context) {
	go s.webhookWorker(ctx)
	go s.outboxWorker(ctx)
	go s.clock(ctx)
}

func wake(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// ---------------------------------------------------------------- secrets

func (s *Service) seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return crypt.Encrypt(s.secret, plain)
}

func (s *Service) open(enc string) string {
	if enc == "" {
		return ""
	}
	out, err := crypt.Decrypt(s.secret, enc)
	if err != nil {
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

// loadViewers reads everyone who may use the module.
func (s *Service) loadViewers(ctx context.Context) (map[uint]*viewer, error) {
	s.mu.Lock()
	if s.viewers != nil && time.Since(s.viewersAt) < 30*time.Second {
		v := s.viewers
		s.mu.Unlock()
		return v, nil
	}
	s.mu.Unlock()

	var ids []uint
	if err := s.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ur.user_id FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		JOIN users u ON u.id = ur.user_id
		WHERE p.key = ? AND u.active`, string(enums.WAView)).Scan(&ids).Error; err != nil {
		return nil, err
	}
	var members []struct {
		ChannelID uint
		UserID    uint
	}
	if err := s.db.WithContext(ctx).Raw("SELECT channel_id, user_id FROM wa_channel_members").Scan(&members).Error; err != nil {
		return nil, err
	}
	var teams []struct {
		TeamID uint
		UserID uint
	}
	if err := s.db.WithContext(ctx).Raw("SELECT team_id, user_id FROM wa_team_members").Scan(&teams).Error; err != nil {
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
		return nil, errs.Forbidden("WhatsApp'ı kullanma yetkiniz yok.")
	}
	v := &viewer{user: u, channels: map[uint]bool{}, teams: map[uint]bool{}}
	var chans []uint
	_ = s.db.WithContext(ctx).Raw("SELECT channel_id FROM wa_channel_members WHERE user_id = ?", userID).Scan(&chans).Error
	for _, c := range chans {
		v.channels[c] = true
	}
	var teams []uint
	_ = s.db.WithContext(ctx).Raw("SELECT team_id FROM wa_team_members WHERE user_id = ?", userID).Scan(&teams).Error
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
	if !u.Can(p) {
		return nil, errs.Forbidden(msg)
	}
	return u, nil
}

func (s *Service) participantSet(ctx context.Context, ticketID uint) map[uint]bool {
	var ids []uint
	_ = s.db.WithContext(ctx).Raw("SELECT user_id FROM wa_ticket_participants WHERE ticket_id = ?", ticketID).Scan(&ids).Error
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// audience lists who sees a ticket right now.
func (s *Service) audience(ctx context.Context, t *models.WATicket) []uint {
	viewers, err := s.loadViewers(ctx)
	if err != nil || t == nil {
		return nil
	}
	parts := s.participantSet(ctx, t.ID)
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
	_ = s.db.WithContext(ctx).Exec("UPDATE wa_conversations SET version = nextval('wa_version_seq') WHERE id = ?", conversationID).Error
}

// publish sends a conversation's fresh summary (and optionally a message)
// to everyone who sees it, and tells those who lost sight of it.
func (s *Service) publish(ctx context.Context, conversationID uint, msg *models.WAMessage, before []uint) {
	s.bump(ctx, conversationID)
	conv, ticket, err := s.loadConv(ctx, conversationID)
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

func (s *Service) loadConv(ctx context.Context, id uint) (*models.WAConversation, *models.WATicket, error) {
	var c models.WAConversation
	if err := s.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, nil, err
	}
	var t *models.WATicket
	if c.TicketID != nil {
		var tt models.WATicket
		if err := s.db.WithContext(ctx).First(&tt, *c.TicketID).Error; err == nil {
			t = &tt
		}
	}
	return &c, t, nil
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func strPtr(s string) *string { return &s }

func uintPtr(u uint) *uint { return &u }

func now() time.Time { return time.Now() }
