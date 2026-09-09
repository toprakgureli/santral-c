package models

import (
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// User is an internal panel user and softphone identity.
type User struct {
	ID                 uint    `gorm:"primarykey"`
	Name               string  `gorm:"size:120;not null"`
	Email              string  `gorm:"size:255;not null;uniqueIndex"`
	Password           string  `gorm:"size:255;not null" json:"-"`
	Active             bool    `gorm:"not null;default:true"`
	MustChangePassword bool    `gorm:"not null;default:false"`
	MFASecret          *string `gorm:"size:255" json:"-"`
	MFAEnabled         bool    `gorm:"not null;default:false"`
	MFAExempt          bool    `gorm:"not null;default:false"`
	SIPExtension       *string `gorm:"size:32" json:"sipExtension,omitempty"`
	SIPProvisioned     bool    `gorm:"not null;default:false"`
	LastLoginAt        *time.Time
	OnboardedAt        *time.Time
	LockedUntil        *time.Time
	FailedCount        int    `gorm:"not null;default:0"`
	CreatedBy          *uint  `gorm:"index"`
	Roles              []Role `gorm:"many2many:user_roles"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

// Permissions returns the deduplicated, sorted permissions across the user's roles.
func (u *User) Permissions() []enums.Permission {
	seen := make(map[enums.Permission]struct{})
	for _, r := range u.Roles {
		for _, p := range r.Permissions {
			seen[enums.Permission(p.Key)] = struct{}{}
		}
	}
	out := make([]enums.Permission, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Can reports whether the user holds a permission through any role.
func (u *User) Can(p enums.Permission) bool {
	for _, r := range u.Roles {
		for _, perm := range r.Permissions {
			if enums.Permission(perm.Key) == p {
				return true
			}
		}
	}
	return false
}

// IsInvisibleAdmin reports whether the user carries the invisible-admin role.
func (u *User) IsInvisibleAdmin() bool {
	for _, r := range u.Roles {
		if enums.Role(r.Name) == enums.RoleInvisibleAdmin {
			return true
		}
	}
	return false
}
