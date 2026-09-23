package escalation

import (
	"context"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// historyLimit caps how many past escalations are returned for one number.
const historyLimit = 25

// Service is the escalation application service.
type Service struct {
	repo  *Repository
	users IActorResolver
	// OnLogged, when set, hears every recorded escalation (user, category).
	OnLogged func(userID uint, category string)
}

// NewService builds an escalation service.
func NewService(repo *Repository, users IActorResolver) *Service {
	return &Service{repo: repo, users: users}
}

// Category is the panel view of a category and its reasons.
type Category struct {
	ID      uint     `json:"id"`
	Name    string   `json:"name"`
	Reasons []Reason `json:"reasons"`
}

// Reason is the panel view of a reason.
type Reason struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// Record is the panel view of a logged escalation.
type Record struct {
	ID           uint   `json:"id"`
	Number       string `json:"number"`
	CategoryName string `json:"categoryName"`
	ReasonName   string `json:"reasonName"`
	Note         string `json:"note"`
	AgentID      uint   `json:"agentId,omitempty"`
	AgentName    string `json:"agentName"`
	CreatedAt    string `json:"createdAt"`
}

// ListQuery is what the list page asks for. Dates are local YYYY-MM-DD.
type ListQuery struct {
	Number     string
	AgentID    uint
	CategoryID uint
	From       string
	To         string
	Page       int
	PerPage    int
}

// ListPage is one page of the escalation list. Scope says whose records the
// page holds: "all" or "own".
type ListPage struct {
	Items   []Record `json:"items"`
	Total   int64    `json:"total"`
	Page    int      `json:"page"`
	PerPage int      `json:"perPage"`
	Scope   string   `json:"scope"`
}

// List returns escalations in pages. Who sees what:
//   - escalation.list_all: everyone's records, any filter.
//   - escalation.list_own: only the actor's own records.
//   - escalation.search with a number: everyone's records for that number
//     (the customer history lookup the search permission always allowed).
func (s *Service) List(ctx context.Context, actorID uint, q ListQuery) (*ListPage, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	f := ListFilter{Page: q.Page, PerPage: q.PerPage, AgentID: q.AgentID, CategoryID: q.CategoryID}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 100 {
		f.PerPage = 25
	}
	if n := strings.TrimSpace(q.Number); n != "" {
		f.NumberKey = phone.Key(n)
		if f.NumberKey == "" {
			return &ListPage{Items: []Record{}, Page: f.Page, PerPage: f.PerPage, Scope: "all"}, nil
		}
	}
	scope := ""
	switch {
	case actor.Can(enums.EscalationListAll):
		scope = "all"
	case f.NumberKey != "" && actor.Can(enums.EscalationSearch):
		scope = "all"
	case actor.Can(enums.EscalationListOwn):
		scope = "own"
		f.AgentID = actorID
	case actor.Can(enums.EscalationSearch):
		return nil, errs.Forbidden("Listeleme yetkiniz yok. Müşteri numarası girerek arayabilirsiniz.")
	default:
		return nil, errs.Forbidden("Eskalasyon kayıtlarını görme yetkiniz yok.")
	}
	if q.From != "" {
		t, err := time.ParseInLocation("2006-01-02", q.From, istanbul)
		if err != nil {
			return nil, errs.Invalid("Başlangıç tarihi geçersiz.", err)
		}
		f.From = t
	}
	if q.To != "" {
		t, err := time.ParseInLocation("2006-01-02", q.To, istanbul)
		if err != nil {
			return nil, errs.Invalid("Bitiş tarihi geçersiz.", err)
		}
		f.To = t.AddDate(0, 0, 1)
	}
	rows, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, errs.Internal(err)
	}
	items := make([]Record, 0, len(rows))
	for i := range rows {
		items = append(items, toRecord(&rows[i]))
	}
	return &ListPage{Items: items, Total: total, Page: f.Page, PerPage: f.PerPage, Scope: scope}, nil
}

// Agents lists the agents behind the records, for the list_all filter.
func (s *Service) Agents(ctx context.Context, actorID uint) ([]AgentRef, error) {
	if _, err := s.authorize(ctx, actorID, enums.EscalationListAll); err != nil {
		return nil, err
	}
	out, err := s.repo.Agents(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return out, nil
}

// Categories returns the full catalog for agents and admins.
func (s *Service) Categories(ctx context.Context, actorID uint) ([]Category, error) {
	if _, err := s.authorize(ctx, actorID, enums.EscalationView); err != nil {
		return nil, err
	}
	cats, err := s.repo.Categories(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]Category, 0, len(cats))
	for i := range cats {
		out = append(out, toCategory(&cats[i]))
	}
	return out, nil
}

// CreateCategory adds a category (admin).
func (s *Service) CreateCategory(ctx context.Context, actorID uint, req requests.EscalationCategoryCreate) (*Category, error) {
	if _, err := s.authorize(ctx, actorID, enums.EscalationManage); err != nil {
		return nil, err
	}
	c := &models.EscalationCategory{Name: strings.TrimSpace(req.Name), CreatedBy: &actorID}
	if err := s.repo.CreateCategory(ctx, c); err != nil {
		return nil, errs.Conflict("Bu kategori zaten var.", err)
	}
	res := toCategory(c)
	return &res, nil
}

// ReorderCategories saves the manager's category order.
func (s *Service) ReorderCategories(ctx context.Context, actorID uint, ids []uint) error {
	if _, err := s.authorize(ctx, actorID, enums.EscalationManage); err != nil {
		return err
	}
	if len(ids) == 0 {
		return errs.Invalid("Sıralanacak kategori yok.", nil)
	}
	if err := s.repo.ReorderCategories(ctx, ids); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// ReorderReasons saves the manager's reason order inside a category.
func (s *Service) ReorderReasons(ctx context.Context, actorID, categoryID uint, ids []uint) error {
	if _, err := s.authorize(ctx, actorID, enums.EscalationManage); err != nil {
		return err
	}
	if len(ids) == 0 {
		return errs.Invalid("Sıralanacak durum yok.", nil)
	}
	if err := s.repo.ReorderReasons(ctx, categoryID, ids); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// DeleteCategory removes a category and its reasons (admin).
func (s *Service) DeleteCategory(ctx context.Context, actorID, id uint) error {
	if _, err := s.authorize(ctx, actorID, enums.EscalationManage); err != nil {
		return err
	}
	if err := s.repo.DeleteCategory(ctx, id); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// CreateReason adds a reason under a category (admin).
func (s *Service) CreateReason(ctx context.Context, actorID, categoryID uint, req requests.EscalationReasonCreate) (*Reason, error) {
	if _, err := s.authorize(ctx, actorID, enums.EscalationManage); err != nil {
		return nil, err
	}
	cat, err := s.repo.GetCategory(ctx, categoryID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if cat == nil {
		return nil, errs.NotFound("Kategori bulunamadı.")
	}
	reason := &models.EscalationReason{CategoryID: categoryID, Name: strings.TrimSpace(req.Name)}
	if err := s.repo.CreateReason(ctx, reason); err != nil {
		return nil, errs.Conflict("Bu durum zaten var.", err)
	}
	return &Reason{ID: reason.ID, Name: reason.Name}, nil
}

// DeleteReason removes a reason (admin).
func (s *Service) DeleteReason(ctx context.Context, actorID, id uint) error {
	if _, err := s.authorize(ctx, actorID, enums.EscalationManage); err != nil {
		return err
	}
	if err := s.repo.DeleteReason(ctx, id); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// Import parses an uploaded spreadsheet of category/reason pairs and adds any
// that are missing (admin).
func (s *Service) Import(ctx context.Context, actorID uint, filename string, data []byte) (int, int, error) {
	if _, err := s.authorize(ctx, actorID, enums.EscalationManage); err != nil {
		return 0, 0, err
	}
	catalog, err := parseCatalog(filename, data)
	if err != nil {
		return 0, 0, errs.Invalid("Dosya okunamadı. İlk sütun kategori, ikinci sütun durum olmalı.", err)
	}
	if len(catalog) == 0 {
		return 0, 0, errs.Invalid("Dosyada işlenecek kategori/durum bulunamadı.", nil)
	}
	cats, reasons, err := s.repo.UpsertCatalog(ctx, actorID, catalog)
	if err != nil {
		return 0, 0, errs.Internal(err)
	}
	return cats, reasons, nil
}

// Log records an escalation an agent selected during a call.
func (s *Service) Log(ctx context.Context, actorID uint, req requests.EscalationCreate) (*Record, error) {
	actor, err := s.authorize(ctx, actorID, enums.EscalationView)
	if err != nil {
		return nil, err
	}
	reason, cat, err := s.repo.GetReason(ctx, req.ReasonID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if reason == nil || cat == nil {
		return nil, errs.NotFound("Seçilen durum bulunamadı.")
	}
	key := phone.Key(req.Number)
	if key == "" {
		return nil, errs.Invalid("Geçersiz numara.", nil)
	}
	e := &models.CallEscalation{
		NumberKey:    key,
		Number:       strings.TrimSpace(req.Number),
		CategoryID:   &cat.ID,
		ReasonID:     &reason.ID,
		CategoryName: cat.Name,
		ReasonName:   reason.Name,
		Note:         strings.TrimSpace(req.Note),
		AgentID:      &actorID,
		AgentName:    actor.Name,
	}
	if req.CallUUID != "" {
		uuid := req.CallUUID
		e.CallUUID = &uuid
	}
	if err := s.repo.CreateEscalation(ctx, e); err != nil {
		return nil, errs.Internal(err)
	}
	if s.OnLogged != nil {
		go s.OnLogged(actorID, cat.Name)
	}
	res := toRecord(e)
	return &res, nil
}

// istanbul is the panel's display zone; stored stamps are UTC.
var istanbul = time.FixedZone("+03", 3*3600)

// Labels of the record written when an agent marks a call as needing no
// escalation. They live on the record itself, not in the catalog.
const (
	NoneCategoryName = "Eskalasyon yok"
	NoneReasonName   = "Eskalasyon gerekli değil"
)

// LogNone records the agent's decision that the call needed no escalation,
// under the actor's own name, so the wrap-up leaves a trace either way.
func (s *Service) LogNone(ctx context.Context, actorID uint, req requests.EscalationNone) (*Record, error) {
	actor, err := s.authorize(ctx, actorID, enums.EscalationView)
	if err != nil {
		return nil, err
	}
	key := phone.Key(req.Number)
	if key == "" {
		return nil, errs.Invalid("Geçersiz numara.", nil)
	}
	e := &models.CallEscalation{
		NumberKey:    key,
		Number:       strings.TrimSpace(req.Number),
		CategoryName: NoneCategoryName,
		ReasonName:   NoneReasonName,
		Note:         actor.Name + " bu müşteriyi eskalasyon gerekli değil olarak işaretledi.",
		AgentID:      &actorID,
		AgentName:    actor.Name,
	}
	if req.CallUUID != "" {
		uuid := req.CallUUID
		e.CallUUID = &uuid
	}
	if err := s.repo.CreateEscalation(ctx, e); err != nil {
		return nil, errs.Internal(err)
	}
	res := toRecord(e)
	return &res, nil
}

// History returns past escalations for a customer number. Agents see it for
// the caller on the line (escalation.view); the standalone search page needs
// escalation.search.
func (s *Service) History(ctx context.Context, actorID uint, number string) ([]Record, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.EscalationView) && !actor.Can(enums.EscalationSearch) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	key := phone.Key(number)
	if key == "" {
		return []Record{}, nil
	}
	rows, err := s.repo.EscalationsByNumberKey(ctx, key, historyLimit)
	if err != nil {
		return nil, errs.Internal(err)
	}
	out := make([]Record, 0, len(rows))
	for i := range rows {
		out = append(out, toRecord(&rows[i]))
	}
	return out, nil
}

func (s *Service) authorize(ctx context.Context, actorID uint, perm enums.Permission) (*models.User, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(perm) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	return actor, nil
}

func toCategory(c *models.EscalationCategory) Category {
	reasons := make([]Reason, 0, len(c.Reasons))
	for _, r := range c.Reasons {
		reasons = append(reasons, Reason{ID: r.ID, Name: r.Name})
	}
	return Category{ID: c.ID, Name: c.Name, Reasons: reasons}
}

func toRecord(e *models.CallEscalation) Record {
	rec := Record{
		ID:           e.ID,
		Number:       e.Number,
		CategoryName: e.CategoryName,
		ReasonName:   e.ReasonName,
		Note:         e.Note,
		AgentName:    e.AgentName,
		CreatedAt:    e.CreatedAt.In(istanbul).Format("02.01.2006 15:04"),
	}
	if e.AgentID != nil {
		rec.AgentID = *e.AgentID
	}
	return rec
}
