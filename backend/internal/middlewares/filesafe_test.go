package middlewares

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestFileHeaders(t *testing.T) {
	tests := []struct {
		name, mime, file string
		download         bool
		wantType         string
		wantInline       bool
		wantSandbox      bool
	}{
		{"web page is only saved", "text/html", "x.html", false, "application/octet-stream", false, true},
		{"svg is only saved", "image/svg+xml", "x.svg", false, "application/octet-stream", false, true},
		{"claimed type with parameters", "TEXT/HTML; charset=utf-8", "x.html", false, "application/octet-stream", false, true},
		{"picture is shown", "image/png", "a.png", false, "image/png", true, true},
		{"picture asked as download", "image/png", "a.png", true, "image/png", false, true},
		{"voice note is played", "audio/ogg; codecs=opus", "v.ogg", false, "audio/ogg", true, true},
		{"pdf is shown without sandbox", "application/pdf", "f.pdf", false, "application/pdf", true, false},
		{"unknown type is saved", "", "blob", false, "application/octet-stream", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/", func(c *fiber.Ctx) error {
				FileHeaders(c, tt.mime, tt.file, tt.download)
				return c.SendString("x")
			})
			res, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Header.Get("Content-Type"); got != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
			}
			disp := res.Header.Get("Content-Disposition")
			if inline := strings.HasPrefix(disp, "inline"); inline != tt.wantInline {
				t.Errorf("Content-Disposition = %q, inline want %v", disp, tt.wantInline)
			}
			if res.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Error("nosniff missing")
			}
			if sandbox := strings.Contains(res.Header.Get("Content-Security-Policy"), "sandbox"); sandbox != tt.wantSandbox {
				t.Errorf("sandbox = %v, want %v", sandbox, tt.wantSandbox)
			}
		})
	}
}

func TestFileNameEscaped(t *testing.T) {
	if got := nameEscape(`a"b;c'd.html`); strings.ContainsAny(got, `";'`) {
		t.Errorf("nameEscape left a separator: %q", got)
	}
}
