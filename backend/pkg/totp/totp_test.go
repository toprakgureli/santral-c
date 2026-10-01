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

func TestCodeMatchesValidate(t *testing.T) {
	key, err := Generate("santral-c", "agent@santral.local")
	if err != nil {
		t.Fatal(err)
	}
	now, err := Code(key.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(key.Secret, now) {
		t.Fatal("Validate rejected the code Code computed for now")
	}
	// One step on either side is accepted, two steps away is not.
	next, _ := Code(key.Secret, time.Now().Add(30*time.Second))
	if !Validate(key.Secret, next) {
		t.Fatal("Validate rejected the next step's code")
	}
	far, _ := Code(key.Secret, time.Now().Add(-5*time.Minute))
	if Validate(key.Secret, far) {
		t.Fatal("Validate accepted a code five minutes old")
	}
	if _, err := Code("not base32!", time.Now()); err == nil {
		t.Fatal("Code accepted a broken secret")
	}
}
