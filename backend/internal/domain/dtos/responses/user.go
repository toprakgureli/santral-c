// Package responses holds outbound response DTOs.
package responses

import (
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

// User is the public view of a user.
type User struct {
	ID                   uint       `json:"id"`
	Name                 string     `json:"name"`
	Email                string     `json:"email"`
	Active               bool       `json:"active"`
	Roles                []string   `json:"roles"`
	RoleIDs              []uint     `json:"roleIds"`
	Permissions          []string   `json:"permissions"`
	MFAEnabled           bool       `json:"mfaEnabled"`
	MustChangePassword   bool       `json:"mustChangePassword"`
	SIPExtension         string     `json:"sipExtension,omitempty"`
	WhatsAppTemplate     string     `json:"whatsappTemplate"`
	WhatsAppTemplateLive string     `json:"whatsappTemplateLive"`
	OnboardedAt          *time.Time `json:"onboardedAt,omitempty"`
	LastLoginAt          *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
}

// UserList is a paginated page of users.
type UserList struct {
	Items   []User `json:"items"`
	Total   int64  `json:"total"`
	Page    int    `json:"page"`
	PerPage int    `json:"perPage"`
}

// NewUser maps a user model to its response view.
func NewUser(u *models.User) User {
	roles := make([]string, 0, len(u.Roles))
	roleIDs := make([]uint, 0, len(u.Roles))
	for _, r := range u.Roles {
		roles = append(roles, r.Name)
		roleIDs = append(roleIDs, r.ID)
	}
	perms := u.Permissions()
	permKeys := make([]string, 0, len(perms))
	for _, p := range perms {
		permKeys = append(permKeys, string(p))
	}
	var ext string
	if u.SIPExtension != nil {
		ext = *u.SIPExtension
	}
	return User{
		ID:                   u.ID,
		Name:                 u.Name,
		Email:                u.Email,
		Active:               u.Active,
		Roles:                roles,
		RoleIDs:              roleIDs,
		Permissions:          permKeys,
		MFAEnabled:           u.MFAEnabled,
		WhatsAppTemplate:     u.WhatsAppTemplate,
		WhatsAppTemplateLive: u.WhatsAppTemplateLive,
		MustChangePassword:   u.MustChangePassword,
		SIPExtension:         ext,
		OnboardedAt:          u.OnboardedAt,
		LastLoginAt:          u.LastLoginAt,
		CreatedAt:            u.CreatedAt,
	}
}
