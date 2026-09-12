package responses

// Settings is the public view of the runtime system flags.
type Settings struct {
	MFARequired bool `json:"mfaRequired"`
}
