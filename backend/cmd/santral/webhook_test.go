package main

// Meta's notices: only a notice signed with the device's app secret is
// stored; anything else is turned away, nothing of it is kept, and the
// device says why in the panel. The same holds for a webhook address that
// was already registered in Meta and lives outside /api.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/internal/whatsapp"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// hookIP is the address the notices come from in these tests.
const hookIP = "198.51.100.30"

// device is a WhatsApp device as these tests set it up.
type device struct {
	id            uint
	phoneNumberID string
	hookKey       string
	secret        string // the app secret; empty when none is entered
	// A webhook already registered in Meta, or empty for the panel's own.
	existingURL    string
	existingPath   string
	verifyToken    string
	acceptUnsigned bool
}

var deviceRun atomic.Int64

// addDevice stores a device as the panel would, with its app secret sealed
// and no access token, so nothing tries to reach Meta. It is removed with
// whatever notices it got when the test ends.
func addDevice(t *testing.T, db *gorm.DB, d device) device {
	t.Helper()
	ring, err := crypt.NewKeyring(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	stamp := fmt.Sprintf("%d%02d", time.Now().UnixNano()%1_000_000_000_000, deviceRun.Add(1)%100)
	d.phoneNumberID = "77" + stamp
	d.hookKey = "hook-test-" + stamp
	sealed := ""
	if d.secret != "" {
		if sealed, err = ring.Seal(whatsapp.SealPurpose, d.secret); err != nil {
			t.Fatal(err)
		}
	}
	if d.existingURL != "" {
		d.existingPath = strings.TrimPrefix(d.existingURL, "https://panel.example")
	}
	if err := db.Raw(`INSERT INTO wa_channels (name, phone_number_id, waba_id, verify_token, hook_key, access_token_enc, app_secret_enc, active,
			existing_hook_url, existing_hook_path, existing_verify_token, accept_unsigned)
		VALUES ('Bildirim testi', ?, ?, 'own-verify', ?, '', ?, true, ?, ?, ?, ?) RETURNING id`,
		d.phoneNumberID, d.phoneNumberID, d.hookKey, sealed, d.existingURL, d.existingPath, d.verifyToken, d.acceptUnsigned).Scan(&d.id).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM wa_webhook_events WHERE channel_id = ?", d.id)
		db.Exec("DELETE FROM wa_channel_members WHERE channel_id = ?", d.id)
		if db.Exec("DELETE FROM wa_channels WHERE id = ?", d.id).Error != nil {
			db.Exec("UPDATE wa_channels SET active = false WHERE id = ?", d.id)
		}
	})
	return d
}

// notice is a message notice for the device's number, as Meta sends it.
func notice(phoneNumberID, text string) []byte {
	body, _ := json.Marshal(map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{map[string]any{"id": "waba", "changes": []any{map[string]any{"field": "messages", "value": map[string]any{
			"messaging_product": "whatsapp",
			"metadata":          map[string]any{"phone_number_id": phoneNumberID},
			"messages": []any{map[string]any{"from": "905551112233", "id": fmt.Sprintf("wamid.hook.%d", time.Now().UnixNano()),
				"timestamp": fmt.Sprint(time.Now().Unix()), "type": "text", "text": map[string]any{"body": text}}},
		}}}}},
	})
	return body
}

// sign is the signature header Meta sends with body for secret.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// postNotice sends a notice from ip and returns the answer code. An empty
// signature sends none.
func postNotice(t *testing.T, app *fiber.App, path, ip, signature string, body []byte) int {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	req.Header.Set(fiber.HeaderXForwardedFor, ip)
	res, err := app.Test(req, 30_000)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res.StatusCode
}

// stored counts the notices kept for a device.
func stored(t *testing.T, db *gorm.DB, id uint) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM wa_webhook_events WHERE channel_id = ?", id).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// warning is what the device shows in the panel about its notices.
func warning(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var s string
	if err := db.Raw("SELECT last_error FROM wa_channels WHERE id = ?", id).Scan(&s).Error; err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWebhookSignature(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	signed := addDevice(t, db, device{secret: "hook-app-secret"})
	noSecret := addDevice(t, db, device{})
	path := "/api/v1/wa/hook/" + signed.hookKey
	body := notice(signed.phoneNumberID, "Merhaba")

	// A correctly signed notice is stored and answered at once.
	if code := postNotice(t, srv.app, path, hookIP, sign(signed.secret, body), body); code != fiber.StatusOK {
		t.Fatalf("signed notice answered %d, want 200", code)
	}
	if n := stored(t, db, signed.id); n != 1 {
		t.Fatalf("%d notices stored after one signed notice, want 1", n)
	}
	var seen *time.Time
	db.Raw("SELECT last_webhook_at FROM wa_channels WHERE id = ?", signed.id).Scan(&seen)
	if seen == nil {
		t.Error("the device does not show when its last notice came")
	}

	// Everything else is turned away and nothing of it is kept.
	tooBig := append(append([]byte(`{"object":"whatsapp_business_account","pad":"`), []byte(strings.Repeat("x", 1<<20))...), []byte(`"}`)...)
	broken := []byte(`{"object": "whatsapp_business_account", "entry": [`)
	for _, tc := range []struct {
		name      string
		path      string
		signature string
		body      []byte
		want      int
	}{
		{"signed with another secret", path, sign("someone-elses-secret", body), body, fiber.StatusUnauthorized},
		{"no signature", path, "", body, fiber.StatusUnauthorized},
		{"signature of another body", path, sign(signed.secret, []byte("{}")), body, fiber.StatusUnauthorized},
		{"signature without its prefix", path, strings.TrimPrefix(sign(signed.secret, body), "sha256="), body, fiber.StatusUnauthorized},
		{"signed but not JSON", path, sign(signed.secret, broken), broken, fiber.StatusBadRequest},
		{"larger than 1 MB", path, sign(signed.secret, tooBig), tooBig, fiber.StatusRequestEntityTooLarge},
		{"unknown address", "/api/v1/wa/hook/no-such-key", sign(signed.secret, body), body, fiber.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := postNotice(t, srv.app, tc.path, hookIP, tc.signature, tc.body); code != tc.want {
				t.Errorf("answered %d, want %d", code, tc.want)
			}
			if n := stored(t, db, signed.id); n != 1 {
				t.Errorf("%d notices stored, want still 1", n)
			}
		})
	}
	if w := warning(t, db, signed.id); !strings.Contains(w, "imza tutmadı") {
		t.Errorf("after a wrongly signed notice the device shows %q, want the signature warning", w)
	}

	// A device without its app secret takes nothing, however it is signed,
	// and says why.
	ownPath := "/api/v1/wa/hook/" + noSecret.hookKey
	nb := notice(noSecret.phoneNumberID, "Merhaba")
	if code := postNotice(t, srv.app, ownPath, hookIP, sign("any-secret", nb), nb); code != fiber.StatusForbidden {
		t.Errorf("a device without its app secret answered %d, want 403", code)
	}
	if n := stored(t, db, noSecret.id); n != 0 {
		t.Errorf("a device without its app secret stored %d notices", n)
	}
	if w := warning(t, db, noSecret.id); !strings.Contains(w, "App secret") {
		t.Errorf("a device without its app secret shows %q, want a warning naming the App secret", w)
	}

	// Meta's check of the address.
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"hub.mode=subscribe&hub.verify_token=own-verify&hub.challenge=12345", fiber.StatusOK},
		{"hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=12345", fiber.StatusForbidden},
		{"hub.mode=unsubscribe&hub.verify_token=own-verify&hub.challenge=12345", fiber.StatusForbidden},
	} {
		res := call(t, srv.app, fiber.MethodGet, path+"?"+tc.query, "", nil)
		got, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != tc.want || (tc.want == fiber.StatusOK && string(got) != "12345") {
			t.Errorf("verify %s answered %d %q, want %d", tc.query, res.StatusCode, got, tc.want)
		}
	}
}

// TestWebhookPerAddressLimit: one address may send 3,000 notices a minute,
// a busy hour from a single Meta address; past that it is refused, while
// another address still gets through.
func TestWebhookPerAddressLimit(t *testing.T) {
	srv, _ := testServer(t, officeSecurity)
	const limit = 3000 // hookRatePerMin in the whatsapp router
	path := "/api/v1/wa/hook/limit-test-unknown"
	body := []byte(`{}`)
	var refused atomic.Int64
	sem := make(chan struct{}, 16)
	together(limit, func(int) {
		sem <- struct{}{}
		defer func() { <-sem }()
		if code := postNotice(t, srv.app, path, "198.51.100.31", "", body); code == fiber.StatusTooManyRequests {
			refused.Add(1)
		}
	})
	if n := refused.Load(); n != 0 {
		t.Fatalf("%d of the first %d notices from one address were refused", n, limit)
	}
	if code := postNotice(t, srv.app, path, "198.51.100.31", "", body); code != fiber.StatusTooManyRequests {
		t.Errorf("notice %d from the same address answered %d, want 429", limit+1, code)
	}
	if code := postNotice(t, srv.app, path, "198.51.100.32", "", body); code != fiber.StatusNotFound {
		t.Errorf("another address answered %d, want 404 for the unknown device", code)
	}
}

func TestExistingWebhook(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	stamp := time.Now().UnixNano()
	at := func(name string) string { return fmt.Sprintf("https://panel.example/webhook/%s-%d", name, stamp) }
	signed := addDevice(t, db, device{secret: "existing-secret", existingURL: at("signed"), verifyToken: "existing-verify"})
	noSecret := addDevice(t, db, device{existingURL: at("nosecret")})
	unsigned := addDevice(t, db, device{existingURL: at("unsigned"), acceptUnsigned: true})
	// Two numbers of one Meta app share an address; a notice goes to the
	// number it names.
	first := addDevice(t, db, device{secret: "shared-secret", existingURL: at("shared")})
	second := addDevice(t, db, device{secret: "shared-secret", existingURL: at("shared")})

	// Saving a device in the panel makes the server learn the new
	// addresses at once instead of within half a minute.
	admin := signIn(t, srv, db, enums.RoleInvisibleAdmin)
	res := call(t, srv.app, fiber.MethodPatch, fmt.Sprintf("/api/v1/wa/channels/%d", signed.id),
		fmt.Sprintf(`{"name":"Bildirim testi","existingHookUrl":%q,"existingVerifyToken":"existing-verify","acceptUnsigned":false}`, signed.existingURL), admin)
	_ = res.Body.Close()
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("saving the device answered %d", res.StatusCode)
	}

	body := notice(signed.phoneNumberID, "Merhaba")
	if code := postNotice(t, srv.app, signed.existingPath, hookIP, sign(signed.secret, body), body); code != fiber.StatusOK {
		t.Fatalf("a signed notice on the registered address answered %d, want 200", code)
	}
	if n := stored(t, db, signed.id); n != 1 {
		t.Fatalf("%d notices stored, want 1", n)
	}
	tooBig := []byte(`{"pad":"` + strings.Repeat("x", 1<<20) + `"}`)
	for _, tc := range []struct {
		name      string
		signature string
		body      []byte
		want      int
	}{
		{"without a signature where the device has a secret", "", body, fiber.StatusUnauthorized},
		{"signed with another secret", sign("other", body), body, fiber.StatusUnauthorized},
		{"larger than 1 MB", sign(signed.secret, tooBig), tooBig, fiber.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := postNotice(t, srv.app, signed.existingPath, hookIP, tc.signature, tc.body); code != tc.want {
				t.Errorf("answered %d, want %d", code, tc.want)
			}
			if n := stored(t, db, signed.id); n != 1 {
				t.Errorf("%d notices stored, want still 1", n)
			}
		})
	}
	if w := warning(t, db, signed.id); !strings.Contains(w, "imza tutmadı") {
		t.Errorf("the device shows %q, want the signature warning", w)
	}

	// No secret and unsigned notices not allowed: refused, with a warning
	// that says what to do.
	nb := notice(noSecret.phoneNumberID, "Merhaba")
	if code := postNotice(t, srv.app, noSecret.existingPath, hookIP, "", nb); code != fiber.StatusForbidden {
		t.Errorf("a device that needs a signature took an unsigned notice: %d, want 403", code)
	}
	if n := stored(t, db, noSecret.id); n != 0 {
		t.Errorf("%d notices stored on a device that needs a signature", n)
	}
	if w := warning(t, db, noSecret.id); !strings.Contains(w, "imzasız bildirimleri kabul et") {
		t.Errorf("the device shows %q, want the warning naming the unsigned option", w)
	}

	// Unsigned notices allowed: stored, but still only real JSON.
	ub := notice(unsigned.phoneNumberID, "Merhaba")
	if code := postNotice(t, srv.app, unsigned.existingPath, hookIP, "", ub); code != fiber.StatusOK {
		t.Errorf("an unsigned notice where they are allowed answered %d, want 200", code)
	}
	if code := postNotice(t, srv.app, unsigned.existingPath, hookIP, "", []byte("not json")); code != fiber.StatusBadRequest {
		t.Errorf("broken JSON answered %d, want 400", code)
	}
	if n := stored(t, db, unsigned.id); n != 1 {
		t.Errorf("%d notices stored on the unsigned device, want 1", n)
	}

	// The shared address.
	sb := notice(second.phoneNumberID, "İkinci numaraya")
	if code := postNotice(t, srv.app, first.existingPath, hookIP, sign("shared-secret", sb), sb); code != fiber.StatusOK {
		t.Errorf("a notice on the shared address answered %d", code)
	}
	if a, b := stored(t, db, first.id), stored(t, db, second.id); a != 0 || b != 1 {
		t.Errorf("the notice for the second number went to first=%d second=%d, want 0 and 1", a, b)
	}

	// Meta's check of the registered address, and a path no device uses.
	for _, tc := range []struct {
		path string
		want int
	}{
		{signed.existingPath + "?hub.mode=subscribe&hub.verify_token=existing-verify&hub.challenge=777", fiber.StatusOK},
		{signed.existingPath + "?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=777", fiber.StatusForbidden},
		{"/webhook/nobody-uses-this", fiber.StatusNotFound},
	} {
		res := call(t, srv.app, fiber.MethodGet, tc.path, "", nil)
		_ = res.Body.Close()
		if res.StatusCode != tc.want {
			t.Errorf("GET %s answered %d, want %d", tc.path, res.StatusCode, tc.want)
		}
	}
	if code := postNotice(t, srv.app, "/webhook/nobody-uses-this", hookIP, "", ub); code != fiber.StatusNotFound {
		t.Errorf("a notice on a path no device uses answered %d, want 404", code)
	}
	// Signed in or not makes no difference on a registered address.
	req := httptest.NewRequest(fiber.MethodPost, signed.existingPath, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(fiber.HeaderXForwardedFor, hookIP)
	for _, c := range admin {
		req.AddCookie(c)
	}
	res, err := srv.app.Test(req, 30_000)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != fiber.StatusUnauthorized || res.Header.Get(fiber.HeaderWWWAuthenticate) != "" {
		t.Errorf("an unsigned notice with a panel session answered %d (%q), want the signature refusal", res.StatusCode, res.Header.Get(fiber.HeaderWWWAuthenticate))
	}
}
