package requests

// SettingsUpdate changes the runtime system flags.
type SettingsUpdate struct {
	MFARequired bool `json:"mfaRequired"`
}
