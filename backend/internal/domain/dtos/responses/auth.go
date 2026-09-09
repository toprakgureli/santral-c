package responses

// MFASetup carries a freshly generated TOTP secret and its QR image.
type MFASetup struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
	QR     string `json:"qr"`
}

// Login is a successful authentication response.
type Login struct {
	User User `json:"user"`
}

// LoginChallenge asks the client for a second step before a session is issued.
type LoginChallenge struct {
	MFARequired            bool   `json:"mfaRequired,omitempty"`
	MFASetupRequired       bool   `json:"mfaSetupRequired,omitempty"`
	MFAToken               string `json:"mfaToken,omitempty"`
	PasswordChangeRequired bool   `json:"passwordChangeRequired,omitempty"`
	PasswordToken          string `json:"passwordToken,omitempty"`
}
