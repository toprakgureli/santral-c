package main

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/toprakgureli/santral-c/backend/configs"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
	"github.com/toprakgureli/santral-c/backend/internal/setup"
	"github.com/toprakgureli/santral-c/backend/internal/testdb"
	"github.com/toprakgureli/santral-c/backend/pkg/crypt"
	"github.com/toprakgureli/santral-c/backend/pkg/enums"
	"github.com/toprakgureli/santral-c/backend/pkg/hash"
	"github.com/toprakgureli/santral-c/backend/pkg/postgresql"
	"github.com/toprakgureli/santral-c/backend/pkg/redis"
)

// envRedis names the Redis the route tests use, as host:port.
const envRedis = "SANTRAL_TEST_REDIS"

// public lists every route that answers without a signed-in user, with the
// reason it may. A new route is guarded unless it is added here on purpose.
var public = map[string]string{
	"GET /healthz":                        "uptime check",
	"GET /metrics":                        "served to the server itself only",
	"GET /api/v1/version":                 "build stamp",
	"POST /api/v1/auth/login":             "signing in",
	"POST /api/v1/auth/refresh":           "renewing a session from its cookie",
	"POST /api/v1/auth/logout":            "ending a session from its cookie",
	"POST /api/v1/auth/mfa/verify":        "second step of signing in",
	"POST /api/v1/auth/mfa/enroll":        "second step of signing in",
	"POST /api/v1/auth/mfa/enroll/verify": "second step of signing in",
	"POST /api/v1/auth/password/change":   "forced password change while signing in",
	"GET /api/v1/wa/hook/:key":            "Meta checks the webhook",
	"POST /api/v1/wa/hook/:key":           "Meta delivers messages",
	"POST /api/v1/wa/survey/:key":         "customer answers a survey",
}

func TestMain(m *testing.M) {
	// Refused requests are logged as warnings; the tests expect many.
	slog.SetDefault(slog.New(slog.DiscardHandler))
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// testServer builds the whole application on the test database and Redis.
// opts adjust the configuration before the application is built.
func testServer(t *testing.T, opts ...func(*configs.Config)) (*server, *gorm.DB) {
	t.Helper()
	db := testdb.Open(t)
	// The same connection limit as a live server, so a burst queues for
	// connections the way it does there.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	postgresql.Pool(sqlDB, 40)
	addr := os.Getenv(envRedis)
	if addr == "" {
		t.Skipf("%s is not set; skipping a test that needs Redis", envRedis)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("%s: %v", envRedis, err)
	}
	if err := redis.Connect(configs.Redis{Host: host, Port: port, DB: 15}); err != nil {
		t.Fatal(err)
	}

	cfg := configs.Config{
		App: configs.App{Name: "santral-test"},
		Auth: configs.Auth{
			Secret:            "route-test-signing-secret-0123456789abcdef",
			AccessTTL:         15 * time.Minute,
			RefreshTTL:        time.Hour,
			Issuer:            "santral-test",
			CookieName:        "sc_access",
			RefreshCookieName: "sc_refresh",
		},
		Security: configs.Security{
			IPFailureLimit:      50,
			IPBanDuration:       time.Minute,
			AccountLockDuration: time.Minute,
			DistinctIPLimit:     10,
			AttemptWindow:       time.Minute,
			MFAKey:              "route-test-mfa-key-0123456789abcdef",
		},
		// The phone system's routes are mounted too; nothing calls it
		// because the background work is never started.
		Bulutsantralim: configs.Bulutsantralim{Enabled: true, APIKey: "route-test", APIBase: "http://127.0.0.1:1"},
		Owner:          configs.Owner{Name: "Test Owner", Email: "owner@route-test.local", Password: "Owner-Route-Test-1"},
	}
	for _, o := range opts {
		o(&cfg)
	}
	configs.Cnf = cfg
	if err := setup.Seed(db); err != nil {
		t.Fatal(err)
	}
	ring, err := crypt.NewKeyring(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := newServer(cfg, db, ring)
	if err != nil {
		t.Fatal(err)
	}
	return srv, db
}

var paramRe = regexp.MustCompile(`:[A-Za-z]+\??|\*`)

// concrete turns a route pattern into a path that matches it.
func concrete(pattern string) string {
	return paramRe.ReplaceAllString(pattern, "1")
}

func call(t *testing.T, app *fiber.App, method, path, body string, cookies []*http.Cookie) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return res
}

// TestEveryRouteNeedsSignIn walks the whole route table: anything not
// listed as public must turn away a request without a session.
func TestEveryRouteNeedsSignIn(t *testing.T) {
	srv, _ := testServer(t)

	seen := map[string]bool{}
	for _, r := range srv.app.GetRoutes(true) {
		if r.Method == fiber.MethodHead || r.Method == fiber.MethodOptions {
			continue
		}
		key := r.Method + " " + r.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		res := call(t, srv.app, r.Method, concrete(r.Path), "", nil)
		_ = res.Body.Close()
		guarded := res.StatusCode == fiber.StatusUnauthorized && res.Header.Get(fiber.HeaderWWWAuthenticate) != ""
		if _, ok := public[key]; ok {
			if guarded {
				t.Errorf("%s is listed as public but asks for a session; remove it from the list", key)
			}
			continue
		}
		if !guarded {
			t.Errorf("%s answered %d without a session; guard it or list it as public", key, res.StatusCode)
		}
	}
	for key := range public {
		if !seen[key] {
			t.Errorf("public route %s no longer exists; remove it from the list", key)
		}
	}
}

// signIn creates an active user with the given roles and returns its
// session cookies.
func signIn(t *testing.T, srv *server, db *gorm.DB, roles ...enums.Role) []*http.Cookie {
	t.Helper()
	const password = "Route-Test-Pass-1"
	pw, err := hash.Password(password)
	if err != nil {
		t.Fatal(err)
	}
	var rs []models.Role
	for _, name := range roles {
		var r models.Role
		if err := db.Where("name = ?", string(name)).First(&r).Error; err != nil {
			t.Fatalf("role %s: %v", name, err)
		}
		rs = append(rs, r)
	}
	email := "route-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405.000000") + "@route-test.local"
	u := models.User{Name: "Route Test", Email: strings.ToLower(email), Password: pw, Active: true, MFAExempt: true, Roles: rs}
	if err := db.Omit("Roles.*").Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { removeUser(context.Background(), db, u.ID) })

	res := call(t, srv.app, fiber.MethodPost, "/api/v1/auth/login",
		`{"email":"`+u.Email+`","password":"`+password+`"}`, nil)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != fiber.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("sign in: %d %s", res.StatusCode, b)
	}
	cookies := res.Cookies()
	if len(cookies) == 0 {
		t.Fatal("sign in set no cookies")
	}
	return cookies
}

// removeUser deletes a test user and what signing in left behind.
func removeUser(ctx context.Context, db *gorm.DB, id uint) {
	db = db.WithContext(ctx)
	for _, q := range []string{
		"DELETE FROM sessions WHERE user_id = ?",
		"DELETE FROM audit_logs WHERE actor_id = ?",
		"DELETE FROM user_roles WHERE user_id = ?",
		"DELETE FROM users WHERE id = ?",
	} {
		db.Exec(q, id)
	}
}

// adminOnly are requests that change who can do what, read the audit
// trail or reach the integrations. Only a role with the right permission
// may make them.
var adminOnly = []struct{ method, path, body string }{
	{fiber.MethodGet, "/api/v1/users", ""},
	{fiber.MethodPost, "/api/v1/users", `{}`},
	{fiber.MethodGet, "/api/v1/roles", ""},
	{fiber.MethodPost, "/api/v1/roles", `{}`},
	{fiber.MethodGet, "/api/v1/audit", ""},
	{fiber.MethodGet, "/api/v1/settings", ""},
	{fiber.MethodGet, "/api/v1/security/bans", ""},
	{fiber.MethodGet, "/api/v1/wa/channels", ""},
	{fiber.MethodPost, "/api/v1/wa/channels", `{}`},
	{fiber.MethodGet, "/api/v1/wa/bots", ""},
	{fiber.MethodGet, "/api/v1/wa/integrations", ""},
	{fiber.MethodGet, "/api/v1/escalations/", ""},
	{fiber.MethodPost, "/api/v1/escalations/categories", `{}`},
	{fiber.MethodPut, "/api/v1/games/settings", `{}`},
	{fiber.MethodPost, "/api/v1/pbx/sip/sync-all", ""},
	{fiber.MethodGet, "/api/v1/calls/route-test/recording", ""},
}

// TestPermissionsAreEnforced signs in without any role and expects every
// admin request to be refused, then signs in as the top role and expects
// the same requests to get past the permission check.
func TestPermissionsAreEnforced(t *testing.T) {
	srv, db := testServer(t)

	nobody := signIn(t, srv, db)
	for _, r := range adminOnly {
		res := call(t, srv.app, r.method, r.path, r.body, nobody)
		_ = res.Body.Close()
		if res.StatusCode != fiber.StatusForbidden {
			t.Errorf("no role: %s %s answered %d, want 403", r.method, r.path, res.StatusCode)
		}
	}

	admin := signIn(t, srv, db, enums.RoleInvisibleAdmin)
	for _, r := range adminOnly {
		res := call(t, srv.app, r.method, r.path, r.body, admin)
		_ = res.Body.Close()
		if res.StatusCode == fiber.StatusForbidden || res.StatusCode == fiber.StatusUnauthorized {
			t.Errorf("admin: %s %s answered %d", r.method, r.path, res.StatusCode)
		}
	}
}
