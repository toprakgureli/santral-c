package requests

// RoleCreate creates a custom role with a permission set.
type RoleCreate struct {
	Name          string `json:"name" validate:"required,min=3,max=60"`
	DisplayName   string `json:"displayName" validate:"required,min=2,max=120"`
	Description   string `json:"description" validate:"max=255"`
	PermissionIDs []uint `json:"permissionIds" validate:"required,min=1"`
}

// RoleUpdate edits a role's display fields and permission set.
type RoleUpdate struct {
	DisplayName   string `json:"displayName" validate:"required,min=2,max=120"`
	Description   string `json:"description" validate:"max=255"`
	PermissionIDs []uint `json:"permissionIds" validate:"required,min=1"`
}

// UserRoles sets a user's role assignment.
type UserRoles struct {
	RoleIDs []uint `json:"roleIds" validate:"required,min=1"`
}
