package responses

import "github.com/toprakgureli/santral-c/backend/internal/domain/models"

// Role is the public view of a role.
type Role struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	DisplayName   string `json:"displayName"`
	Description   string `json:"description"`
	System        bool   `json:"system"`
	UserCount     int64  `json:"userCount"`
	PermissionIDs []uint `json:"permissionIds"`
}

// NewRoles maps role models to their response view. Permission ids are included
// when the models were loaded with their permissions; counts default to zero.
func NewRoles(roles []models.Role) []Role {
	out := make([]Role, 0, len(roles))
	for i := range roles {
		out = append(out, NewRole(&roles[i], 0))
	}
	return out
}

// NewRole maps one role model to its response view with a user count.
func NewRole(r *models.Role, userCount int64) Role {
	ids := make([]uint, 0, len(r.Permissions))
	for _, p := range r.Permissions {
		ids = append(ids, p.ID)
	}
	return Role{
		ID:            r.ID,
		Name:          r.Name,
		DisplayName:   r.DisplayName,
		Description:   r.Description,
		System:        r.System,
		UserCount:     userCount,
		PermissionIDs: ids,
	}
}

// PermissionGroup is a module's permissions for the role editor.
type PermissionGroup struct {
	Module string           `json:"module"`
	Label  string           `json:"label"`
	Items  []PermissionItem `json:"items"`
}

// PermissionItem is one assignable permission.
type PermissionItem struct {
	ID          uint   `json:"id"`
	Key         string `json:"key"`
	Description string `json:"description"`
}
