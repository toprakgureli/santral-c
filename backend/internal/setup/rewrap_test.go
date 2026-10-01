package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"

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
	// Ids come from the sequences, so nothing collides with rows other
	// tests add to the same database at the same time.
	stamp := time.Now().UnixNano()
	var channel uint
	if err := db.Raw(`INSERT INTO wa_channels (name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, settings)
		VALUES ('rewrap', ?, ?, 'v', ?, ?, ?, ?::jsonb) RETURNING id`,
		fmt.Sprintf("rw%d", stamp), fmt.Sprintf("rw%d", stamp), fmt.Sprintf("rewrap-%d", stamp),
		legacy(testLegacyWA, "EAAG-token"), legacy(testLegacyWA, "app-secret"), string(settings)).Scan(&channel).Error; err != nil {
		t.Fatal(err)
	}
	var integrations []uint
	if err := db.Raw(`INSERT INTO wa_integrations (name, method, url, headers_enc) VALUES ('legacy', 'GET', 'https://example.com', ?),
		('rotated', 'GET', 'https://example.com', ?), ('broken', 'GET', 'https://example.com', 'not-a-secret') RETURNING id`,
		legacy(testLegacyWA, "legacy-headers"), rotated).Scan(&integrations).Error; err != nil || len(integrations) != 3 {
		t.Fatalf("integrations: %v %v", integrations, err)
	}
	slices.Sort(integrations)
	legacyHeaders, rotatedHeaders, brokenHeaders := integrations[0], integrations[1], integrations[2]
	// The two shared settings are put back as they were, not removed.
	restore := keepSetting(t, db, "wa_global_settings", "key", "ai")
	restoreDrive := keepSetting(t, db, "system_settings", "key", "drive_token")
	exec(`INSERT INTO wa_global_settings (key, value) VALUES ('ai', ?::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, string(ai))
	exec(`INSERT INTO system_settings (key, value, updated_at) VALUES ('drive_token', ?, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, legacy(testLegacyDrive, "drive-refresh"))
	t.Cleanup(func() {
		db.Exec("DELETE FROM wa_channels WHERE id = ?", channel)
		db.Exec("DELETE FROM wa_integrations WHERE id IN ?", integrations)
		restore()
		restoreDrive()
	})

	ring, err := crypt.NewKeyring(testDataKey, testOldDataKey)
	if err != nil {
		t.Fatal(err)
	}
	legacyKeys := LegacyKeys{WhatsApp: testLegacyWA, Drive: testLegacyDrive}
	if err := RewrapSecrets(ctx, db, ring, testPurposes, legacyKeys); err != nil {
		t.Fatalf("RewrapSecrets() = %v", err)
	}

	read := func(q string, args ...any) string {
		t.Helper()
		var v string
		if err := db.Raw(q, args...).Scan(&v).Error; err != nil {
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
	open("whatsapp", read("SELECT access_token_enc FROM wa_channels WHERE id = ?", channel), "EAAG-token")
	open("whatsapp", read("SELECT app_secret_enc FROM wa_channels WHERE id = ?", channel), "app-secret")
	open("whatsapp", read("SELECT settings->'survey'->>'secretEnc' FROM wa_channels WHERE id = ?", channel), "tally-secret")
	if mode := read("SELECT settings->'survey'->>'mode' FROM wa_channels WHERE id = ?", channel); mode != "tally" {
		t.Fatalf("the rest of the settings changed: mode = %q", mode)
	}
	open("whatsapp", read("SELECT headers_enc FROM wa_integrations WHERE id = ?", legacyHeaders), "legacy-headers")
	open("whatsapp", read("SELECT headers_enc FROM wa_integrations WHERE id = ?", rotatedHeaders), "rotated-headers")
	open("whatsapp", read("SELECT value->>'keyEnc' FROM wa_global_settings WHERE key = 'ai'"), "sk-ai-key")
	open("drive", read("SELECT value FROM system_settings WHERE key = 'drive_token'"), "drive-refresh")
	if v := read("SELECT headers_enc FROM wa_integrations WHERE id = ?", brokenHeaders); v != "not-a-secret" {
		t.Fatalf("an unreadable value was changed: %q", v)
	}

	// A second run finds nothing to do and changes nothing.
	before := read("SELECT access_token_enc FROM wa_channels WHERE id = ?", channel)
	if err := RewrapSecrets(ctx, db, ring, testPurposes, legacyKeys); err != nil {
		t.Fatalf("second RewrapSecrets() = %v", err)
	}
	if after := read("SELECT access_token_enc FROM wa_channels WHERE id = ?", channel); after != before {
		t.Fatal("a current value was sealed again")
	}
}

// keepSetting remembers one row of a key/value settings table and returns a
// function that puts it back as it was, or removes it when there was none.
func keepSetting(t *testing.T, db *gorm.DB, table, keyColumn, key string) func() {
	t.Helper()
	var rows []map[string]any
	if err := db.Table(table).Where(keyColumn+" = ?", key).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return func() {
		db.Exec("DELETE FROM "+table+" WHERE "+keyColumn+" = ?", key)
		for _, r := range rows {
			db.Table(table).Create(r)
		}
	}
}
