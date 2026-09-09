package contact

import (
	"context"
	"strconv"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/phone"
)

// Meta carries request context for audit trails.
type Meta struct {
	IP string
}

// Service is the contact application service.
type Service struct {
	repo  *Repository
	users IActorResolver
	audit IAudit
}

// NewService builds a contact service.
func NewService(repo *Repository, users IActorResolver, auditor IAudit) *Service {
	return &Service{repo: repo, users: users, audit: auditor}
}

// Create stores a new contact and its phone numbers.
func (s *Service) Create(ctx context.Context, actorID uint, req requests.ContactCreate, meta Meta) (*responses.Contact, error) {
	if _, err := s.authorize(ctx, actorID, enums.ContactManage); err != nil {
		return nil, err
	}

	phones, err := s.buildPhones(ctx, req.Phones)
	if err != nil {
		return nil, err
	}

	c := &models.Contact{
		Name:      req.Name,
		Company:   req.Company,
		Email:     req.Email,
		Notes:     req.Notes,
		CreatedBy: &actorID,
		Phones:    phones,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, errs.Internal(err)
	}

	s.record(ctx, actorID, enums.AuditContactCreated, c.ID, meta, map[string]any{"name": c.Name})

	res := responses.NewContact(c)
	return &res, nil
}

// List returns a filtered page of contacts.
func (s *Service) List(ctx context.Context, actorID uint, filter requests.ContactFilter) (*responses.ContactList, error) {
	if _, err := s.authorize(ctx, actorID, enums.ContactView); err != nil {
		return nil, err
	}
	filter.Page, filter.PerPage = normalizePaging(filter.Page, filter.PerPage)

	contacts, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, errs.Internal(err)
	}
	items := make([]responses.Contact, 0, len(contacts))
	for i := range contacts {
		items = append(items, responses.NewContact(&contacts[i]))
	}
	return &responses.ContactList{Items: items, Total: total, Page: filter.Page, PerPage: filter.PerPage}, nil
}

// Get returns one contact by id.
func (s *Service) Get(ctx context.Context, actorID, id uint) (*responses.Contact, error) {
	if _, err := s.authorize(ctx, actorID, enums.ContactView); err != nil {
		return nil, err
	}
	c, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	res := responses.NewContact(c)
	return &res, nil
}

// Update edits a contact's core fields.
func (s *Service) Update(ctx context.Context, actorID, id uint, req requests.ContactUpdate, meta Meta) (*responses.Contact, error) {
	if _, err := s.authorize(ctx, actorID, enums.ContactManage); err != nil {
		return nil, err
	}
	if _, err := s.load(ctx, id); err != nil {
		return nil, err
	}

	fields := map[string]any{
		"name":    req.Name,
		"company": req.Company,
		"email":   req.Email,
		"notes":   req.Notes,
	}
	if err := s.repo.UpdateCore(ctx, id, fields); err != nil {
		return nil, errs.Internal(err)
	}
	s.record(ctx, actorID, enums.AuditContactUpdated, id, meta, nil)

	c, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	res := responses.NewContact(c)
	return &res, nil
}

// Delete soft-deletes a contact.
func (s *Service) Delete(ctx context.Context, actorID, id uint, meta Meta) error {
	if _, err := s.authorize(ctx, actorID, enums.ContactManage); err != nil {
		return err
	}
	if _, err := s.load(ctx, id); err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return errs.Internal(err)
	}
	s.record(ctx, actorID, enums.AuditContactDeleted, id, meta, nil)
	return nil
}

// AddPhone attaches a phone number to a contact.
func (s *Service) AddPhone(ctx context.Context, actorID, contactID uint, req requests.ContactPhoneInput, meta Meta) (*responses.Contact, error) {
	if _, err := s.authorize(ctx, actorID, enums.ContactManage); err != nil {
		return nil, err
	}
	if _, err := s.load(ctx, contactID); err != nil {
		return nil, err
	}

	number, err := phone.Normalize(req.Number)
	if err != nil {
		return nil, errs.Invalid("Geçersiz telefon numarası.", err)
	}
	exists, err := s.repo.PhoneExists(ctx, number)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if exists {
		return nil, errs.Conflict("Bu numara zaten bir kişiye kayıtlı.", nil)
	}

	p := &models.ContactPhone{
		ContactID:  contactID,
		Label:      label(req.Label),
		NumberE164: number,
		IsPrimary:  req.IsPrimary,
	}
	if err := s.repo.AddPhone(ctx, p); err != nil {
		return nil, errs.Internal(err)
	}
	s.record(ctx, actorID, enums.AuditContactPhoneAdded, contactID, meta, map[string]any{"number": number})

	c, err := s.load(ctx, contactID)
	if err != nil {
		return nil, err
	}
	res := responses.NewContact(c)
	return &res, nil
}

// RemovePhone detaches a phone number from a contact.
func (s *Service) RemovePhone(ctx context.Context, actorID, contactID, phoneID uint, meta Meta) error {
	if _, err := s.authorize(ctx, actorID, enums.ContactManage); err != nil {
		return err
	}
	removed, err := s.repo.RemovePhone(ctx, contactID, phoneID)
	if err != nil {
		return errs.Internal(err)
	}
	if !removed {
		return errs.NotFound("Telefon numarası bulunamadı.")
	}
	s.record(ctx, actorID, enums.AuditContactPhoneRemove, contactID, meta, map[string]any{"phoneId": phoneID})
	return nil
}

// ResolveByNumber returns the contact that owns a number.
func (s *Service) ResolveByNumber(ctx context.Context, actorID uint, number string) (*responses.Contact, error) {
	if _, err := s.authorize(ctx, actorID, enums.ContactView); err != nil {
		return nil, err
	}
	e164, err := phone.Normalize(number)
	if err != nil {
		return nil, errs.Invalid("Geçersiz telefon numarası.", err)
	}
	c, err := s.repo.ResolveByNumber(ctx, e164)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if c == nil {
		return nil, errs.NotFound("Bu numaraya ait kişi bulunamadı.")
	}
	res := responses.NewContact(c)
	return &res, nil
}

// buildPhones normalizes and de-duplicates create-time phones, ensuring exactly
// one primary and no number already held by another contact.
func (s *Service) buildPhones(ctx context.Context, in []requests.ContactPhoneInput) ([]models.ContactPhone, error) {
	phones := make([]models.ContactPhone, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	primarySet := false
	for _, p := range in {
		number, err := phone.Normalize(p.Number)
		if err != nil {
			return nil, errs.Invalid("Geçersiz telefon numarası: "+p.Number, err)
		}
		if _, dup := seen[number]; dup {
			continue
		}
		seen[number] = struct{}{}

		exists, err := s.repo.PhoneExists(ctx, number)
		if err != nil {
			return nil, errs.Internal(err)
		}
		if exists {
			return nil, errs.Conflict("Bu numara zaten bir kişiye kayıtlı: "+number, nil)
		}

		isPrimary := p.IsPrimary && !primarySet
		if isPrimary {
			primarySet = true
		}
		phones = append(phones, models.ContactPhone{
			Label:      label(p.Label),
			NumberE164: number,
			IsPrimary:  isPrimary,
		})
	}
	if !primarySet && len(phones) > 0 {
		phones[0].IsPrimary = true
	}
	return phones, nil
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

func (s *Service) load(ctx context.Context, id uint) (*models.Contact, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if c == nil {
		return nil, errs.NotFound("Kişi bulunamadı.")
	}
	return c, nil
}

func (s *Service) record(ctx context.Context, actorID uint, action string, contactID uint, meta Meta, detail map[string]any) {
	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     action,
		TargetType: "contact",
		TargetID:   strconv.FormatUint(uint64(contactID), 10),
		IP:         meta.IP,
		Detail:     detail,
	})
}

func label(v string) string {
	if v == "" {
		return string(enums.PhoneLabelOther)
	}
	return v
}

func normalizePaging(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return page, perPage
}
