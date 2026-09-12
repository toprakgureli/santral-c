package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

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

// Reader serves the audit trail to users holding system.audit_view.
type Reader struct {
	db    *gorm.DB
	users IActorResolver
}

// NewReader builds an audit reader.
func NewReader(db *gorm.DB, users IActorResolver) *Reader {
	return &Reader{db: db, users: users}
}

type auditRow struct {
	models.AuditLog
	ActorName  string
	ActorEmail string
}

// List returns a filtered page of audit entries, newest first, with the
// actor's name and email resolved.
func (r *Reader) List(ctx context.Context, actorID uint, f requests.AuditFilter) (*responses.AuditList, error) {
	actor, err := r.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actor.Can(enums.SystemAuditView) {
		return nil, errs.Forbidden("Denetim kayıtlarını görüntüleme yetkiniz yok.")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 || f.PerPage > 200 {
		f.PerPage = 50
	}

	build := func() *gorm.DB {
		q := r.db.WithContext(ctx).
			Table("audit_log").
			Joins("LEFT JOIN users ON users.id = audit_log.actor_id")
		if a := strings.TrimSpace(f.Action); a != "" {
			if strings.HasSuffix(a, ".") {
				q = q.Where("audit_log.action LIKE ?", a+"%")
			} else {
				q = q.Where("audit_log.action = ?", a)
			}
		}
		if s := strings.TrimSpace(f.Query); s != "" {
			like := "%" + s + "%"
			q = q.Where("users.name ILIKE ? OR users.email ILIKE ? OR audit_log.target_id = ? OR audit_log.ip = ?", like, like, s, s)
		}
		return q
	}

	var total int64
	if err := build().Count(&total).Error; err != nil {
		return nil, errs.Internal(fmt.Errorf("audit entries could not be counted: %w", err))
	}
	var rows []auditRow
	if err := build().
		Select("audit_log.*, users.name AS actor_name, users.email AS actor_email").
		Order("audit_log.created_at DESC, audit_log.id DESC").
		Limit(f.PerPage).
		Offset((f.Page - 1) * f.PerPage).
		Scan(&rows).Error; err != nil {
		return nil, errs.Internal(fmt.Errorf("audit entries could not be listed: %w", err))
	}

	items := make([]responses.AuditItem, 0, len(rows))
	for i := range rows {
		detail := json.RawMessage(rows[i].Detail)
		if !json.Valid(detail) {
			detail = json.RawMessage("{}")
		}
		items = append(items, responses.AuditItem{
			ID:         rows[i].ID,
			ActorID:    rows[i].ActorID,
			ActorName:  rows[i].ActorName,
			ActorEmail: rows[i].ActorEmail,
			Action:     rows[i].Action,
			TargetType: rows[i].TargetType,
			TargetID:   rows[i].TargetID,
			IP:         rows[i].IP,
			Detail:     detail,
			CreatedAt:  rows[i].CreatedAt,
		})
	}
	return &responses.AuditList{Items: items, Total: total, Page: f.Page, PerPage: f.PerPage}, nil
}
