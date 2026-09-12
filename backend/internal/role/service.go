package role

import (
	"context"
	"strconv"
	"strings"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// IActorResolver loads the acting user for authorization.
type IActorResolver interface {
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// IAudit records privileged mutations.
type IAudit interface {
	Record(ctx context.Context, e audit.Entry)
}

// Meta carries request context for audit trails.
type Meta struct {
	IP string
}

// Service is the role application service.
type Service struct {
	repo  *Repository
	users IActorResolver
	audit IAudit
}

// NewService builds a role service.
func NewService(repo *Repository, users IActorResolver, auditor IAudit) *Service {
	return &Service{repo: repo, users: users, audit: auditor}
}

// List returns the assignable roles with their permission sets and user counts.
// The invisible-admin role is hidden from actors who are not themselves
// invisible admins.
func (s *Service) List(ctx context.Context, actorID uint) ([]responses.Role, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.RoleView) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}

	roles, err := s.repo.List(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	counts, err := s.repo.UserCounts(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}

	out := make([]responses.Role, 0, len(roles))
	for i := range roles {
		if enums.Role(roles[i].Name) == enums.RoleInvisibleAdmin && !actor.IsInvisibleAdmin() {
			continue
		}
		out = append(out, responses.NewRole(&roles[i], counts[roles[i].ID]))
	}
	return out, nil
}

// Permissions returns the permission catalog grouped by module for the editor.
func (s *Service) Permissions(ctx context.Context, actorID uint) ([]responses.PermissionGroup, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.RoleView) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	perms, err := s.repo.Permissions(ctx)
	if err != nil {
		return nil, errs.Internal(err)
	}
	order := make([]string, 0)
	byModule := make(map[string]*responses.PermissionGroup)
	for i := range perms {
		mod := perms[i].Module
		g, ok := byModule[mod]
		if !ok {
			g = &responses.PermissionGroup{Module: mod, Label: enums.ModuleLabel(enums.Module(mod))}
			byModule[mod] = g
			order = append(order, mod)
		}
		g.Items = append(g.Items, responses.PermissionItem{ID: perms[i].ID, Key: perms[i].Key, Description: perms[i].Description})
	}
	out := make([]responses.PermissionGroup, 0, len(order))
	for _, mod := range order {
		out = append(out, *byModule[mod])
	}
	return out, nil
}

// Create adds a custom role. The technical name is normalized to lower snake
// case because it cannot be changed afterwards.
func (s *Service) Create(ctx context.Context, actorID uint, req requests.RoleCreate, meta Meta) (*responses.Role, error) {
	actor, err := s.authorize(ctx, actorID)
	if err != nil {
		return nil, err
	}
	name := normalizeName(req.Name)
	if len(name) < 3 {
		return nil, errs.Invalid("Rol adı en az 3 harf veya rakam içermeli.", nil)
	}
	perms, err := s.resolvePermissions(ctx, req.PermissionIDs)
	if err != nil {
		return nil, err
	}
	if err := ensureCanGrant(actor, nil, perms); err != nil {
		return nil, err
	}
	role := &models.Role{
		Name:        name,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Description: strings.TrimSpace(req.Description),
		System:      false,
		Permissions: perms,
	}
	if err := s.repo.Create(ctx, role); err != nil {
		return nil, errs.Conflict("Bu rol adı zaten kullanımda.", err)
	}
	s.record(ctx, actorID, enums.AuditRoleCreated, role.ID, meta, map[string]any{
		"name": role.Name, "displayName": role.DisplayName, "permissionIds": req.PermissionIDs,
	})
	res := responses.NewRole(role, 0)
	return &res, nil
}

// Update edits a role's display fields and permission set. System roles keep
// their name, and the invisible-admin role can never lose role management (it
// would leave nobody able to repair roles).
func (s *Service) Update(ctx context.Context, actorID, id uint, req requests.RoleUpdate, meta Meta) (*responses.Role, error) {
	actor, err := s.authorize(ctx, actorID)
	if err != nil {
		return nil, err
	}
	role, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	perms, err := s.resolvePermissions(ctx, req.PermissionIDs)
	if err != nil {
		return nil, err
	}
	if err := ensureCanGrant(actor, role.Permissions, perms); err != nil {
		return nil, err
	}
	if enums.Role(role.Name) == enums.RoleInvisibleAdmin && !hasKey(perms, enums.RoleManage) {
		return nil, errs.Invalid("Görünmez yönetici rolünden rol yönetimi yetkisi kaldırılamaz.", nil)
	}
	fields := map[string]any{
		"display_name": strings.TrimSpace(req.DisplayName),
		"description":  strings.TrimSpace(req.Description),
	}
	if err := s.repo.UpdateCore(ctx, id, fields); err != nil {
		return nil, errs.Internal(err)
	}
	if err := s.repo.ReplacePermissions(ctx, role, perms); err != nil {
		return nil, errs.Internal(err)
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil || updated == nil {
		return nil, errs.Internal(err)
	}
	s.record(ctx, actorID, enums.AuditRoleUpdated, id, meta, map[string]any{
		"name": updated.Name, "displayName": updated.DisplayName, "permissionIds": req.PermissionIDs,
	})
	res := responses.NewRole(updated, 0)
	return &res, nil
}

// Delete removes a custom role. System roles and roles still assigned to users
// cannot be deleted.
func (s *Service) Delete(ctx context.Context, actorID, id uint, meta Meta) error {
	actor, err := s.authorize(ctx, actorID)
	if err != nil {
		return err
	}
	role, err := s.load(ctx, actor, id)
	if err != nil {
		return err
	}
	if role.System {
		return errs.Invalid("Sistem rolleri silinemez.", nil)
	}
	count, err := s.repo.UserCount(ctx, id)
	if err != nil {
		return errs.Internal(err)
	}
	if count > 0 {
		return errs.Conflict("Bu role atanmış kullanıcılar var. Önce rollerini değiştirin.", nil)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return errs.Internal(err)
	}
	s.record(ctx, actorID, enums.AuditRoleDeleted, id, meta, map[string]any{"name": role.Name, "displayName": role.DisplayName})
	return nil
}

func (s *Service) authorize(ctx context.Context, actorID uint) (*models.User, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.RoleManage) {
		return nil, errs.Forbidden("Bu işlem için yetkiniz yok.")
	}
	return actor, nil
}

func (s *Service) load(ctx context.Context, actor *models.User, id uint) (*models.Role, error) {
	role, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.Internal(err)
	}
	if role == nil {
		return nil, errs.NotFound("Rol bulunamadı.")
	}
	if enums.Role(role.Name) == enums.RoleInvisibleAdmin && !actor.IsInvisibleAdmin() {
		return nil, errs.Forbidden("Bu rolü düzenleyemezsiniz.")
	}
	return role, nil
}

func (s *Service) resolvePermissions(ctx context.Context, ids []uint) ([]models.Permission, error) {
	perms, err := s.repo.PermissionsByIDs(ctx, dedupe(ids))
	if err != nil {
		return nil, errs.Internal(err)
	}
	if len(perms) != len(dedupe(ids)) {
		return nil, errs.Invalid("Geçersiz yetki seçimi.", nil)
	}
	return perms, nil
}

func (s *Service) record(ctx context.Context, actorID uint, action string, roleID uint, meta Meta, detail map[string]any) {
	if s.audit == nil {
		return
	}
	s.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     action,
		TargetType: "role",
		TargetID:   strconv.FormatUint(uint64(roleID), 10),
		IP:         meta.IP,
		Detail:     detail,
	})
}

// ensureCanGrant blocks an actor from handing out permissions they do not hold
// themselves; invisible admins are exempt. Permissions the role already has are
// not re-checked, so an editor with fewer rights can still adjust the rest.
func ensureCanGrant(actor *models.User, current, next []models.Permission) error {
	if actor.IsInvisibleAdmin() {
		return nil
	}
	existing := make(map[uint]bool, len(current))
	for i := range current {
		existing[current[i].ID] = true
	}
	for i := range next {
		if existing[next[i].ID] {
			continue
		}
		if !actor.Can(enums.Permission(next[i].Key)) {
			return errs.Forbidden("Kendinizde olmayan bir yetkiyi role veremezsiniz.")
		}
	}
	return nil
}

func hasKey(perms []models.Permission, key enums.Permission) bool {
	for i := range perms {
		if enums.Permission(perms[i].Key) == key {
			return true
		}
	}
	return false
}

// normalizeName lowercases a technical role name and folds whitespace and
// punctuation into single underscores.
func normalizeName(raw string) string {
	var b strings.Builder
	lastUnderscore := true
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
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
