// Package profile serves a person's page: who they are (photo, headline,
// biography, roles, extension), their all-time totals, and a call-centre
// record over any day range. Anyone signed in may look at anyone's profile;
// only the owner edits their own text.
package profile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/calllog"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/middlewares"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/validator"
)

var istanbul = time.FixedZone("+03", 3*3600)

// A connected call of 30 seconds or more is a real conversation (the same
// rule as the team page).
const realCallSeconds = 30

// Stats is the all-time line on the profile.
type Stats struct {
	TotalReal        int64 `json:"totalReal"`
	TotalTalkSeconds int64 `json:"totalTalkSeconds"`
	TotalEscalations int64 `json:"totalEscalations"`
}

// Day is one day's real-call count in the record's series.
type Day struct {
	Day  string `json:"day"` // local YYYY-MM-DD
	Real int64  `json:"real"`
}

// Record is the call-centre record over a day range.
type Record struct {
	From           string `json:"from"`
	To             string `json:"to"`
	Real           int64  `json:"real"`
	InboundReal    int64  `json:"inboundReal"`
	OutboundReal   int64  `json:"outboundReal"`
	Unanswered     int64  `json:"unanswered"`
	Short          int64  `json:"short"`
	TalkSeconds    int64  `json:"talkSeconds"`
	AvgTalkSeconds int64  `json:"avgTalkSeconds"`
	LongestSeconds int64  `json:"longestSeconds"`
	Escalations    int64  `json:"escalations"`
	ShiftSeconds   int64  `json:"shiftSeconds"`
	BreakSeconds   int64  `json:"breakSeconds"`
	// BusiestHour is the local hour with the most real calls, -1 when none.
	BusiestHour int   `json:"busiestHour"`
	Days        []Day `json:"days"`
}

// Profile is the page payload.
type Profile struct {
	ID            uint     `json:"id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	Headline      string   `json:"headline"`
	Bio           string   `json:"bio"`
	HasAvatar     bool     `json:"hasAvatar"`
	AvatarVersion int64    `json:"avatarVersion,omitempty"`
	Roles         []string `json:"roles"`
	Extension     string   `json:"extension,omitempty"`
	Active        bool     `json:"active"`
	JoinedAt      string   `json:"joinedAt"`
	Stats         Stats    `json:"stats"`
	Editable      bool     `json:"editable"`
}

// Update is the owner's edit of the profile text.
type Update struct {
	Headline string `json:"headline" validate:"max=120"`
	Bio      string `json:"bio" validate:"max=2000"`
}

// Repository reads users and their figures.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a profile repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// User loads a user with roles, or nil.
func (r *Repository) User(ctx context.Context, id uint) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).Preload("Roles").First(&u, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user could not be loaded: %w", err)
	}
	return &u, nil
}

// SetText stores the headline and biography.
func (r *Repository) SetText(ctx context.Context, id uint, headline, bio string) error {
	if err := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).
		Updates(map[string]any{"headline": headline, "bio": bio}).Error; err != nil {
		return fmt.Errorf("profile text could not be saved: %w", err)
	}
	return nil
}

// Stats computes the all-time totals.
func (r *Repository) Stats(ctx context.Context, id uint) (Stats, error) {
	var s Stats
	var calls struct {
		Real int64
		Talk int64
	}
	if err := r.db.WithContext(ctx).Model(&models.CallLog{}).
		Select("count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds >= ?) AS real, "+
			"COALESCE(SUM(duration_seconds) FILTER (WHERE disposition = 'answered'), 0) AS talk", realCallSeconds).
		Where("user_id = ?", id).Scan(&calls).Error; err != nil {
		return s, fmt.Errorf("call totals could not be computed: %w", err)
	}
	s.TotalReal, s.TotalTalkSeconds = calls.Real, calls.Talk
	if err := r.db.WithContext(ctx).Model(&models.CallEscalation{}).
		Where("agent_id = ?", id).Count(&s.TotalEscalations).Error; err != nil {
		return s, fmt.Errorf("escalation count could not be computed: %w", err)
	}
	return s, nil
}

// Record computes the call-centre record for [from, to).
func (r *Repository) Record(ctx context.Context, id uint, from, to time.Time) (Record, error) {
	rec := Record{BusiestHour: -1, Days: []Day{}}
	var calls struct {
		Real         int64
		InboundReal  int64
		OutboundReal int64
		Unanswered   int64
		Short        int64
		Talk         int64
		Avg          int64
		Longest      int64
	}
	if err := r.db.WithContext(ctx).Model(&models.CallLog{}).
		Select(
			"count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds >= ?) AS real, "+
				"count(*) FILTER (WHERE direction = 'inbound' AND disposition = 'answered' AND duration_seconds >= ?) AS inbound_real, "+
				"count(*) FILTER (WHERE direction = 'outbound' AND disposition = 'answered' AND duration_seconds >= ?) AS outbound_real, "+
				"count(*) FILTER (WHERE disposition NOT IN ('answered', 'in_progress') AND NOT ("+calllog.NotMineSQL+")) AS unanswered, "+
				"count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds < ?) AS short, "+
				"COALESCE(SUM(duration_seconds) FILTER (WHERE disposition = 'answered'), 0) AS talk, "+
				"COALESCE(AVG(duration_seconds) FILTER (WHERE disposition = 'answered' AND duration_seconds >= ?), 0)::bigint AS avg, "+
				"COALESCE(MAX(duration_seconds) FILTER (WHERE disposition = 'answered'), 0) AS longest",
			realCallSeconds, realCallSeconds, realCallSeconds, realCallSeconds, realCallSeconds).
		Where("user_id = ? AND started_at >= ? AND started_at < ?", id, from, to).
		Scan(&calls).Error; err != nil {
		return rec, fmt.Errorf("call figures could not be computed: %w", err)
	}
	rec.Real, rec.InboundReal, rec.OutboundReal = calls.Real, calls.InboundReal, calls.OutboundReal
	rec.Unanswered, rec.Short = calls.Unanswered, calls.Short
	rec.TalkSeconds, rec.AvgTalkSeconds, rec.LongestSeconds = calls.Talk, calls.Avg, calls.Longest

	if err := r.db.WithContext(ctx).Model(&models.CallEscalation{}).
		Where("agent_id = ? AND created_at >= ? AND created_at < ?", id, from, to).Count(&rec.Escalations).Error; err != nil {
		return rec, fmt.Errorf("escalation count could not be computed: %w", err)
	}

	// Shift and break time clipped to the window; open stretches count to now.
	if err := r.db.WithContext(ctx).Model(&models.Shift{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM (LEAST(COALESCE(ended_at, now()), ?) - GREATEST(started_at, ?)))), 0)::bigint", to, from).
		Where("user_id = ? AND started_at < ? AND COALESCE(ended_at, now()) > ?", id, to, from).
		Scan(&rec.ShiftSeconds).Error; err != nil {
		return rec, fmt.Errorf("shift time could not be computed: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&models.PresenceEvent{}).
		Select("COALESCE(SUM(EXTRACT(EPOCH FROM (LEAST(COALESCE(ended_at, now()), ?) - GREATEST(started_at, ?)))), 0)::bigint", to, from).
		Where("user_id = ? AND state = 'break' AND started_at < ? AND COALESCE(ended_at, now()) > ?", id, to, from).
		Scan(&rec.BreakSeconds).Error; err != nil {
		return rec, fmt.Errorf("break time could not be computed: %w", err)
	}

	// Real calls per local day, and the busiest local hour.
	var days []struct {
		Day  string
		Real int64
	}
	if err := r.db.WithContext(ctx).Model(&models.CallLog{}).
		Select("to_char(started_at AT TIME ZONE 'Europe/Istanbul', 'YYYY-MM-DD') AS day, count(*) AS real").
		Where("user_id = ? AND started_at >= ? AND started_at < ? AND disposition = 'answered' AND duration_seconds >= ?", id, from, to, realCallSeconds).
		Group("day").Order("day").Scan(&days).Error; err != nil {
		return rec, fmt.Errorf("daily figures could not be computed: %w", err)
	}
	byDay := map[string]int64{}
	for _, d := range days {
		byDay[d.Day] = d.Real
	}
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		rec.Days = append(rec.Days, Day{Day: key, Real: byDay[key]})
	}
	var hour struct {
		Hour int
		N    int64
	}
	if err := r.db.WithContext(ctx).Model(&models.CallLog{}).
		Select("EXTRACT(HOUR FROM started_at AT TIME ZONE 'Europe/Istanbul')::int AS hour, count(*) AS n").
		Where("user_id = ? AND started_at >= ? AND started_at < ? AND disposition = 'answered' AND duration_seconds >= ?", id, from, to, realCallSeconds).
		Group("hour").Order("n DESC, hour").Limit(1).Scan(&hour).Error; err != nil {
		return rec, fmt.Errorf("busiest hour could not be computed: %w", err)
	}
	if hour.N > 0 {
		rec.BusiestHour = hour.Hour
	}
	return rec, nil
}

// Service builds and edits profiles.
type Service struct {
	repo *Repository
}

// NewService builds a profile service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Get returns a user's profile as seen by the actor.
func (s *Service) Get(ctx context.Context, actorID, userID uint) (*Profile, error) {
	u, err := s.repo.User(ctx, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if u == nil {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	stats, err := s.repo.Stats(ctx, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return build(u, stats, actorID == userID), nil
}

// RecordRange returns a user's record over an inclusive local day range.
func (s *Service) RecordRange(ctx context.Context, userID uint, fromDay, toDay string) (*Record, error) {
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
	u, err := s.repo.User(ctx, userID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if u == nil {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	rec, err := s.repo.Record(ctx, userID, from, toStart.AddDate(0, 0, 1))
	if err != nil {
		return nil, errs.Internal(err)
	}
	rec.From, rec.To = fromDay, toDay
	return &rec, nil
}

// UpdateMine stores the actor's headline and biography.
func (s *Service) UpdateMine(ctx context.Context, actorID uint, req Update) (*Profile, error) {
	if err := s.repo.SetText(ctx, actorID, strings.TrimSpace(req.Headline), strings.TrimSpace(req.Bio)); err != nil {
		return nil, errs.Internal(err)
	}
	return s.Get(ctx, actorID, actorID)
}

func build(u *models.User, stats Stats, editable bool) *Profile {
	roles := make([]string, 0, len(u.Roles))
	for _, r := range u.Roles {
		roles = append(roles, r.DisplayName)
	}
	p := &Profile{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		Headline:  u.Headline,
		Bio:       u.Bio,
		HasAvatar: u.Avatar != "",
		Roles:     roles,
		Active:    u.Active,
		JoinedAt:  u.CreatedAt.In(istanbul).Format("2006-01-02"),
		Stats:     stats,
		Editable:  editable,
	}
	if u.Avatar != "" {
		p.AvatarVersion = u.UpdatedAt.Unix()
	}
	if u.SIPExtension != nil {
		p.Extension = *u.SIPExtension
	}
	return p
}

// Handler serves the profile endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds a profile handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Mine returns the caller's profile.
func (h *Handler) Mine(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	res, err := h.service.Get(c.UserContext(), id, id)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// ByID returns another user's profile.
func (h *Handler) ByID(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	target, err := c.ParamsInt("id")
	if err != nil || target < 1 {
		return errs.Invalid("Geçersiz kullanıcı numarası.", err)
	}
	res, err := h.service.Get(c.UserContext(), id, uint(target))
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Record returns a user's record for the from/to query days (local,
// inclusive); "me" stands for the caller.
func (h *Handler) Record(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	target := id
	if c.Params("id") != "me" {
		n, err := c.ParamsInt("id")
		if err != nil || n < 1 {
			return errs.Invalid("Geçersiz kullanıcı numarası.", err)
		}
		target = uint(n)
	}
	from, to := c.Query("from"), c.Query("to")
	if from == "" {
		from = to
	}
	if to == "" {
		to = from
	}
	if from == "" {
		today := time.Now().In(istanbul)
		to = today.Format("2006-01-02")
		from = today.AddDate(0, 0, -6).Format("2006-01-02")
	}
	res, err := h.service.RecordRange(c.UserContext(), target, from, to)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// UpdateMine stores the caller's headline and biography.
func (h *Handler) UpdateMine(c *fiber.Ctx) error {
	id, err := actor(c)
	if err != nil {
		return err
	}
	var req Update
	if err := c.BodyParser(&req); err != nil {
		return errs.Invalid("İstek gövdesi okunamadı.", err)
	}
	if err := validator.Struct(req); err != nil {
		return err
	}
	res, err := h.service.UpdateMine(c.UserContext(), id, req)
	if err != nil {
		return err
	}
	return c.JSON(res)
}

// Router mounts the profile endpoints.
type Router struct {
	handler *Handler
	guard   fiber.Handler
}

// NewRouter builds a profile router.
func NewRouter(handler *Handler, guard fiber.Handler) *Router {
	return &Router{handler: handler, guard: guard}
}

// Routes registers the profile routes onto g.
func (r *Router) Routes(g fiber.Router) {
	group := g.Group("/profile", r.guard)
	group.Get("/me", r.handler.Mine)
	group.Put("/me", r.handler.UpdateMine)
	group.Get("/:id/record", r.handler.Record)
	group.Get("/:id", r.handler.ByID)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}
