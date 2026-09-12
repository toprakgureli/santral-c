package responses

import (
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// LoginAttemptItem is one authentication attempt in the security log.
type LoginAttemptItem struct {
	ID        uint      `json:"id"`
	Email     string    `json:"email"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"userAgent"`
	Success   bool      `json:"success"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

// LoginAttemptList is a page of login attempts.
type LoginAttemptList struct {
	Items   []LoginAttemptItem `json:"items"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	PerPage int                `json:"perPage"`
}

// IPBanItem is one active IP ban.
type IPBanItem struct {
	ID       uint      `json:"id"`
	IP       string    `json:"ip"`
	Reason   string    `json:"reason"`
	Attempts int       `json:"attempts"`
	Until    time.Time `json:"until"`
}

// NewLoginAttemptList maps attempt models to a page view.
func NewLoginAttemptList(items []models.LoginAttempt, total int64, page, perPage int) LoginAttemptList {
	list := make([]LoginAttemptItem, 0, len(items))
	for i := range items {
		list = append(list, LoginAttemptItem{
			ID:        items[i].ID,
			Email:     items[i].Email,
			IP:        items[i].IP,
			UserAgent: items[i].UserAgent,
			Success:   items[i].Success,
			Reason:    items[i].Reason,
			CreatedAt: items[i].CreatedAt,
		})
	}
	return LoginAttemptList{Items: list, Total: total, Page: page, PerPage: perPage}
}

// NewIPBans maps ban models to their list view.
func NewIPBans(items []models.IPBan) []IPBanItem {
	list := make([]IPBanItem, 0, len(items))
	for i := range items {
		list = append(list, IPBanItem{
			ID:       items[i].ID,
			IP:       items[i].IP,
			Reason:   items[i].Reason,
			Attempts: items[i].Attempts,
			Until:    items[i].Until,
		})
	}
	return list
}
