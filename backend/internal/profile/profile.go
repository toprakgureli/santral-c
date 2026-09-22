// Package profile serves a person's page: who they are (photo, headline,
// biography, roles, extension) and their call-centre record (today and this
// month). Anyone signed in may look at anyone's profile; only the owner
// edits their own text.
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

// Stats is the call-centre record shown on the profile.
type Stats struct {
	TodayReal        int64 `json:"todayReal"`
	TodayUnanswered  int64 `json:"todayUnanswered"`
	MonthReal        int64 `json:"monthReal"`
	MonthTalkSeconds int64 `json:"monthTalkSeconds"`
	MonthEscalations int64 `json:"monthEscalations"`
	TotalEscalations int64 `json:"totalEscalations"`
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

// Stats computes the record from the panel's own call log and escalations.
func (r *Repository) Stats(ctx context.Context, id uint) (Stats, error) {
	now := time.Now().In(istanbul)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, istanbul)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, istanbul)
	var s Stats
	type row struct {
		Real       int64
		Unanswered int64
		Talk       int64
	}
	calls := func(from time.Time) (row, error) {
		var out row
		err := r.db.WithContext(ctx).Model(&models.CallLog{}).
			Select(
				"count(*) FILTER (WHERE disposition = 'answered' AND duration_seconds >= ?) AS real, "+
					"count(*) FILTER (WHERE disposition NOT IN ('answered', 'in_progress') AND NOT ("+calllog.NotMineSQL+")) AS unanswered, "+
					"COALESCE(SUM(duration_seconds) FILTER (WHERE disposition = 'answered'), 0) AS talk",
				realCallSeconds).
			Where("user_id = ? AND started_at >= ?", id, from).
			Scan(&out).Error
		if err != nil {
			return out, fmt.Errorf("call figures could not be computed: %w", err)
		}
		return out, nil
	}
	t, err := calls(today)
	if err != nil {
		return s, err
	}
	m, err := calls(month)
	if err != nil {
		return s, err
	}
	s.TodayReal, s.TodayUnanswered = t.Real, t.Unanswered
	s.MonthReal, s.MonthTalkSeconds = m.Real, m.Talk
	if err := r.db.WithContext(ctx).Model(&models.CallEscalation{}).
		Where("agent_id = ? AND created_at >= ?", id, month).Count(&s.MonthEscalations).Error; err != nil {
		return s, fmt.Errorf("escalation count could not be computed: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&models.CallEscalation{}).
		Where("agent_id = ?", id).Count(&s.TotalEscalations).Error; err != nil {
		return s, fmt.Errorf("escalation count could not be computed: %w", err)
	}
	return s, nil
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
	group.Get("/:id", r.handler.ByID)
}

func actor(c *fiber.Ctx) (uint, error) {
	id, ok := c.Locals(middlewares.UserIDKey).(uint)
	if !ok {
		return 0, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return id, nil
}
