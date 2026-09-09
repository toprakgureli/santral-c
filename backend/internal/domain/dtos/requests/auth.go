// Package requests holds inbound request DTOs.
package requests

// Login is the credential step of authentication.
type Login struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

// MFAVerify is the login second-factor step.
type MFAVerify struct {
	Token string `json:"token" validate:"required"`
	Code  string `json:"code" validate:"required,len=6,numeric"`
}

// MFAEnroll starts forced enrollment during login.
type MFAEnroll struct {
	Token string `json:"token" validate:"required"`
}

// MFAEnrollVerify completes forced enrollment during login.
type MFAEnrollVerify struct {
	Token string `json:"token" validate:"required"`
	Code  string `json:"code" validate:"required,len=6,numeric"`
}

// MFACode is a bare code for the voluntary enable step.
type MFACode struct {
	Code string `json:"code" validate:"required,len=6,numeric"`
}

// PasswordChange is the forced first-login password step.
type PasswordChange struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

// SIPSetup is the forced first-login SIP step; skipped when a manager
// pre-provisioned the extension.
type SIPSetup struct {
	Token     string `json:"token" validate:"required"`
	Extension string `json:"extension" validate:"required,min=2,max=32"`
	Password  string `json:"password" validate:"required,min=8,max=128"`
}
