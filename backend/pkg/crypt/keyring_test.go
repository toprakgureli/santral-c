package crypt

import (
	"errors"
	"strings"
	"testing"
)

const (
	keyA = "0123456789abcdef0123456789abcdef-a"
	keyB = "0123456789abcdef0123456789abcdef-b"
)

func TestKeyringRoundTripAndPurposes(t *testing.T) {
	ring, err := NewKeyring(keyA)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := ring.Seal("whatsapp", "EAAG-token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "k") || strings.Contains(sealed, "EAAG") {
		t.Fatalf("sealed value looks wrong: %q", sealed)
	}
	got, err := ring.Open("whatsapp", sealed)
	if err != nil || got != "EAAG-token" {
		t.Fatalf("Open() = %q, %v", got, err)
	}
	if _, err := ring.Open("drive", sealed); err == nil {
		t.Fatal("a value opened under another purpose")
	}
	if !ring.Current(sealed) {
		t.Fatal("a fresh value is not current")
	}
}

func TestKeyringRotation(t *testing.T) {
	old, _ := NewKeyring(keyA)
	sealed, _ := old.Seal("drive", "refresh")

	rotated, err := NewKeyring(keyB, keyA)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Current(sealed) {
		t.Fatal("a value made with the previous key counts as current")
	}
	if got, err := rotated.Open("drive", sealed); err != nil || got != "refresh" {
		t.Fatalf("Open() with previous key = %q, %v", got, err)
	}

	withoutOld, _ := NewKeyring(keyB)
	if _, err := withoutOld.Open("drive", sealed); err == nil {
		t.Fatal("a value opened without its key")
	}
}

func TestKeyringLegacyAndShortKey(t *testing.T) {
	ring, _ := NewKeyring(keyA)
	legacy, _ := Encrypt("old-secret", "value")
	if _, err := ring.Open("whatsapp", legacy); !errors.Is(err, ErrNotSealed) {
		t.Fatalf("Open(legacy) error = %v, want ErrNotSealed", err)
	}
	if _, err := NewKeyring("short"); err == nil {
		t.Fatal("a short data key was accepted")
	}
}
