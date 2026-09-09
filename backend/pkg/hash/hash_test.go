package hash

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	encoded, err := Password("correct horse battery")
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	if !Compare(encoded, "correct horse battery") {
		t.Fatal("Compare rejected the correct password")
	}
	if Compare(encoded, "wrong password") {
		t.Fatal("Compare accepted a wrong password")
	}
}

func TestCompareRejectsMalformedHash(t *testing.T) {
	for _, bad := range []string{"", "plain", "$argon2id$bad", "$bcrypt$v=19$m=1,t=1,p=1$a$b"} {
		if Compare(bad, "x") {
			t.Fatalf("Compare accepted malformed hash %q", bad)
		}
	}
}

func TestPasswordProducesDistinctHashes(t *testing.T) {
	a, _ := Password("same")
	b, _ := Password("same")
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestTokenAndSHA256(t *testing.T) {
	tok, err := Token(32)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if len(tok) == 0 {
		t.Fatal("Token returned empty string")
	}
	if SHA256("x") == SHA256("y") {
		t.Fatal("SHA256 collided on different inputs")
	}
	if len(SHA256("x")) != 64 {
		t.Fatal("SHA256 must be 64 hex chars")
	}
}
