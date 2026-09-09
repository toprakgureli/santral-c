package crypt

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc, err := Encrypt("master-key", "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "JBSWY3DPEHPK3PXP" {
		t.Fatal("ciphertext equals plaintext")
	}
	dec, err := Decrypt("master-key", enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("round trip = %q", dec)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	enc, _ := Encrypt("master-key", "secret")
	if _, err := Decrypt("other-key", enc); err == nil {
		t.Fatal("Decrypt accepted the wrong key")
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	if _, err := Decrypt("master-key", "not-valid-ciphertext"); err == nil {
		t.Fatal("Decrypt accepted garbage input")
	}
}
