package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/toprakgureli/santral-c/backend/pkg/enums"
)

// streamFor writes a chunk every 50 ms for d, like a live stream or a
// long download does.
func streamFor(d time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, "text/event-stream")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			for end := time.Now().Add(d); time.Now().Before(end); {
				if _, err := w.WriteString("data: x\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			_, _ = w.WriteString("data: end\n\n")
			_ = w.Flush()
		})
		return nil
	}
}

// TestTimeoutsDoNotCutStreams runs a real server with short timeouts: an
// ordinary answer that takes longer than the write timeout is cut, a live
// stream and a download taking as long are not.
func TestTimeoutsDoNotCutStreams(t *testing.T) {
	short := timeouts{Read: time.Second, Upload: time.Second, Write: 300 * time.Millisecond, Download: 3 * time.Second, Stream: 3 * time.Second, Idle: time.Second}
	cfg := fiber.Config{DisableStartupMessage: true}
	short.apply(&cfg)
	app := fiber.New(cfg)
	short.install(app)
	app.Get("/api/v1/teams/stream", streamFor(time.Second))
	app.Get("/api/v1/wa/media/1", streamFor(time.Second))
	app.Post("/api/v1/slow", streamFor(time.Second))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	base := "http://" + ln.Addr().String()

	read := func(method, path string) string {
		t.Helper()
		req, _ := http.NewRequest(method, base+path, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return "error: " + err.Error()
		}
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(res.Body)
		return string(body)
	}
	if got := read(http.MethodGet, "/api/v1/teams/stream"); !strings.HasSuffix(got, "data: end\n\n") {
		t.Errorf("the live stream was cut: %q", got)
	}
	if got := read(http.MethodGet, "/api/v1/wa/media/1"); !strings.HasSuffix(got, "data: end\n\n") {
		t.Errorf("the download was cut: %q", got)
	}
	if got := read(http.MethodPost, "/api/v1/slow"); strings.HasSuffix(got, "data: end\n\n") {
		t.Error("an ordinary answer outlived the write timeout")
	}
}

func TestRequestLimits(t *testing.T) {
	var h fasthttp.RequestHeader
	h.SetMethod(fiber.MethodPost)
	h.SetRequestURI("/api/v1/wa/files")
	h.SetContentLength(50 << 20)
	if rc := serverTimeouts.forRequest(&h); rc.ReadTimeout != serverTimeouts.Upload || rc.WriteTimeout != 0 {
		t.Errorf("a 50 MB upload got %+v", rc)
	}
	h.SetContentLength(2 << 10)
	if rc := serverTimeouts.forRequest(&h); rc.ReadTimeout != 0 {
		t.Errorf("a small body got %+v", rc)
	}
	h.SetMethod(fiber.MethodGet)
	h.SetContentLength(0)
	h.SetRequestURI("/api/v1/pbx/stream?x=1")
	if rc := serverTimeouts.forRequest(&h); rc.WriteTimeout != serverTimeouts.Stream {
		t.Errorf("a live stream got %+v", rc)
	}
	h.SetRequestURI("/api/v1/calls/export")
	if rc := serverTimeouts.forRequest(&h); rc.WriteTimeout != serverTimeouts.Download {
		t.Errorf("a download got %+v", rc)
	}
}

// TestPathsAreCaseSensitive: the API answers only as written; the web
// servers in front decide by the path as written too.
func TestPathsAreCaseSensitive(t *testing.T) {
	srv, _ := testServer(t)
	// The version route exists for signed-in people only, so written right it
	// asks for a session (401); any other spelling is not a route at all.
	for path, want := range map[string]int{
		"/api/v1/version": fiber.StatusUnauthorized,
		"/API/v1/version": fiber.StatusNotFound,
		"/Api/V1/Version": fiber.StatusNotFound,
	} {
		res := call(t, srv.app, fiber.MethodGet, path, "", nil)
		_ = res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, res.StatusCode, want)
		}
	}
}

// TestHookPortReachesOnlyHooks: a request that came in on the port for a
// webhook already registered in Meta reaches neither the API (in any
// spelling) nor the health check or the metrics.
func TestHookPortReachesOnlyHooks(t *testing.T) {
	srv, _ := testServer(t)
	for _, path := range []string{"/api/v1/version", "/API/v1/version", "/api", "/healthz", "/HEALTHZ", "/metrics", "/api/v1/auth/login"} {
		req := httptest.NewRequest(fiber.MethodGet, path, nil)
		req.Header.Set(hookEntryHeader, "existing-hook")
		res, err := srv.app.Test(req, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != fiber.StatusNotFound {
			t.Errorf("GET %s on the hook port = %d, want 404", path, res.StatusCode)
		}
	}
	// The same request without the header is the panel's own: the route is
	// there (it asks for a session) instead of being hidden.
	res := call(t, srv.app, fiber.MethodGet, "/api/v1/version", "", nil)
	_ = res.Body.Close()
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("version on the panel = %d, want 401", res.StatusCode)
	}
}

// TestSystemHealthNeedsItsPermission: the system warnings answer the
// roles that hold system.health and refuse the others.
func TestSystemHealthNeedsItsPermission(t *testing.T) {
	srv, db := testServer(t)
	held := func(r enums.Role) bool {
		for _, p := range enums.RolePermissions(r) {
			if p == enums.SystemHealth {
				return true
			}
		}
		return false
	}
	if !held(enums.RoleInvisibleAdmin) || !held(enums.RoleManager) || held(enums.RoleTechnicalTeam) || held(enums.RoleSalesTeam) {
		t.Fatal("system.health must be a default of the admin and manager roles only")
	}
	for _, role := range []enums.Role{enums.RoleInvisibleAdmin, enums.RoleManager, enums.RoleTechnicalTeam, enums.RoleSalesTeam, ""} {
		var cookies []*http.Cookie
		if role == "" {
			cookies = signIn(t, srv, db)
		} else {
			cookies = signIn(t, srv, db, role)
		}
		res := call(t, srv.app, fiber.MethodGet, "/api/v1/system/health", "", cookies)
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if !held(role) {
			if res.StatusCode != fiber.StatusForbidden {
				t.Errorf("%q: %d, want 403", role, res.StatusCode)
			}
			continue
		}
		if res.StatusCode != fiber.StatusOK {
			t.Errorf("%q: %d %s", role, res.StatusCode, body)
			continue
		}
		var report struct {
			Warnings  []map[string]any `json:"warnings"`
			CheckedAt time.Time        `json:"checkedAt"`
		}
		if err := json.Unmarshal(body, &report); err != nil || report.Warnings == nil || report.CheckedAt.IsZero() {
			t.Errorf("%q: report %s (%v)", role, body, err)
		}
		for _, w := range report.Warnings {
			if w["key"] == "database" || w["key"] == "redis" {
				t.Errorf("%q: the test database or Redis was reported down: %v", role, w)
			}
		}
	}
}
