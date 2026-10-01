package middlewares

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func sameOriginApp() *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Use(SameOrigin(SameOriginConfig{
		Origins:       []string{"http://localhost:5173", "https://cm.example.com/", ""},
		Cookies:       []string{"sc_access", "sc_refresh"},
		GuardedPrefix: "/api/v1/auth/",
		Multipart:     []string{"/api/v1/wa/files", "/api/v1/wa/conversations/*/media"},
	}))
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }
	app.All("/*", ok)
	return app
}

func TestSameOrigin(t *testing.T) {
	app := sameOriginApp()
	cases := []struct {
		name    string
		method  string
		path    string
		host    string
		cookie  bool
		headers map[string]string
		body    string
		want    int
	}{
		{"read with a foreign origin", "GET", "/api/v1/users", "", true, map[string]string{"Origin": "https://evil.example.com"}, "", 204},
		{"json from the panel's address", "POST", "/api/v1/users", "", true, map[string]string{"Origin": "https://cm.example.com", "Content-Type": "application/json"}, "{}", 204},
		{"json with a default port", "POST", "/api/v1/users", "", true, map[string]string{"Origin": "https://CM.example.com:443", "Content-Type": "application/json; charset=utf-8"}, "{}", 204},
		{"json from the dev server", "PUT", "/api/v1/users/1", "", true, map[string]string{"Origin": "http://localhost:5173", "Content-Type": "application/json"}, "{}", 204},
		{"json from the address it was sent to", "POST", "/api/v1/users", "panel.internal:8443", true, map[string]string{"Origin": "https://panel.internal:8443", "Content-Type": "application/json"}, "{}", 204},
		{"json from a neighbouring site", "POST", "/api/v1/users", "", true, map[string]string{"Origin": "https://evil.example.com", "Content-Type": "application/json"}, "{}", 403},
		{"origin null", "POST", "/api/v1/users", "", true, map[string]string{"Origin": "null", "Content-Type": "application/json"}, "{}", 403},
		{"same-site but not same-origin", "DELETE", "/api/v1/users/1", "", true, map[string]string{"Sec-Fetch-Site": "same-site"}, "", 403},
		{"same-origin without an Origin", "DELETE", "/api/v1/users/1", "", true, map[string]string{"Sec-Fetch-Site": "same-origin"}, "", 204},
		{"a client that is not a browser", "POST", "/api/v1/users", "", true, map[string]string{"Content-Type": "application/json"}, "{}", 204},
		{"a form post from the panel's own address", "POST", "/api/v1/users", "", true, map[string]string{"Origin": "https://cm.example.com", "Content-Type": "application/x-www-form-urlencoded"}, "a=1", 415},
		{"an empty form post", "POST", "/api/v1/auth/logout", "", true, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "", 415},
		{"a text body", "POST", "/api/v1/calls/log/", "", true, map[string]string{"Origin": "https://cm.example.com", "Content-Type": "text/plain"}, "{}", 415},
		{"a body without a type", "POST", "/api/v1/users", "", true, nil, "{}", 415},
		{"no body at all", "POST", "/api/v1/auth/refresh", "", true, map[string]string{"Origin": "https://cm.example.com"}, "", 204},
		{"multipart on an upload route", "POST", "/api/v1/wa/conversations/7/media", "", true, map[string]string{"Origin": "https://cm.example.com", "Content-Type": "multipart/form-data; boundary=x"}, "--x--", 204},
		{"multipart with a trailing slash", "POST", "/api/v1/wa/files/", "", true, map[string]string{"Origin": "https://cm.example.com", "Content-Type": "multipart/form-data; boundary=x"}, "--x--", 204},
		{"multipart anywhere else", "POST", "/api/v1/users", "", true, map[string]string{"Origin": "https://cm.example.com", "Content-Type": "multipart/form-data; boundary=x"}, "--x--", 415},
		{"multipart upload from a neighbouring site", "POST", "/api/v1/wa/files", "", true, map[string]string{"Origin": "https://evil.example.com", "Content-Type": "multipart/form-data; boundary=x"}, "--x--", 403},
		{"a webhook without cookies", "POST", "/api/v1/wa/hook/k", "", false, map[string]string{"Origin": "https://graph.facebook.com", "Content-Type": "text/plain"}, "x", 204},
		{"sign in from a neighbouring site", "POST", "/api/v1/auth/login", "", false, map[string]string{"Origin": "https://evil.example.com", "Content-Type": "application/json"}, "{}", 403},
		{"sign in from the panel", "POST", "/api/v1/auth/login", "", false, map[string]string{"Origin": "https://cm.example.com", "Content-Type": "application/json"}, "{}", 204},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			var req *http.Request
			if body != nil {
				req = httptest.NewRequest(tc.method, tc.path, body)
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			if tc.host != "" {
				req.Host = tc.host
			}
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: "sc_access", Value: "token"})
			}
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("answered %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestNormalOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"https://CM.example.com":     "https://cm.example.com",
		"https://cm.example.com:443": "https://cm.example.com",
		"http://localhost:80":        "http://localhost",
		"http://localhost:5173/":     "http://localhost:5173",
		"null":                       "",
		"chrome-extension://abc":     "",
		"":                           "",
	} {
		if got := normalOrigin(in); got != want {
			t.Errorf("normalOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}
