package setup

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
)

// Replacing mfaKey or sipKey keeps every authenticator and phone line: the
// old values open with the previous key and are sealed with the new one.
func TestRewrapUserSecrets(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	const oldMFA, newMFA = "change-me-to-a-32-byte-mfa-key", "new-mfa-key-0123456789abcdef"
	const oldSIP, newSIP = "change-me-to-a-32-byte-sip-key", "new-sip-key-0123456789abcdef"
	seal := func(key, plain string) string {
		t.Helper()
		v, err := crypt.Encrypt(key, plain)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	stamp := time.Now().UnixNano()
	var ids []uint
	if err := db.Raw(`INSERT INTO users (name, email, password, mfa_secret, sip_extension, sip_secret) VALUES
		('old keys', ?, 'x', ?, ?, ?),
		('new keys', ?, 'x', ?, NULL, NULL),
		('lost', ?, 'x', 'not-a-secret', NULL, NULL)
		RETURNING id`,
		fmt.Sprintf("old-%d@test.local", stamp), seal(oldMFA, "TOTPSECRET1"), fmt.Sprintf("9%d", stamp%1000000), seal(oldSIP, "sip-pass-1"),
		fmt.Sprintf("new-%d@test.local", stamp), seal(newMFA, "TOTPSECRET2"),
		fmt.Sprintf("lost-%d@test.local", stamp)).Scan(&ids).Error; err != nil || len(ids) != 3 {
		t.Fatalf("users: %v %v", ids, err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id IN ?", ids) })
	oldUser, newUser, lostUser := ids[0], ids[1], ids[2]

	read := func(column string, id uint) string {
		t.Helper()
		var v string
		if err := db.Raw(fmt.Sprintf("SELECT COALESCE(%s, '') FROM users WHERE id = ?", column), id).Scan(&v).Error; err != nil {
			t.Fatal(err)
		}
		return v
	}
	opens := func(key, column string, id uint, want string) {
		t.Helper()
		got, err := crypt.Decrypt(key, read(column, id))
		if err != nil || got != want {
			t.Fatalf("users.%s of %d = %q, %v; want %q under the new key", column, id, got, err, want)
		}
	}
	keys := UserKeys{MFA: newMFA, PreviousMFA: oldMFA, SIP: newSIP, PreviousSIP: oldSIP}
	untouched := read("mfa_secret", newUser)
	if err := RewrapUserSecrets(ctx, db, keys); err != nil {
		t.Fatalf("RewrapUserSecrets() = %v", err)
	}
	opens(newMFA, "mfa_secret", oldUser, "TOTPSECRET1")
	opens(newSIP, "sip_secret", oldUser, "sip-pass-1")
	opens(newMFA, "mfa_secret", newUser, "TOTPSECRET2")
	if read("mfa_secret", newUser) != untouched {
		t.Fatal("a value already under the new key was sealed again")
	}
	if v := read("mfa_secret", lostUser); v != "not-a-secret" {
		t.Fatalf("an unreadable value was changed: %q", v)
	}

	// A second run, and a run without a previous key, change nothing.
	moved := read("mfa_secret", oldUser)
	if err := RewrapUserSecrets(ctx, db, UserKeys{MFA: newMFA, SIP: newSIP}); err != nil {
		t.Fatalf("second RewrapUserSecrets() = %v", err)
	}
	if read("mfa_secret", oldUser) != moved {
		t.Fatal("a current value changed on the second run")
	}
}
