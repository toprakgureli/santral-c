package requests

// UserCreate is a manager-created account.
type UserCreate struct {
	Name         string `json:"name" validate:"required,min=2,max=120"`
	Email        string `json:"email" validate:"required,email,max=255"`
	Password     string `json:"password" validate:"required,min=8,max=128"`
	RoleIDs      []uint `json:"roleIds" validate:"required,min=1,dive,gt=0"`
	SIPExtension string `json:"sipExtension" validate:"omitempty,min=2,max=32"`
}

// UserPassword is an admin password reset.
type UserPassword struct {
	Password string `json:"password" validate:"required,min=8,max=128"`
}

// UserActive toggles a user's active state.
type UserActive struct {
	Active bool `json:"active"`
}

// UserFilter filters the user listing.
type UserFilter struct {
	Query                 string
	RoleID                *uint
	Active                *bool
	Page                  int
	PerPage               int
	ExcludeInvisibleAdmin bool
}
