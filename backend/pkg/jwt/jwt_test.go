package jwt

import (
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/configs"
)

func testCfg() configs.Auth {
	return configs.Auth{Secret: "test-secret-please-change-me", Issuer: "santral-test", AccessTTL: time.Hour}
}

func TestGenerateAndParse(t *testing.T) {
	tok, err := Generate(testCfg(), 7)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	claims, err := Parse(testCfg(), tok.Value)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != 7 {
		t.Fatalf("UserID = %d, want 7", claims.UserID)
	}
	if claims.Purpose != PurposeAccess {
		t.Fatalf("Purpose = %q, want %q", claims.Purpose, PurposeAccess)
	}
	if claims.ID != tok.ID {
		t.Fatal("token id mismatch between generate and parse")
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	tok, _ := Generate(testCfg(), 1)
	bad := testCfg()
	bad.Secret = "a-different-secret"
	if _, err := Parse(bad, tok.Value); err == nil {
		t.Fatal("Parse accepted a token signed with a different secret")
	}
}

func TestParseRejectsWrongIssuer(t *testing.T) {
	tok, _ := Generate(testCfg(), 1)
	bad := testCfg()
	bad.Issuer = "someone-else"
	if _, err := Parse(bad, tok.Value); err == nil {
		t.Fatal("Parse accepted a token from a different issuer")
	}
}

func TestPurposesAreDistinct(t *testing.T) {
	cfg := testCfg()
	for purpose, gen := range map[string]func(configs.Auth, uint) (*Token, error){
		PurposeMFA:      GenerateMFA,
		PurposeEnroll:   GenerateEnroll,
		PurposePassword: GeneratePassword,
	} {
		tok, err := gen(cfg, 1)
		if err != nil {
			t.Fatalf("generate %s: %v", purpose, err)
		}
		claims, err := Parse(cfg, tok.Value)
		if err != nil {
			t.Fatalf("parse %s: %v", purpose, err)
		}
		if claims.Purpose != purpose {
			t.Fatalf("Purpose = %q, want %q", claims.Purpose, purpose)
		}
	}
}
