package user

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
)

// Meta carries request context for audit trails.
type Meta struct {
	IP string
}

// Service is the user application service.
type Service struct {
	repo        *Repository
	audit       IAudit
	revoker     ISessionRevoker
	provisioner IProvisioner
}

// NewService builds a user service. provisioner may be nil when Asterisk
// integration is disabled.
func NewService(repo *Repository, auditor IAudit, revoker ISessionRevoker, provisioner IProvisioner) *Service {
	return &Service{repo: repo, audit: auditor, revoker: revoker, provisioner: provisioner}
}

// GetByEmail loads a user by email, or nil when absent.
func (s *Service) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return nil, errs.Internal(err)
	}
	return u, nil
}

// GetByID loads a user by id and fails when absent.
func (s *Service) GetByID(ctx context.Context, id uint) (*models.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if u == nil {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	return u, nil
}

// MarkLogin stamps the last login time.
func (s *Service) MarkLogin(ctx context.Context, id uint) error {
	if err := s.repo.UpdateLastLogin(ctx, id, time.Now()); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// MarkOnboarded records that the user finished onboarding.
func (s *Service) MarkOnboarded(ctx context.Context, id uint) error {
	if err := s.repo.MarkOnboarded(ctx, id, time.Now()); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// SetMFA updates the encrypted secret and enabled flag.
func (s *Service) SetMFA(ctx context.Context, id uint, secret *string, enabled bool) error {
	if err := s.repo.SetMFA(ctx, id, secret, enabled); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// ChangePassword hashes and stores a new password and clears the must-change flag.
func (s *Service) ChangePassword(ctx context.Context, id uint, password string) error {
	hashed, err := hash.Password(password)
	if err != nil {
		return errs.Internal(err)
	}
	if err := s.repo.SetPassword(ctx, id, hashed, false); err != nil {
		return errs.Internal(err)
	}
	return nil
}

// CreateUser provisions a new account, assigns roles and forces a first-login
// password change. It requires the actor to hold both create and assign rights.
func (s *Service) CreateUser(ctx context.Context, actorID uint, req requests.UserCreate, meta Meta) (*responses.User, error) {
	actor, err := s.authorize(ctx, actorID, enums.UserCreate)
	if err != nil {
		return nil, err
	}
	if _, err := s.require(actor, enums.RoleAssign); err != nil {
		return nil, err
	}

	roles, err := s.resolveRoles(ctx, actor, req.RoleIDs)
	if err != nil {
		return nil, err
	}

	exists, err := s.repo.EmailExists(ctx, req.Email)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if exists {
		return nil, errs.Conflict("Bu e-posta zaten kullanımda.", nil)
	}

	hashed, err := hash.Password(req.Password)
	if err != nil {
		return nil, errs.Internal(err)
	}

	u := &models.User{
		Name:               req.Name,
		Email:              req.Email,
		Password:           hashed,
		Active:             true,
		MustChangePassword: true,
		CreatedBy:          &actorID,
	}
	if req.SIPExtension != "" {
		ext := req.SIPExtension
		u.SIPExtension = &ext
	}

	if err := s.repo.Create(ctx, u, roles); err != nil {
		return nil, errs.Internal(err)
	}
	u.Roles = roles

	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     enums.AuditUserCreated,
		TargetType: "user",
		TargetID:   strconv.FormatUint(uint64(u.ID), 10),
		IP:         meta.IP,
		Detail:     map[string]any{"email": u.Email, "roleIds": req.RoleIDs},
	})

	if req.SIPExtension != "" && s.provisioner != nil {
		if err := s.provisioner.Provision(ctx, u.ID); err != nil {
			slog.Warn("sip endpoint could not be provisioned", "user_id", u.ID, "error", err)
		}
	}

	res := responses.NewUser(u)
	return &res, nil
}

// List returns a filtered page of users. Non invisible-admin actors never see
// invisible-admin accounts.
func (s *Service) List(ctx context.Context, actorID uint, filter requests.UserFilter) (*responses.UserList, error) {
	actor, err := s.authorize(ctx, actorID, enums.UserView)
	if err != nil {
		return nil, err
	}

	filter.Page, filter.PerPage = normalizePaging(filter.Page, filter.PerPage)
	if !actor.IsInvisibleAdmin() {
		filter.ExcludeInvisibleAdmin = true
	}

	users, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, errs.Internal(err)
	}

	items := make([]responses.User, 0, len(users))
	for i := range users {
		items = append(items, responses.NewUser(&users[i]))
	}
	return &responses.UserList{
		Items:   items,
		Total:   total,
		Page:    filter.Page,
		PerPage: filter.PerPage,
	}, nil
}

// SetActive activates or deactivates a user. Deactivation revokes the target's
// sessions so the change takes effect immediately.
func (s *Service) SetActive(ctx context.Context, actorID, targetID uint, active bool, meta Meta) error {
	actor, err := s.authorize(ctx, actorID, enums.UserDeactivate)
	if err != nil {
		return err
	}
	if actorID == targetID {
		return errs.Invalid("Kendi hesabınızın durumunu değiştiremezsiniz.", nil)
	}

	target, err := s.visibleTarget(ctx, actor, targetID)
	if err != nil {
		return err
	}

	if err := s.repo.SetActive(ctx, target.ID, active); err != nil {
		return errs.Internal(err)
	}

	action := enums.AuditUserActivated
	if !active {
		action = enums.AuditUserDeactivated
		if err := s.revoker.RevokeUserSessions(ctx, target.ID, time.Now()); err != nil {
			return errs.Internal(err)
		}
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     action,
		TargetType: "user",
		TargetID:   strconv.FormatUint(uint64(target.ID), 10),
		IP:         meta.IP,
	})
	return nil
}

// ResetPassword sets a new password for a user, forces a change at next login
// and revokes the target's sessions.
func (s *Service) ResetPassword(ctx context.Context, actorID, targetID uint, password string, meta Meta) error {
	actor, err := s.authorize(ctx, actorID, enums.UserUpdate)
	if err != nil {
		return err
	}

	target, err := s.visibleTarget(ctx, actor, targetID)
	if err != nil {
		return err
	}

	hashed, err := hash.Password(password)
	if err != nil {
		return errs.Internal(err)
	}
	if err := s.repo.SetPassword(ctx, target.ID, hashed, true); err != nil {
		return errs.Internal(err)
	}
	if err := s.revoker.RevokeUserSessions(ctx, target.ID, time.Now()); err != nil {
		return errs.Internal(err)
	}

	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     enums.AuditUserPasswordReset,
		TargetType: "user",
		TargetID:   strconv.FormatUint(uint64(target.ID), 10),
		IP:         meta.IP,
	})
	return nil
}

// authorize loads the actor and verifies a permission.
func (s *Service) authorize(ctx context.Context, actorID uint, perm enums.Permission) (*models.User, error) {
	actor, err := s.repo.GetByID(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if actor == nil {
		return nil, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	return s.require(actor, perm)
}

// require verifies a loaded actor holds a permission.
func (s *Service) require(actor *models.User, perm enums.Permission) (*models.User, error) {
	if !actor.Can(perm) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	return actor, nil
}

// resolveRoles loads the requested roles and blocks non invisible-admins from
// assigning the invisible-admin role.
func (s *Service) resolveRoles(ctx context.Context, actor *models.User, ids []uint) ([]models.Role, error) {
	roles, err := s.repo.RolesByIDs(ctx, ids)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if len(roles) != len(dedupe(ids)) {
		return nil, errs.Invalid("Geçersiz rol seçimi.", nil)
	}
	if !actor.IsInvisibleAdmin() {
		for i := range roles {
			if enums.Role(roles[i].Name) == enums.RoleInvisibleAdmin {
				return nil, errs.Forbidden("Bu rolü atayamazsınız.")
			}
		}
	}
	return roles, nil
}

// visibleTarget loads a target user, hiding invisible-admin accounts from
// actors who are not themselves invisible admins.
func (s *Service) visibleTarget(ctx context.Context, actor *models.User, targetID uint) (*models.User, error) {
	target, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if target == nil || (target.IsInvisibleAdmin() && !actor.IsInvisibleAdmin()) {
		return nil, errs.NotFound("Kullanıcı bulunamadı.")
	}
	return target, nil
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

func dedupe(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
