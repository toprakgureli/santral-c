package models

import (
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// User is an internal panel user and softphone identity.
type User struct {
	ID                   uint    `gorm:"primarykey"`
	Name                 string  `gorm:"size:120;not null"`
	Email                string  `gorm:"size:255;not null;uniqueIndex"`
	Password             string  `gorm:"size:255;not null" json:"-"`
	Active               bool    `gorm:"not null;default:true"`
	MustChangePassword   bool    `gorm:"not null;default:false"`
	MFASecret            *string `gorm:"size:255" json:"-"`
	MFAEnabled           bool    `gorm:"not null;default:false"`
	MFAExempt            bool    `gorm:"not null;default:false"`
	SIPExtension         *string `gorm:"column:sip_extension;size:32" json:"sipExtension,omitempty"`
	SIPSecret            *string `gorm:"column:sip_secret;size:255" json:"-"`
	SIPProvisioned       bool    `gorm:"column:sip_provisioned;not null;default:false"`
	WhatsAppTemplate     string  `gorm:"column:whatsapp_template;type:text;not null;default:''" json:"-"`
	WhatsAppTemplateLive string  `gorm:"column:whatsapp_template_live;type:text;not null;default:''" json:"-"`
	Avatar               string  `gorm:"column:avatar;type:text;not null;default:''" json:"-"`
	Headline             string  `gorm:"column:headline;size:120;not null;default:''" json:"-"`
	Bio                  string  `gorm:"column:bio;type:text;not null;default:''" json:"-"`
	LastLoginAt          *time.Time
	OnboardedAt          *time.Time
	LockedUntil          *time.Time
	FailedCount          int    `gorm:"not null;default:0"`
	CreatedBy            *uint  `gorm:"index"`
	Roles                []Role `gorm:"many2many:user_roles"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            gorm.DeletedAt `gorm:"index"`
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

// CanGrant reports whether the user may give p to someone else; see
// enums.Grantable.
func (u *User) CanGrant(p enums.Permission) bool {
	return enums.Grantable(u.Can, p)
}

// CanManage reports whether the user may change target's account: the target
// holds no permission the user could not hand out themselves. Invisible
// admins may manage anyone.
func (u *User) CanManage(target *User) bool {
	if u.IsInvisibleAdmin() {
		return true
	}
	for _, p := range target.Permissions() {
		if !u.CanGrant(p) {
			return false
		}
	}
	return true
}

// InvisibleAdminIDsSQL selects the ids of the users holding the
// invisible-admin role. Lists use it to leave the owner account out for
// everyone but another invisible admin.
var InvisibleAdminIDsSQL = "SELECT user_roles.user_id FROM user_roles JOIN roles ON roles.id = user_roles.role_id WHERE roles.name = '" +
	string(enums.RoleInvisibleAdmin) + "'"

// StatsHiddenIDsSQL selects the invisible admins kept out of the statistics
// (team performance, profiles, the agent list): all of them except those who
// also hold another role with performance.show_hidden_admin, since they work
// in that role and their colleagues should see their numbers. The
// invisible-admin role itself holds every permission, so it does not count.
var StatsHiddenIDsSQL = InvisibleAdminIDsSQL + " AND user_roles.user_id NOT IN (" +
	"SELECT ur.user_id FROM user_roles ur JOIN roles r ON r.id = ur.role_id AND r.deleted_at IS NULL " +
	"JOIN role_permissions rp ON rp.role_id = r.id JOIN permissions p ON p.id = rp.permission_id " +
	"WHERE r.name <> '" + string(enums.RoleInvisibleAdmin) + "' AND p.key = '" + string(enums.PerformanceShowHiddenAdmin) + "')"

// HiddenInStats is StatsHiddenIDsSQL for a loaded user; it needs the roles
// with their permissions.
func (u *User) HiddenInStats() bool {
	if !u.IsInvisibleAdmin() {
		return false
	}
	for _, r := range u.Roles {
		if enums.Role(r.Name) == enums.RoleInvisibleAdmin {
			continue
		}
		for _, p := range r.Permissions {
			if enums.Permission(p.Key) == enums.PerformanceShowHiddenAdmin {
				return false
			}
		}
	}
	return true
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
