// Package custommenu manages admin-defined sidebar menu items that embed
// external web applications inside Santral-C or open them in a new tab.
package custommenu

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/whatsapp/outside"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Allowed icon keys that the frontend icon catalog renders.
var allowedIcons = map[string]bool{
	"globe":          true,
	"telegram":       true,
	"send":           true,
	"message-circle": true,
	"briefcase":      true,
	"folder-kanban":  true,
	"database":       true,
	"cloud":          true,
	"mail":           true,
	"calendar":       true,
	"bar-chart-3":    true,
	"shopping-bag":   true,
	"file-text":      true,
	"bot":            true,
	"wrench":         true,
	"link":           true,
	"sparkles":       true,
	"building-2":     true,
	"headset":        true,
	"phone":          true,
	"shield":         true,
	"terminal":       true,
	"book-open":      true,
	"credit-card":    true,
	"truck":          true,
	"layout-grid":    true,
}

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IAudit records administrative changes.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// Service manages custom menus.
type Service struct {
	repo       *Repository
	users      IActorResolver
	audit      IAudit
	httpClient *http.Client
}

// NewService builds a custommenu service.
func NewService(repo *Repository, users IActorResolver, auditor IAudit) *Service {
	return &Service{
		repo:       repo,
		users:      users,
		audit:      auditor,
		httpClient: outside.Client,
	}
}

// RoleRef is a role summary attached to a menu item.
type RoleRef struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// Item is the JSON representation of a custom menu entry.
type Item struct {
	ID        uint      `json:"id"`
	Label     string    `json:"label"`
	Icon      string    `json:"icon"`
	URL       string    `json:"url"`
	OpenMode  string    `json:"openMode"`
	Active    bool      `json:"active"`
	SortOrder int       `json:"sortOrder"`
	RoleIDs   []uint    `json:"roleIds"`
	Roles     []RoleRef `json:"roles"`
}

// Input is the payload for creating or updating a custom menu entry.
type Input struct {
	Label    string `json:"label"`
	Icon     string `json:"icon"`
	URL      string `json:"url"`
	OpenMode string `json:"openMode"`
	Active   *bool  `json:"active"`
	RoleIDs  []uint `json:"roleIds"`
}

// EmbedCheck reports whether a target URL's response headers permit embedding
// inside an iframe.
type EmbedCheck struct {
	Embeddable     bool   `json:"embeddable"`
	Reason         string `json:"reason,omitempty"`
	XFrameOptions  string `json:"xFrameOptions,omitempty"`
	FrameAncestors string `json:"frameAncestors,omitempty"`
}

// Visible returns the active custom menu items that actorID is permitted to see.
func (s *Service) Visible(ctx context.Context, actorID uint) ([]Item, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.List(ctx, true)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]Item, 0, len(list))
	for i := range list {
		if maySee(actor, &list[i]) {
			out = append(out, toItem(&list[i]))
		}
	}
	return out, nil
}

// List returns all custom menu items (active and inactive) for administrators.
func (s *Service) List(ctx context.Context, actorID uint) ([]Item, error) {
	if _, err := s.requireManage(ctx, actorID); err != nil {
		return nil, err
	}
	list, err := s.repo.List(ctx, false)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]Item, 0, len(list))
	for i := range list {
		out = append(out, toItem(&list[i]))
	}
	return out, nil
}

// Roles returns all selectable roles for the menu permission picker.
func (s *Service) Roles(ctx context.Context, actorID uint) ([]RoleRef, error) {
	if _, err := s.requireManage(ctx, actorID); err != nil {
		return nil, err
	}
	roles, err := s.repo.AllRoles(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]RoleRef, 0, len(roles))
	for _, r := range roles {
		out = append(out, RoleRef{ID: r.ID, Name: r.Name, DisplayName: r.DisplayName})
	}
	return out, nil
}

// Create validates and stores a new custom menu entry.
func (s *Service) Create(ctx context.Context, actorID uint, in Input, ip string) (*Item, error) {
	if _, err := s.requireManage(ctx, actorID); err != nil {
		return nil, err
	}
	label, icon, targetURL, openMode, active, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	roles, err := s.repo.RolesByIDs(ctx, in.RoleIDs)
	if err != nil {
		return nil, errs.Internal(err)
	}
	nextOrder, err := s.repo.NextSortOrder(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	m := &models.CustomMenu{
		Label:     label,
		Icon:      icon,
		TargetURL: targetURL,
		OpenMode:  openMode,
		Active:    active,
		SortOrder: nextOrder,
		CreatedBy: &actorID,
	}
	if err := s.repo.Create(ctx, m, roles); err != nil {
		return nil, errs.Internal(err)
	}
	if s.audit != nil {
		s.audit.Record(ctx, audit.Entry{
			ActorID:    &actorID,
			Action:     enums.AuditMenuCreated,
			TargetType: "custom_menu",
			TargetID:   strconv.FormatUint(uint64(m.ID), 10),
			IP:         ip,
			Detail:     map[string]any{"label": m.Label, "url": m.TargetURL, "active": m.Active},
		})
	}
	res := toItem(m)
	return &res, nil
}

// Update validates and updates an existing custom menu entry.
func (s *Service) Update(ctx context.Context, actorID, id uint, in Input, ip string) (*Item, error) {
	if _, err := s.requireManage(ctx, actorID); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if existing == nil {
		return nil, errs.NotFound("Menü bulunamadı.")
	}
	label, icon, targetURL, openMode, active, err := validateInput(in)
	if err != nil {
		return nil, err
	}
	roles, err := s.repo.RolesByIDs(ctx, in.RoleIDs)
	if err != nil {
		return nil, errs.Internal(err)
	}
	existing.Label = label
	existing.Icon = icon
	existing.TargetURL = targetURL
	existing.OpenMode = openMode
	existing.Active = active
	if err := s.repo.Update(ctx, existing, roles); err != nil {
		return nil, errs.Internal(err)
	}
	if s.audit != nil {
		s.audit.Record(ctx, audit.Entry{
			ActorID:    &actorID,
			Action:     enums.AuditMenuUpdated,
			TargetType: "custom_menu",
			TargetID:   strconv.FormatUint(uint64(id), 10),
			IP:         ip,
			Detail:     map[string]any{"label": existing.Label, "url": existing.TargetURL, "active": existing.Active},
		})
	}
	res := toItem(existing)
	return &res, nil
}

// Delete removes a custom menu entry.
func (s *Service) Delete(ctx context.Context, actorID, id uint, ip string) error {
	if _, err := s.requireManage(ctx, actorID); err != nil {
		return err
	}
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return errs.Internal(err)
	}
	if existing == nil {
		return errs.NotFound("Menü bulunamadı.")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return errs.Internal(err)
	}
	if s.audit != nil {
		s.audit.Record(ctx, audit.Entry{
			ActorID:    &actorID,
			Action:     enums.AuditMenuDeleted,
			TargetType: "custom_menu",
			TargetID:   strconv.FormatUint(uint64(id), 10),
			IP:         ip,
			Detail:     map[string]any{"label": existing.Label, "url": existing.TargetURL},
		})
	}
	return nil
}

// Reorder updates the display order of custom menu entries.
func (s *Service) Reorder(ctx context.Context, actorID uint, ids []uint) error {
	if _, err := s.requireManage(ctx, actorID); err != nil {
		return err
	}
	if len(ids) == 0 {
		return errs.Invalid("Sıralanacak menü yok.", nil)
	}
	if err := s.repo.Reorder(ctx, ids); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// Check inspects whether the target URL of menu id sets X-Frame-Options or
// Content-Security-Policy frame-ancestors that forbid embedding inside an iframe.
func (s *Service) Check(ctx context.Context, actorID, id uint) (*EmbedCheck, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if m == nil || (!m.Active && !actor.Can(enums.MenuManage)) {
		return nil, errs.NotFound("Menü bulunamadı.")
	}
	if !maySee(actor, m) && !actor.Can(enums.MenuManage) {
		return nil, errs.Forbidden("Bu menüyü görme yetkin yok.")
	}
	return s.ProbeURL(ctx, m.TargetURL), nil
}

// ProbeURL performs a short HTTP HEAD/GET request to inspect framing headers.
// If the remote server cannot be reached from the backend (e.g. an intranet tool
// reachable only from the user's browser), it returns Embeddable: true so the
// client iframe is not needlessly blocked.
func (s *Service) ProbeURL(ctx context.Context, rawURL string) *EmbedCheck {
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	client := s.httpClient
	if client == nil {
		client = outside.Client
	}
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return &EmbedCheck{Embeddable: true}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return &EmbedCheck{Embeddable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	return InspectFramingHeaders(resp.Header)
}

// InspectFramingHeaders evaluates X-Frame-Options and Content-Security-Policy
// headers to determine whether cross-origin iframe embedding is blocked.
func InspectFramingHeaders(h http.Header) *EmbedCheck {
	for _, raw := range h.Values("Content-Security-Policy") {
		if fa := extractFrameAncestors(raw); fa != "" {
			if !frameAncestorsAllowsAny(fa) {
				return &EmbedCheck{
					Embeddable:     false,
					Reason:         "Hedef web sitesi güvenlik politikası (CSP frame-ancestors) nedeniyle iframe içinde gömülmeyi engelliyor.",
					FrameAncestors: fa,
				}
			}
		}
	}
	xfo := strings.TrimSpace(h.Get("X-Frame-Options"))
	if xfo != "" {
		upper := strings.ToUpper(xfo)
		if strings.Contains(upper, "DENY") || strings.Contains(upper, "SAMEORIGIN") {
			return &EmbedCheck{
				Embeddable:    false,
				Reason:        "Hedef web sitesi (X-Frame-Options: " + xfo + ") nedeniyle iframe içinde açılmaya izin vermiyor.",
				XFrameOptions: xfo,
			}
		}
	}
	return &EmbedCheck{Embeddable: true}
}

func frameAncestorsAllowsAny(fa string) bool {
	for _, tok := range strings.Fields(strings.ToLower(fa)) {
		if tok == "'none'" {
			return false
		}
		if tok == "*" || tok == "https:" || tok == "http:" {
			return true
		}
	}
	return false
}

func extractFrameAncestors(csp string) string {
	for _, dir := range strings.Split(csp, ";") {
		dir = strings.TrimSpace(dir)
		lower := strings.ToLower(dir)
		if strings.HasPrefix(lower, "frame-ancestors") {
			rest := strings.TrimSpace(dir[len("frame-ancestors"):])
			return rest
		}
	}
	return ""
}

func (s *Service) requireManage(ctx context.Context, actorID uint) (*models.User, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.MenuManage) {
		return nil, errs.Forbidden("Özel menüleri yönetme yetkin yok.")
	}
	return actor, nil
}

func maySee(actor *models.User, m *models.CustomMenu) bool {
	if actor == nil {
		return false
	}
	if actor.IsInvisibleAdmin() || len(m.Roles) == 0 {
		return true
	}
	for _, mr := range m.Roles {
		for _, ur := range actor.Roles {
			if mr.ID == ur.ID {
				return true
			}
		}
	}
	return false
}

func validateInput(in Input) (label, icon, targetURL, openMode string, active bool, err error) {
	label = strings.TrimSpace(in.Label)
	if label == "" {
		return "", "", "", "", false, errs.Invalid("Menü adı zorunludur.", nil)
	}
	if utf8.RuneCountInString(label) > 120 {
		return "", "", "", "", false, errs.Invalid("Menü adı en fazla 120 karakter olabilir.", nil)
	}
	icon = strings.TrimSpace(in.Icon)
	if icon == "" {
		icon = "globe"
	}
	if !allowedIcons[icon] {
		return "", "", "", "", false, errs.Invalid("Geçersiz menü ikonu.", nil)
	}
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return "", "", "", "", false, errs.Invalid("Açılacak URL zorunludur.", nil)
	}
	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", "", "", false, errs.Invalid("URL http:// veya https:// ile başlamalıdır.", nil)
	}
	openMode = strings.TrimSpace(in.OpenMode)
	if openMode == "" {
		openMode = "iframe"
	}
	if openMode != "iframe" && openMode != "new_tab" {
		return "", "", "", "", false, errs.Invalid("Geçersiz açılış modu.", nil)
	}
	active = true
	if in.Active != nil {
		active = *in.Active
	}
	return label, icon, parsed.String(), openMode, active, nil
}

func toItem(m *models.CustomMenu) Item {
	roleIDs := make([]uint, 0, len(m.Roles))
	roles := make([]RoleRef, 0, len(m.Roles))
	for _, r := range m.Roles {
		if enums.Role(r.Name) == enums.RoleInvisibleAdmin {
			continue
		}
		roleIDs = append(roleIDs, r.ID)
		roles = append(roles, RoleRef{ID: r.ID, Name: r.Name, DisplayName: r.DisplayName})
	}
	return Item{
		ID:        m.ID,
		Label:     m.Label,
		Icon:      m.Icon,
		URL:       m.TargetURL,
		OpenMode:  m.OpenMode,
		Active:    m.Active,
		SortOrder: m.SortOrder,
		RoleIDs:   roleIDs,
		Roles:     roles,
	}
}
