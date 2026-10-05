package configs

import (
	"strings"
	"testing"
	"time"
)

func TestOnlyAnExplicitTestValueIsNotLive(t *testing.T) {
	for _, v := range []string{"test", "Test", " development ", "DEVELOPMENT"} {
		if Development(v).IsLive() {
			t.Errorf("%q counted as live", v)
		}
	}
	for _, v := range []string{"live", "", "tset", "prod", "production", "dev"} {
		if !Development(v).IsLive() {
			t.Errorf("%q did not count as live", v)
		}
	}
}

// valid is a configuration a live server accepts.
func valid() Config {
	c := Config{
		App:      App{Development: Live, Host: "127.0.0.1", TrustedProxies: "127.0.0.1"},
		Auth:     Auth{Secret: strings.Repeat("a", 64), CookieSecure: true},
		Security: Security{DataKey: strings.Repeat("d", 64), MFAKey: "mfa-key", IPFailureLimit: 100},
		Database: Database{Host: "localhost", Name: "santral", User: "santral"},
		Redis:    Redis{Host: "localhost", Port: "6379"},
	}
	ApplyDefaults(&c)
	return c
}

func fatalKeys(c Config) []string {
	var keys []string
	for _, p := range Check(c) {
		if p.Fatal {
			keys = append(keys, p.Key)
		}
	}
	return keys
}

func TestValidLiveConfigPasses(t *testing.T) {
	if err := Validate(valid()); err != nil {
		t.Fatal(err)
	}
}

// TestMissingOrMistypedModeKeepsTheLiveChecks: a typo in app.development
// must not switch off what a live server needs (here: secure cookies).
func TestMissingOrMistypedModeKeepsTheLiveChecks(t *testing.T) {
	for _, mode := range []Development{"", "lvie", "prod"} {
		c := valid()
		c.App.Development = mode
		c.Auth.CookieSecure = false
		if keys := fatalKeys(c); !strings.Contains(strings.Join(keys, ","), "auth.cookieSecure") {
			t.Errorf("mode %q: insecure cookies were accepted (%v)", mode, keys)
		}
		unknown := false
		for _, p := range Check(c) {
			if p.Key == "app.development" {
				unknown = true
			}
		}
		if !unknown {
			t.Errorf("mode %q: the unknown value was not reported", mode)
		}
	}
	c := valid()
	c.App.Development = Test
	c.Auth.CookieSecure = false
	if err := Validate(c); err != nil {
		t.Errorf("a test setup was refused: %v", err)
	}
}

// TestPreviousDataKeysAreChecked: the check builds the same keyring the
// server builds at start, so a previous key that would stop the start
// stops the check too.
func TestPreviousDataKeysAreChecked(t *testing.T) {
	c := valid()
	c.Security.PreviousDataKeys = strings.Repeat("p", 64) + ", too-short"
	if keys := fatalKeys(c); !strings.Contains(strings.Join(keys, ","), "security.previousDataKeys") {
		t.Fatalf("a short previous key passed the check (%v)", keys)
	}
	c.Security.PreviousDataKeys = strings.Repeat("p", 64) + ", " + strings.Repeat("q", 40) + ","
	if err := Validate(c); err != nil {
		t.Fatalf("good previous keys were refused: %v", err)
	}
}

func TestDatabaseDefaults(t *testing.T) {
	c := valid()
	if c.Database.StatementTimeout != 30*time.Second || c.Database.DataPath != "/var/lib/postgresql" {
		t.Fatalf("defaults = %v %q", c.Database.StatementTimeout, c.Database.DataPath)
	}
}

func TestPreviousUserKeysMustDiffer(t *testing.T) {
	c := valid()
	c.Security.PreviousMFAKey = c.Security.MFAKey
	if keys := strings.Join(fatalKeys(c), ","); !strings.Contains(keys, "security.previousMfaKey") {
		t.Fatalf("a previous MFA key equal to the new one passed (%v)", keys)
	}
	c.Security.PreviousMFAKey = "change-me-to-a-32-byte-mfa-key"
	if err := Validate(c); err != nil {
		t.Fatalf("the example value as the previous key was refused: %v", err)
	}
}
