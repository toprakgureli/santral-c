package main

// Security, round three: the owner account (invisible admin) stays out of
// every list, a colleague's figures need a performance permission, requests
// that change something must come from the panel's own pages, and a stolen
// refresh token that comes back ends its session.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
)

// secRun tells this file's rows apart from other runs'.
var secRun = strconv.FormatInt(time.Now().UnixNano(), 36)

// person creates an active user holding roles who signs in with
// loadPassword; withExt gives them an extension.
func person(t *testing.T, db *gorm.DB, name string, withExt bool, roles ...models.Role) models.User {
	t.Helper()
	pw, err := hash.Password(loadPassword)
	if err != nil {
		t.Fatal(err)
	}
	n := loadRun.Add(1)
	u := models.User{
		Name:      fmt.Sprintf("%s %s-%d", name, secRun, n),
		Email:     fmt.Sprintf("r3sec-%s-%d@route-test.local", secRun, n),
		Password:  pw,
		Active:    true,
		MFAExempt: true,
		Roles:     roles,
	}
	if withExt {
		ext := freeExtension(t, db)
		u.SIPExtension = &ext
	}
	if err := db.Omit("Roles.*").Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("UPDATE users SET active = false, sip_extension = NULL WHERE id = ?", u.ID)
		db.Exec("DELETE FROM sessions WHERE user_id = ?", u.ID)
		db.Exec("DELETE FROM user_roles WHERE user_id = ?", u.ID)
	})
	return u
}

// signedIn signs u in and returns their browser.
func signInAs(t *testing.T, srv *server, u models.User) *browser {
	t.Helper()
	b := newBrowser(t, srv.app, nil, u)
	b.mustSignIn()
	return b
}

// raw sends one request with the given cookies and headers.
func raw(t *testing.T, app *fiber.App, method, path, body string, jar map[string]*http.Cookie, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range jar {
		req.AddCookie(c)
	}
	res, err := app.Test(req, 30_000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	out, _ := io.ReadAll(res.Body)
	return res, out
}

func idsIn(t *testing.T, a answer, field string) map[uint]bool {
	t.Helper()
	var page struct {
		Items []map[string]any `json:"items"`
	}
	a.json(t, &page)
	out := map[uint]bool{}
	for _, it := range page.Items {
		if v, ok := it[field].(float64); ok {
			out[uint(v)] = true
		}
	}
	return out
}

// aboutUser reports whether an audit page holds an entry made by the user
// or aimed at their account.
func aboutUser(t *testing.T, a answer, id uint) bool {
	t.Helper()
	var page struct {
		Items []struct {
			ActorID    *uint  `json:"actorId"`
			TargetType string `json:"targetType"`
			TargetID   string `json:"targetId"`
		} `json:"items"`
	}
	a.json(t, &page)
	for _, it := range page.Items {
		if (it.ActorID != nil && *it.ActorID == id) || (it.TargetType == "user" && it.TargetID == strconv.FormatUint(uint64(id), 10)) {
			return true
		}
	}
	return false
}

func totalOf(t *testing.T, a answer) int64 {
	t.Helper()
	var page struct {
		Total int64 `json:"total"`
	}
	a.json(t, &page)
	return page.Total
}

// TestHiddenOwnerStaysOutOfLists: someone with every viewing permission
// does not find the owner account in the audit trail, the sign-in records,
// the chat's people, the team page, the escalation filter or a profile;
// another invisible admin does.
func TestHiddenOwnerStaysOutOfLists(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	admin := systemRole(t, db, enums.RoleInvisibleAdmin)
	watcher := customRole(t, db, enums.SystemAuditView, enums.SystemLogs, enums.TeamsView, enums.PerformanceViewAll, enums.EscalationListAll)
	owner := person(t, db, "Sahip", true, admin)
	other := person(t, db, "Diğer sahip", false, admin)
	viewer := person(t, db, "Bakan", false, watcher)

	// The owner signs in once with a typo, then properly.
	ob := newBrowser(t, srv.app, nil, owner)
	if a := ob.login("wrong-password-1"); a.status != fiber.StatusUnauthorized {
		t.Fatalf("wrong password answered %d", a.status)
	}
	ob.mustSignIn()
	ownerID := strconv.FormatUint(uint64(owner.ID), 10)
	entries := []models.AuditLog{
		{ActorID: &owner.ID, Action: enums.AuditUserUpdated, TargetType: "user", TargetID: strconv.FormatUint(uint64(viewer.ID), 10), Detail: "{}"},
		{ActorID: &viewer.ID, Action: enums.AuditUserUpdated, TargetType: "user", TargetID: ownerID, Detail: "{}"},
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	esc := models.CallEscalation{NumberKey: "5550001122", Number: "05550001122", CategoryName: "Deneme", ReasonName: "Deneme", AgentID: &owner.ID, AgentName: owner.Name, CreatedAt: time.Now()}
	if err := db.Create(&esc).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM audit_log WHERE id IN ?", []uint{entries[0].ID, entries[1].ID})
		db.Exec("DELETE FROM call_escalations WHERE id = ?", esc.ID)
		db.Exec("DELETE FROM login_attempts WHERE email = ?", owner.Email)
	})

	vb, xb := signInAs(t, srv, viewer), signInAs(t, srv, other)
	type look struct {
		name string
		path string
		seen func(b *browser, a answer) bool
	}
	looks := []look{
		{"audit search by email", "/api/v1/audit?query=" + owner.Email, func(_ *browser, a answer) bool { return totalOf(t, a) > 0 }},
		// The id is a short number other tests' entries may contain too, so
		// only an entry by or about the owner counts as seeing them.
		{"audit search by target", "/api/v1/audit?query=" + ownerID, func(_ *browser, a answer) bool { return aboutUser(t, a, owner.ID) }},
		{"sign-in records", "/api/v1/security/attempts?email=" + owner.Email, func(_ *browser, a answer) bool { return totalOf(t, a) > 0 }},
		{"chat people", "/api/v1/teams/people", func(_ *browser, a answer) bool { return idsIn(t, a, "id")[owner.ID] }},
		{"team performance", "/api/v1/performance/today", func(_ *browser, a answer) bool { return idsIn(t, a, "userId")[owner.ID] }},
		{"escalation agents", "/api/v1/escalations/agents", func(_ *browser, a answer) bool { return idsIn(t, a, "id")[owner.ID] }},
		{"profile", "/api/v1/profile/" + ownerID, func(_ *browser, a answer) bool { return a.status == fiber.StatusOK }},
	}
	for _, l := range looks {
		a := vb.do(fiber.MethodGet, l.path, nil)
		if a.status != fiber.StatusOK && !(l.name == "profile" && a.status == fiber.StatusNotFound) {
			t.Errorf("%s: viewer got %d %s", l.name, a.status, a.body)
		}
		if l.seen(vb, a) {
			t.Errorf("%s: a colleague sees the owner account: %s", l.name, a.body)
		}
		if a := xb.do(fiber.MethodGet, l.path, nil); !l.seen(xb, a) {
			t.Errorf("%s: another invisible admin does not see the owner account: %d %s", l.name, a.status, a.body)
		}
	}
	// A guessed id does not open a conversation with the owner either.
	if a := vb.do(fiber.MethodPost, "/api/v1/teams/dm/"+ownerID, nil); a.status != fiber.StatusNotFound {
		t.Errorf("opening a chat with the owner answered %d, want 404", a.status)
	}
	// The owner's own profile still opens for the owner.
	if a := ob.do(fiber.MethodGet, "/api/v1/profile/me", nil); a.status != fiber.StatusOK {
		t.Errorf("own profile answered %d", a.status)
	}
}

// TestProfileFiguresNeedPerformancePermission: the card is for everyone,
// the figures follow the team page's rule.
func TestProfileFiguresNeedPerformancePermission(t *testing.T) {
	srv, db := testServer(t)
	sales, tech := systemRole(t, db, enums.RoleSalesTeam), systemRole(t, db, enums.RoleTechnicalTeam)
	all := customRole(t, db, enums.PerformanceViewAll)
	nothing := customRole(t, db, enums.TeamsView)
	target := person(t, db, "Hedef", true, sales)
	teammate := person(t, db, "Takım arkadaşı", false, sales)
	otherTeam := person(t, db, "Başka ekip", false, tech)
	boss := person(t, db, "Herkesi gören", false, all)
	plain := person(t, db, "Yetkisiz", false, nothing)
	id := strconv.FormatUint(uint64(target.ID), 10)

	type card struct {
		Name         string          `json:"name"`
		Email        string          `json:"email"`
		Stats        json.RawMessage `json:"stats"`
		CanSeeRecord bool            `json:"canSeeRecord"`
	}
	for _, c := range []struct {
		who     models.User
		figures bool
	}{{teammate, true}, {boss, true}, {otherTeam, false}, {plain, false}} {
		b := signInAs(t, srv, c.who)
		a := b.do(fiber.MethodGet, "/api/v1/profile/"+id, nil)
		if a.status != fiber.StatusOK {
			t.Fatalf("%s: card answered %d %s", c.who.Name, a.status, a.body)
		}
		var p card
		a.json(t, &p)
		if p.Name != target.Name {
			t.Errorf("%s: card shows %q", c.who.Name, p.Name)
		}
		shows := p.CanSeeRecord || len(p.Stats) > 0 || p.Email != ""
		if shows != c.figures {
			t.Errorf("%s: figures shown = %v, want %v (%s)", c.who.Name, shows, c.figures, a.body)
		}
		rec := b.do(fiber.MethodGet, "/api/v1/profile/"+id+"/record?from=2026-01-01&to=2026-01-31", nil)
		want := fiber.StatusForbidden
		if c.figures {
			want = fiber.StatusOK
		}
		if rec.status != want {
			t.Errorf("%s: record answered %d, want %d", c.who.Name, rec.status, want)
		}
		if mine := b.do(fiber.MethodGet, "/api/v1/profile/me/record", nil); mine.status != fiber.StatusOK {
			t.Errorf("%s: own record answered %d", c.who.Name, mine.status)
		}
	}
}

// TestChangesComeFromThePanel: with the session cookies, a change needs
// the panel's own address and a JSON body; the call-log beacon (a JSON
// blob) still goes through.
func TestChangesComeFromThePanel(t *testing.T) {
	srv, db := testServer(t, func(c *configs.Config) { c.App.PublicURL = "https://cm.example.com" })
	u := person(t, db, "Panelden", false, systemRole(t, db, enums.RoleSalesTeam))
	b := signInAs(t, srv, u)
	jar := b.cookies()
	body := `{"headline":"Satış","bio":""}`
	for _, c := range []struct {
		name    string
		headers map[string]string
		body    string
		want    int
	}{
		{"panel origin, JSON", map[string]string{"Origin": "https://cm.example.com", "Content-Type": "application/json"}, body, fiber.StatusOK},
		{"same address it was sent to", map[string]string{"Origin": "http://example.com", "Content-Type": "application/json"}, body, fiber.StatusOK},
		{"neighbouring site", map[string]string{"Origin": "https://evil.example.com", "Content-Type": "application/json"}, body, fiber.StatusForbidden},
		{"neighbouring site, no Origin", map[string]string{"Sec-Fetch-Site": "same-site", "Content-Type": "application/json"}, body, fiber.StatusForbidden},
		{"form post", map[string]string{"Origin": "https://cm.example.com", "Content-Type": "application/x-www-form-urlencoded"}, "headline=x", fiber.StatusUnsupportedMediaType},
		{"text body", map[string]string{"Origin": "https://cm.example.com", "Content-Type": "text/plain"}, body, fiber.StatusUnsupportedMediaType},
	} {
		res, out := raw(t, srv.app, fiber.MethodPut, "/api/v1/profile/me", c.body, jar, c.headers)
		if res.StatusCode != c.want {
			t.Errorf("%s: answered %d, want %d: %s", c.name, res.StatusCode, c.want, out)
		}
	}
	// The softphone's last word from a closing tab: a JSON blob.
	beacon := `{"callId":"r3-beacon-` + secRun + `","phase":"end","direction":"outbound","peer":"05550009988","disposition":"no_answer"}`
	res, out := raw(t, srv.app, fiber.MethodPost, "/api/v1/calls/log/", beacon, jar,
		map[string]string{"Origin": "https://cm.example.com", "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"})
	if res.StatusCode == fiber.StatusForbidden || res.StatusCode == fiber.StatusUnsupportedMediaType || res.StatusCode >= 500 {
		t.Errorf("call-log beacon answered %d: %s", res.StatusCode, out)
	}
	db.Exec("DELETE FROM call_logs WHERE call_id = ?", "r3-beacon-"+secRun)
	// Without the cookies nothing is checked here: a webhook or a script
	// with a token is not a browser riding on someone's session.
	if res, _ := raw(t, srv.app, fiber.MethodPost, "/api/v1/wa/hook/r3-nothing", "x", nil,
		map[string]string{"Origin": "https://graph.facebook.com", "Content-Type": "text/plain"}); res.StatusCode == fiber.StatusForbidden || res.StatusCode == fiber.StatusUnsupportedMediaType {
		t.Errorf("a webhook without cookies answered %d", res.StatusCode)
	}
}

// TestRefreshCookieStaysOnTheAuthPath: the refresh cookie is only sent to
// the sign-in endpoints, and the older one kept for every path is removed.
func TestRefreshCookieStaysOnTheAuthPath(t *testing.T) {
	srv, db := testServer(t)
	u := person(t, db, "Çerez", false, systemRole(t, db, enums.RoleSalesTeam))
	req := httptest.NewRequest(fiber.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"`+u.Email+`","password":"`+loadPassword+`"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := srv.app.Test(req, 30_000)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	paths := map[string]string{}
	var refresh *http.Cookie
	for _, c := range res.Cookies() {
		paths[c.Name] = c.Path
		if c.Name == "sc_refresh" {
			refresh = c
		}
	}
	if paths["sc_access"] != "/" || paths["sc_refresh"] != "/api/v1/auth" {
		t.Fatalf("cookie paths %v; want the access cookie everywhere and the refresh cookie on /api/v1/auth", paths)
	}

	// A browser that still has the old refresh cookie for every path sends
	// both; the answer keeps the new one and removes the old one.
	stale := &http.Cookie{Name: "sc_refresh", Value: "stale-" + secRun}
	r := httptest.NewRequest(fiber.MethodPost, "/api/v1/auth/refresh", nil)
	r.Header.Add("Cookie", refresh.Name+"="+refresh.Value)
	r.Header.Add("Cookie", stale.Name+"="+stale.Value)
	res, err = srv.app.Test(r, 30_000)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("refresh with both cookies answered %d", res.StatusCode)
	}
	var renewed, removed bool
	for _, c := range res.Cookies() {
		if c.Name != "sc_refresh" {
			continue
		}
		switch {
		case c.Path == "/api/v1/auth" && c.Value != "":
			renewed = true
		case c.Path == "/" && (c.Value == "" || c.MaxAge < 0 || c.Expires.Before(time.Now())):
			removed = true
		}
	}
	if !renewed || !removed {
		t.Fatalf("renewed %v, old cookie removed %v; set-cookie %v", renewed, removed, res.Header.Values("Set-Cookie"))
	}
}

// TestReusedRefreshTokenEndsTheSession: a replaced refresh token that comes
// back after the grace ends the session for whoever holds either token,
// and is written in the audit trail. Inside the grace it still renews.
func TestReusedRefreshTokenEndsTheSession(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	u := person(t, db, "Çalınan", false, systemRole(t, db, enums.RoleSalesTeam))
	victim := signInAs(t, srv, u)
	// The thief copied every cookie, the browser's id too, and uses them
	// from another network.
	thief := newBrowser(t, srv.app, nil, u)
	thief.jar = victim.cookies()
	thief.ip = "198.51.100.66"
	old := victim.cookies()["sc_refresh"].Value

	// The thief renews first; the victim's tab, a moment later, still works.
	if a := thief.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusOK {
		t.Fatalf("thief renewing answered %d %s", a.status, a.body)
	}
	if a := victim.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusOK {
		t.Fatalf("renewing within the grace answered %d %s", a.status, a.body)
	}
	// The thief renews again, so the victim's token is two steps behind.
	db.Exec("UPDATE session_spent_tokens SET spent_at = spent_at - interval '2 minutes' WHERE token_hash = ?", hash.SHA256(old))
	if a := thief.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusOK {
		t.Fatalf("thief renewing again answered %d %s", a.status, a.body)
	}
	if a := thief.do(fiber.MethodGet, "/api/v1/auth/me", nil); a.status != fiber.StatusOK {
		t.Fatalf("thief signed in before the reuse: %d", a.status)
	}

	// The victim comes back with the old token after the grace.
	time.Sleep(1100 * time.Millisecond) // the cutoff has whole seconds
	if a := victim.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusUnauthorized {
		t.Fatalf("reused token answered %d, want 401", a.status)
	}
	var revoked int64
	db.Raw("SELECT count(*) FROM sessions WHERE user_id = ? AND revoked_at IS NOT NULL", u.ID).Scan(&revoked)
	if revoked != 1 {
		t.Fatalf("%d sessions revoked, want 1", revoked)
	}
	if a := thief.do(fiber.MethodGet, "/api/v1/auth/me", nil); a.status != fiber.StatusUnauthorized {
		t.Errorf("thief's access token still works: %d", a.status)
	}
	if a := thief.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusUnauthorized {
		t.Errorf("thief's refresh token still works: %d", a.status)
	}
	var logged int64
	db.Raw("SELECT count(*) FROM audit_log WHERE action = ? AND actor_id = ?", enums.AuditSessionReused, u.ID).Scan(&logged)
	if logged != 1 {
		t.Errorf("%d audit entries for the reuse, want 1", logged)
	}
	db.Exec("DELETE FROM audit_log WHERE actor_id = ?", u.ID)

	// Signing in again works (a moment later: tokens of the second the
	// session ended are refused).
	time.Sleep(1100 * time.Millisecond)
	again := newBrowser(t, srv.app, nil, u)
	again.mustSignIn()
	if a := again.do(fiber.MethodGet, "/api/v1/auth/me", nil); a.status != fiber.StatusOK {
		t.Fatalf("a new sign-in answered %d", a.status)
	}
}

// TestReusedTokenFromBeforeTheListIsCaught: a token replaced before every
// replaced token was remembered is still recognised as the previous one.
func TestReusedTokenFromBeforeTheListIsCaught(t *testing.T) {
	srv, db := testServer(t)
	u := person(t, db, "Eski kayıt", false, systemRole(t, db, enums.RoleSalesTeam))
	b := signInAs(t, srv, u)
	old := b.cookies()
	if a := b.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusOK {
		t.Fatalf("renewing answered %d", a.status)
	}
	db.Exec("DELETE FROM session_spent_tokens WHERE token_hash = ?", hash.SHA256(old["sc_refresh"].Value))
	db.Exec("UPDATE sessions SET rotated_at = now() - interval '5 minutes' WHERE user_id = ?", u.ID)
	stale := newBrowser(t, srv.app, nil, u)
	stale.jar = old
	if a := stale.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusUnauthorized {
		t.Fatalf("reused token answered %d", a.status)
	}
	if a := b.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusUnauthorized {
		t.Fatalf("the session survived the reuse: %d", a.status)
	}
	db.Exec("DELETE FROM audit_log WHERE actor_id = ?", u.ID)
}

// TestLogoutWithAJustReplacedToken: one tab renews while another signs
// out with the token it still had; the session ends all the same.
func TestLogoutWithAJustReplacedToken(t *testing.T) {
	srv, db := testServer(t)
	u := person(t, db, "Çıkış", false, systemRole(t, db, enums.RoleSalesTeam))
	tabA := signInAs(t, srv, u)
	tabB := newBrowser(t, srv.app, nil, u)
	tabB.jar = tabA.cookies()
	if a := tabA.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusOK {
		t.Fatalf("renewing answered %d", a.status)
	}
	if a := tabB.do(fiber.MethodPost, "/api/v1/auth/logout", nil); a.status != fiber.StatusNoContent {
		t.Fatalf("logout answered %d %s", a.status, a.body)
	}
	if a := tabA.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusUnauthorized {
		t.Fatalf("the renewed token still works after logout: %d", a.status)
	}
}

// TestEscalationSearchNeedsAWholeNumber: the search that reaches everyone's
// records does not answer for a few digits.
func TestEscalationSearchNeedsAWholeNumber(t *testing.T) {
	srv, db := testServer(t)
	tech := systemRole(t, db, enums.RoleTechnicalTeam)
	writer := person(t, db, "Kaydeden", false, systemRole(t, db, enums.RoleSalesTeam))
	searcher := person(t, db, "Arayan", false, tech)
	rows := []models.CallEscalation{
		{NumberKey: "123", Number: "123", CategoryName: "Deneme", ReasonName: "Kısa", AgentID: &writer.ID, AgentName: writer.Name, CreatedAt: time.Now()},
		{NumberKey: "5559876543", Number: "05559876543", CategoryName: "Deneme", ReasonName: "Tam", AgentID: &writer.ID, AgentName: writer.Name, CreatedAt: time.Now()},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM call_escalations WHERE id IN ?", []uint{rows[0].ID, rows[1].ID}) })
	b := signInAs(t, srv, searcher)
	if a := b.do(fiber.MethodGet, "/api/v1/escalations/list?number=123", nil); totalOf(t, a) != 0 {
		t.Errorf("a three-digit search listed others' records: %s", a.body)
	}
	if a := b.do(fiber.MethodGet, "/api/v1/escalations/list?number=05559876543", nil); totalOf(t, a) != 1 {
		t.Errorf("a whole number did not find its record: %s", a.body)
	}
	// Without a number the technical team sees only its own records.
	if a := b.do(fiber.MethodGet, "/api/v1/escalations/list", nil); idsIn(t, a, "agentId")[writer.ID] {
		t.Errorf("the own-records list shows someone else's: %s", a.body)
	}
}

// TestLostRenewalAnswerKeepsTheSession: the server replaces the token but
// its answer never reaches the browser, which keeps the old token. When
// that browser comes back with it later, from the same address, it gets a
// new token and stays signed in; the old token from another network still
// ends the session.
func TestLostRenewalAnswerKeepsTheSession(t *testing.T) {
	srv, db := testServer(t, officeSecurity)
	u := person(t, db, "Kopan bağlantı", false, systemRole(t, db, enums.RoleSalesTeam))
	b := signInAs(t, srv, u)
	before := b.cookies()
	old := before["sc_refresh"].Value
	if a := b.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusOK {
		t.Fatalf("renewing answered %d", a.status)
	}
	// The answer is lost: the browser still holds the old token.
	b.jar = copyJar(before)
	db.Exec("UPDATE session_spent_tokens SET spent_at = spent_at - interval '20 minutes' WHERE token_hash = ?", hash.SHA256(old))
	if a := b.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusOK {
		t.Fatalf("the same browser with the token whose answer was lost answered %d %s", a.status, a.body)
	}
	if c := b.cookies()["sc_refresh"]; c == nil || c.Value == old {
		t.Fatal("no fresh refresh token was handed out")
	}
	if a := b.do(fiber.MethodGet, "/api/v1/auth/me", nil); a.status != fiber.StatusOK {
		t.Fatalf("after recovering, me answered %d", a.status)
	}
	var revoked int64
	db.Raw("SELECT count(*) FROM sessions WHERE user_id = ? AND revoked_at IS NOT NULL", u.ID).Scan(&revoked)
	if revoked != 0 {
		t.Fatalf("%d sessions ended for a lost answer, want 0", revoked)
	}
	// The same old token from another network is a copy.
	other := newBrowser(t, srv.app, nil, u)
	other.jar = copyJar(before)
	other.ip = "198.51.100.67"
	time.Sleep(1100 * time.Millisecond)
	if a := other.do(fiber.MethodPost, "/api/v1/auth/refresh", nil); a.status != fiber.StatusUnauthorized {
		t.Fatalf("the old token from another network answered %d, want 401", a.status)
	}
	db.Exec("DELETE FROM audit_log WHERE actor_id = ?", u.ID)
}
