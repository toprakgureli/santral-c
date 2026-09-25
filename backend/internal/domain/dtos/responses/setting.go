package responses

// Settings is the public view of the runtime system flags. ClientIP is the
// address the admin is reading them from, so it can be trusted in one click.
type Settings struct {
	MFAMode       string   `json:"mfaMode"`
	MFATrustedIPs []string `json:"mfaTrustedIps"`
	ClientIP      string   `json:"clientIp,omitempty"`
}
