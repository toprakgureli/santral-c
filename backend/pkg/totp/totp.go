// Package totp generates and validates RFC 6238 TOTP secrets.
package totp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Key is a generated TOTP secret with its provisioning URL and QR image.
type Key struct {
	Secret string
	URL    string
	QR     string
}

// Generate creates a new TOTP key for an issuer and account.
func Generate(issuer, account string) (*Key, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, fmt.Errorf("totp key could not be generated: %w", err)
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return nil, fmt.Errorf("totp qr could not be rendered: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("totp qr could not be encoded: %w", err)
	}
	return &Key{
		Secret: key.Secret(),
		URL:    key.URL(),
		QR:     "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// Validate reports whether code matches secret within a one-step skew.
func Validate(secret, code string) bool {
	valid, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false
	}
	return valid
}
