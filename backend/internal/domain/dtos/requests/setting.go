package requests

// SettingsUpdate changes the runtime system flags. MFAMode is "on" (everyone
// enrolls and is asked), "off" (nobody is asked) or "trusted" (nobody is
// asked from the listed addresses, everyone else as "on"). MFATrustedIPs are
// single addresses or CIDR blocks.
type SettingsUpdate struct {
	MFAMode       string   `json:"mfaMode" validate:"required,oneof=on off trusted"`
	MFATrustedIPs []string `json:"mfaTrustedIps" validate:"max=200,dive,max=64"`
}
