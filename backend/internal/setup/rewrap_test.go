package setup

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
)

const (
	testDataKey     = "test-data-key-0123456789abcdef-0123456789"
	testOldDataKey  = "test-old-data-key-0123456789abcdef-012345"
	testLegacyWA    = "wa:legacy-session-secret"
	testLegacyDrive = "legacy-session-secret"
)

var testPurposes = SecretPurposes{WhatsApp: "whatsapp", Drive: "drive"}

func TestRewrapSecrets(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()

	legacy := func(key, plain string) string {
		t.Helper()
		v, err := crypt.Encrypt(key, plain)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	oldRing, err := crypt.NewKeyring(testOldDataKey)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := oldRing.Seal("whatsapp", "rotated-headers")
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{"survey": map[string]any{"mode": "tally", "secretEnc": legacy(testLegacyWA, "tally-secret")}})
	ai, _ := json.Marshal(map[string]any{"enabled": true, "keyEnc": legacy(testLegacyWA, "sk-ai-key")})

	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO wa_channels (id, name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, settings)
		VALUES (97001, 'rewrap', '97001', '97001', 'v', 'rewrap-97001', ?, ?, ?::jsonb)`,
		legacy(testLegacyWA, "EAAG-token"), legacy(testLegacyWA, "app-secret"), string(settings))
	exec(`INSERT INTO wa_integrations (id, name, method, url, headers_enc) VALUES (97001, 'legacy', 'GET', 'https://example.com', ?),
		(97002, 'rotated', 'GET', 'https://example.com', ?), (97003, 'broken', 'GET', 'https://example.com', 'not-a-secret')`,
		legacy(testLegacyWA, "legacy-headers"), rotated)
	exec(`INSERT INTO wa_global_settings (key, value) VALUES ('ai', ?::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, string(ai))
	exec(`INSERT INTO system_settings (key, value, updated_at) VALUES ('drive_token', ?, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, legacy(testLegacyDrive, "drive-refresh"))
	t.Cleanup(func() {
		db.Exec("DELETE FROM wa_channels WHERE id = 97001")
		db.Exec("DELETE FROM wa_integrations WHERE id IN (97001, 97002, 97003)")
		db.Exec("DELETE FROM wa_global_settings WHERE key = 'ai'")
		db.Exec("DELETE FROM system_settings WHERE key = 'drive_token'")
	})

	ring, err := crypt.NewKeyring(testDataKey, testOldDataKey)
	if err != nil {
		t.Fatal(err)
	}
	legacyKeys := LegacyKeys{WhatsApp: testLegacyWA, Drive: testLegacyDrive}
	if err := RewrapSecrets(ctx, db, ring, testPurposes, legacyKeys); err != nil {
		t.Fatalf("RewrapSecrets() = %v", err)
	}

	read := func(q string) string {
		t.Helper()
		var v string
		if err := db.Raw(q).Scan(&v).Error; err != nil {
			t.Fatal(err)
		}
		return v
	}
	open := func(purpose, v, want string) {
		t.Helper()
		if !ring.Current(v) {
			t.Fatalf("%q is not sealed with the current key", want)
		}
		got, err := ring.Open(purpose, v)
		if err != nil || got != want {
			t.Fatalf("Open() = %q, %v; want %q", got, err, want)
		}
	}
	open("whatsapp", read("SELECT access_token_enc FROM wa_channels WHERE id = 97001"), "EAAG-token")
	open("whatsapp", read("SELECT app_secret_enc FROM wa_channels WHERE id = 97001"), "app-secret")
	open("whatsapp", read("SELECT settings->'survey'->>'secretEnc' FROM wa_channels WHERE id = 97001"), "tally-secret")
	if mode := read("SELECT settings->'survey'->>'mode' FROM wa_channels WHERE id = 97001"); mode != "tally" {
		t.Fatalf("the rest of the settings changed: mode = %q", mode)
	}
	open("whatsapp", read("SELECT headers_enc FROM wa_integrations WHERE id = 97001"), "legacy-headers")
	open("whatsapp", read("SELECT headers_enc FROM wa_integrations WHERE id = 97002"), "rotated-headers")
	open("whatsapp", read("SELECT value->>'keyEnc' FROM wa_global_settings WHERE key = 'ai'"), "sk-ai-key")
	open("drive", read("SELECT value FROM system_settings WHERE key = 'drive_token'"), "drive-refresh")
	if v := read("SELECT headers_enc FROM wa_integrations WHERE id = 97003"); v != "not-a-secret" {
		t.Fatalf("an unreadable value was changed: %q", v)
	}

	// A second run finds nothing to do and changes nothing.
	before := read("SELECT access_token_enc FROM wa_channels WHERE id = 97001")
	if err := RewrapSecrets(ctx, db, ring, testPurposes, legacyKeys); err != nil {
		t.Fatalf("second RewrapSecrets() = %v", err)
	}
	if after := read("SELECT access_token_enc FROM wa_channels WHERE id = 97001"); after != before {
		t.Fatal("a current value was sealed again")
	}
}
