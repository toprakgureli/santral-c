package requests

// UserCreate is a manager-created account.
type UserCreate struct {
	Name         string `json:"name" validate:"required,min=2,max=120"`
	Email        string `json:"email" validate:"required,email,max=255"`
	Password     string `json:"password" validate:"required,min=8,max=16"`
	RoleIDs      []uint `json:"roleIds" validate:"required,min=1,dive,gt=0"`
	SIPExtension string `json:"sipExtension" validate:"omitempty,min=2,max=32"`
}

// UserUpdate edits a user's profile and role assignment.
type UserUpdate struct {
	Name    string `json:"name" validate:"required,min=2,max=120"`
	Email   string `json:"email" validate:"required,email,max=255"`
	RoleIDs []uint `json:"roleIds" validate:"required,min=1,dive,gt=0"`
}

// WhatsAppTemplate carries the actor's own WhatsApp texts: Template for an
// unreached customer, Live for a customer currently on the phone.
type WhatsAppTemplate struct {
	Template string `json:"template" validate:"max=1000"`
	Live     string `json:"live" validate:"max=1000"`
}

// AvatarUpdate carries the actor's cropped photo as a webp data URI; empty removes it.
type AvatarUpdate struct {
	Avatar string `json:"avatar" validate:"max=130000"`
}

// UserPassword is an admin password reset.
type UserPassword struct {
	Password string `json:"password" validate:"required,min=8,max=16"`
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
