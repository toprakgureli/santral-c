// Package notice keeps short notices for one person: something that
// happened which they should know about, such as a colleague reaching a
// customer they could not. The panel asks for new ones every few seconds,
// shows each once as a toast on whichever page is open, and keeps the last
// thirty days under the bell until they are read.
package notice

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/errs"
)

// keep is how far back the bell reaches.
const keep = 30 * 24 * time.Hour

// listLimit is how many notices one read returns.
const listLimit = 50

// Notice is one notice as the panel shows it.
type Notice struct {
	ID        uint      `json:"id"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	Link      string    `json:"link,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Read      bool      `json:"read"`
}

// List is the newest notices with how many are unread.
type List struct {
	Items  []Notice `json:"items"`
	Unread int64    `json:"unread"`
}

// Service stores and reads notices.
type Service struct {
	db *gorm.DB
}

// NewService builds the notice service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Add leaves a notice for a person. A notice is a courtesy: when it cannot
// be stored the reason is logged and nothing else fails.
func (s *Service) Add(ctx context.Context, userID uint, kind, text, link string) {
	n := models.UserNotice{UserID: userID, Kind: kind, Text: text, Link: link}
	if err := s.db.WithContext(ctx).Create(&n).Error; err != nil {
		slog.WarnContext(ctx, "notice could not be stored", "user", userID, "kind", kind, "error", err)
	}
}

// List returns the person's newest notices of the last thirty days; with
// after set, only those newer than that id.
func (s *Service) List(ctx context.Context, userID, after uint) (*List, error) {
	since := time.Now().Add(-keep)
	var rows []models.UserNotice
	q := s.db.WithContext(ctx).Where("user_id = ? AND created_at >= ?", userID, since)
	if after > 0 {
		q = q.Where("id > ?", after)
	}
	if err := q.Order("id DESC").Limit(listLimit).Find(&rows).Error; err != nil {
		return nil, errs.Internal(fmt.Errorf("notices could not be read: %w", err))
	}
	out := &List{Items: make([]Notice, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, Notice{ID: r.ID, Kind: r.Kind, Text: r.Text, Link: r.Link, CreatedAt: r.CreatedAt, Read: r.ReadAt != nil})
	}
	if err := s.db.WithContext(ctx).Model(&models.UserNotice{}).
		Where("user_id = ? AND created_at >= ? AND read_at IS NULL", userID, since).Count(&out.Unread).Error; err != nil {
		return nil, errs.Internal(fmt.Errorf("unread notices could not be counted: %w", err))
	}
	return out, nil
}

// MarkRead marks the person's notices read: the given ones, or all.
func (s *Service) MarkRead(ctx context.Context, userID uint, ids []uint, all bool) error {
	q := s.db.WithContext(ctx).Model(&models.UserNotice{}).Where("user_id = ? AND read_at IS NULL", userID)
	if !all {
		if len(ids) == 0 {
			return nil
		}
		q = q.Where("id IN ?", ids)
	}
	if err := q.Update("read_at", gorm.Expr("now()")).Error; err != nil {
		return errs.Internal(fmt.Errorf("notices could not be marked read: %w", err))
	}
	return nil
}
