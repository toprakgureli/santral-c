package responses

// MFASetup carries a freshly generated TOTP secret and its QR image.
type MFASetup struct {
	Secret string `json:"secret"`
	URL    string `json:"url"`
	QR     string `json:"qr"`
}
