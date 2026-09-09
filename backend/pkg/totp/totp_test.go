package totp

import (
	"strings"
	"testing"
	"time"

	otplib "github.com/pquerna/otp/totp"
)

func TestGenerateAndValidate(t *testing.T) {
	key, err := Generate("santral-c", "agent@santral.local")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(key.QR, "data:image/png;base64,") {
		t.Fatal("QR is not a PNG data URL")
	}

	code, err := otplib.GenerateCode(key.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !Validate(key.Secret, code) {
		t.Fatal("Validate rejected the current code")
	}

	stale, err := otplib.GenerateCode(key.Secret, time.Now().Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("GenerateCode stale: %v", err)
	}
	if Validate(key.Secret, stale) {
		t.Fatal("Validate accepted a code well outside the skew window")
	}
}
