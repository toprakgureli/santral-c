package user

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
	"github.com/toprakgureli/santral-c/backend/pkg/password"
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
func (s *Service) ChangePassword(ctx context.Context, id uint, pw string) error {
	if err := password.Validate(pw); err != nil {
		return err
	}
	hashed, err := hash.Password(pw)
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
	if err := password.Validate(req.Password); err != nil {
		return nil, err
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

// SetRoles replaces a user's role assignment.
func (s *Service) SetRoles(ctx context.Context, actorID, targetID uint, roleIDs []uint, meta Meta) (*responses.User, error) {
	actor, err := s.authorize(ctx, actorID, enums.RoleAssign)
	if err != nil {
		return nil, err
	}
	target, err := s.visibleTarget(ctx, actor, targetID)
	if err != nil {
		return nil, err
	}
	// An actor may not strip the invisible-admin role from an invisible admin,
	// nor grant it; resolveRoles blocks granting, and this blocks demotion of a
	// hidden owner by a non-owner.
	if target.IsInvisibleAdmin() && !actor.IsInvisibleAdmin() {
		return nil, errs.Forbidden("Bu kullanıcının rollerini değiştiremezsiniz.")
	}
	roles, err := s.resolveRoles(ctx, actor, roleIDs)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceRoles(ctx, target, roles); err != nil {
		return nil, errs.Internal(err)
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     enums.AuditUserRolesUpdated,
		TargetType: "user",
		TargetID:   strconv.FormatUint(uint64(target.ID), 10),
		IP:         meta.IP,
		Detail:     map[string]any{"roleIds": roleIDs},
	})
	updated, err := s.repo.GetByID(ctx, target.ID)
	if err != nil || updated == nil {
		return nil, errs.Internal(err)
	}
	res := responses.NewUser(updated)
	return &res, nil
}

// UpdateUser edits a user's name, email and role assignment in one step.
// Changing roles additionally needs role.assign, mirroring SetRoles, and the
// same invisible-admin guards apply.
func (s *Service) UpdateUser(ctx context.Context, actorID, targetID uint, req requests.UserUpdate, meta Meta) (*responses.User, error) {
	actor, err := s.authorize(ctx, actorID, enums.UserUpdate)
	if err != nil {
		return nil, err
	}
	target, err := s.visibleTarget(ctx, actor, targetID)
	if err != nil {
		return nil, err
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	exists, err := s.repo.EmailExistsExcept(ctx, email, target.ID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if exists {
		return nil, errs.Conflict("Bu e-posta adresi başka bir kullanıcıya ait.", nil)
	}

	currentRoles := make([]uint, 0, len(target.Roles))
	for _, r := range target.Roles {
		currentRoles = append(currentRoles, r.ID)
	}
	rolesChanged := !sameIDs(currentRoles, req.RoleIDs)
	var roles []models.Role
	if rolesChanged {
		if _, err := s.require(actor, enums.RoleAssign); err != nil {
			return nil, err
		}
		if target.IsInvisibleAdmin() && !actor.IsInvisibleAdmin() {
			return nil, errs.Forbidden("Bu kullanıcının rollerini değiştiremezsiniz.")
		}
		roles, err = s.resolveRoles(ctx, actor, req.RoleIDs)
		if err != nil {
			return nil, err
		}
	}

	fields := map[string]any{
		"name":  strings.TrimSpace(req.Name),
		"email": email,
	}
	if err := s.repo.UpdateCore(ctx, target.ID, fields); err != nil {
		return nil, errs.Internal(err)
	}
	if rolesChanged {
		if err := s.repo.ReplaceRoles(ctx, target, roles); err != nil {
			return nil, errs.Internal(err)
		}
	}

	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     enums.AuditUserUpdated,
		TargetType: "user",
		TargetID:   strconv.FormatUint(uint64(target.ID), 10),
		IP:         meta.IP,
		Detail:     map[string]any{"name": fields["name"], "email": email, "roleIds": req.RoleIDs, "rolesChanged": rolesChanged},
	})

	updated, err := s.repo.GetByID(ctx, target.ID)
	if err != nil || updated == nil {
		return nil, errs.Internal(err)
	}
	res := responses.NewUser(updated)
	return &res, nil
}

// ResetPassword sets a new password for a user, forces a change at next login
// and revokes the target's sessions.
func (s *Service) ResetPassword(ctx context.Context, actorID, targetID uint, pw string, meta Meta) error {
	actor, err := s.authorize(ctx, actorID, enums.UserUpdate)
	if err != nil {
		return err
	}

	target, err := s.visibleTarget(ctx, actor, targetID)
	if err != nil {
		return err
	}
	if err := password.Validate(pw); err != nil {
		return err
	}

	hashed, err := hash.Password(pw)
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

// SetWhatsAppTemplate stores the actor's own WhatsApp follow-up text. Any
// signed-in user may set their own; empty restores the panel default.
func (s *Service) SetWhatsAppTemplate(ctx context.Context, actorID uint, template, live string) (*responses.User, error) {
	actor, err := s.repo.GetByID(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if actor == nil {
		return nil, errs.Unauthorized("Oturum bulunamadı. Lütfen giriş yapın.")
	}
	fields := map[string]any{"whatsapp_template": strings.TrimSpace(template), "whatsapp_template_live": strings.TrimSpace(live)}
	if err := s.repo.UpdateCore(ctx, actorID, fields); err != nil {
		return nil, errs.Internal(err)
	}
	updated, err := s.repo.GetByID(ctx, actorID)
	if err != nil {
		return nil, errs.Internal(err)
	}
	dto := responses.NewUser(updated)
	return &dto, nil
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

// sameIDs reports whether two id lists hold the same set.
func sameIDs(a, b []uint) bool {
	x, y := dedupe(a), dedupe(b)
	if len(x) != len(y) {
		return false
	}
	sort.Slice(x, func(i, j int) bool { return x[i] < x[j] })
	sort.Slice(y, func(i, j int) bool { return y[i] < y[j] })
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
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
