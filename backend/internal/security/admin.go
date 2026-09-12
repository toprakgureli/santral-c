package security

import (
	"context"
	"strconv"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/audit"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/requests"
	"github.com/toprakgureli/santral-c/backend/internal/domain/dtos/responses"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// Admin serves the login-attempt and IP-ban views of the security page. It is
// gated by system.logs and kept apart from the login-time Service so the hot
// path never loads the actor.
type Admin struct {
	repo  *Repository
	users IActorResolver
	audit IAudit
}

// NewAdmin builds the security administration service.
func NewAdmin(repo *Repository, users IActorResolver, auditor IAudit) *Admin {
	return &Admin{repo: repo, users: users, audit: auditor}
}

// Attempts returns a page of login attempts.
func (a *Admin) Attempts(ctx context.Context, actorID uint, f requests.SecurityFilter) (*responses.LoginAttemptList, error) {
	if err := a.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 200 {
		f.PerPage = 50
	}
	items, total, err := a.repo.Attempts(ctx, f)
	if err != nil {
		return nil, errs.Internal(err)
	}
	list := responses.NewLoginAttemptList(items, total, f.Page, f.PerPage)
	return &list, nil
}

// Bans lists the IP bans still in force.
func (a *Admin) Bans(ctx context.Context, actorID uint) ([]responses.IPBanItem, error) {
	if err := a.authorize(ctx, actorID); err != nil {
		return nil, err
	}
	items, err := a.repo.Bans(ctx, time.Now())
	if err != nil {
		return nil, errs.Internal(err)
	}
	return responses.NewIPBans(items), nil
}

// Unban lifts an IP ban and records who did it.
func (a *Admin) Unban(ctx context.Context, actorID, id uint, ip string) error {
	if err := a.authorize(ctx, actorID); err != nil {
		return err
	}
	ban, err := a.repo.BanByID(ctx, id)
	if err != nil {
		return errs.Internal(err)
	}
	if ban == nil {
		return errs.NotFound("Ban kaydı bulunamadı.")
	}
	if err := a.repo.Unban(ctx, id); err != nil {
		return errs.Internal(err)
	}
	a.audit.Record(ctx, audit.Entry{
		ActorID:    &actorID,
		Action:     enums.AuditIPUnbanned,
		TargetType: "ip_ban",
		TargetID:   strconv.FormatUint(uint64(id), 10),
		IP:         ip,
		Detail:     map[string]any{"ip": ban.IP, "reason": ban.Reason},
	})
	return nil
}

func (a *Admin) authorize(ctx context.Context, actorID uint) error {
	actor, err := a.users.GetByID(ctx, actorID)
	if err != nil {
		return err
	}
	if !actor.Can(enums.SystemLogs) {
		return errs.Forbidden("Güvenlik kayıtlarını görüntüleme yetkiniz yok.")
	}
	return nil
}
