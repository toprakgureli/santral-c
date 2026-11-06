package custommenu_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/toprakgureli/santral-c/backend/internal/custommenu"
	"github.com/toprakgureli/santral-c/backend/internal/domain/models"
)

type fakeActors struct {
	user *models.User
}

func (f fakeActors) GetByID(_ context.Context, _ uint) (*models.User, error) {
	return f.user, nil
}

func TestInspectFramingHeaders(t *testing.T) {
	cases := []struct {
		name      string
		xfo       string
		csp       string
		embed     bool
		wantInWhy string
	}{
		{
			name:  "no framing headers",
			embed: true,
		},
		{
			name:      "X-Frame-Options DENY",
			xfo:       "DENY",
			embed:     false,
			wantInWhy: "X-Frame-Options",
		},
		{
			name:      "X-Frame-Options SAMEORIGIN",
			xfo:       "sameorigin",
			embed:     false,
			wantInWhy: "X-Frame-Options",
		},
		{
			name:      "CSP frame-ancestors none",
			csp:       "default-src 'self'; frame-ancestors 'none'; script-src 'self'",
			embed:     false,
			wantInWhy: "frame-ancestors",
		},
		{
			name:      "CSP frame-ancestors self only",
			csp:       "frame-ancestors 'self' https://login.example.com",
			embed:     false,
			wantInWhy: "frame-ancestors",
		},
		{
			name:  "CSP frame-ancestors wildcard",
			csp:   "frame-ancestors *",
			embed: true,
		},
		{
			name:  "CSP frame-ancestors https scheme",
			csp:   "frame-ancestors https: http:",
			embed: true,
		},
		{
			name:  "CSP without frame-ancestors overrides nothing",
			csp:   "default-src 'self'",
			embed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			if tc.xfo != "" {
				h.Set("X-Frame-Options", tc.xfo)
			}
			if tc.csp != "" {
				h.Set("Content-Security-Policy", tc.csp)
			}
			res := custommenu.InspectFramingHeaders(h)
			if res.Embeddable != tc.embed {
				t.Fatalf("InspectFramingHeaders(%v) Embeddable = %v, want %v (reason: %q)", h, res.Embeddable, tc.embed, res.Reason)
			}
			if !tc.embed && res.Reason == "" {
				t.Fatalf("expected non-empty Reason when Embeddable is false")
			}
		})
	}
}

func TestCreateRejectsWithoutPermission(t *testing.T) {
	svc := custommenu.NewService(nil, fakeActors{user: &models.User{ID: 7}}, nil)
	_, err := svc.Create(context.Background(), 7, custommenu.Input{
		Label: "Telegram",
		Icon:  "telegram",
		URL:   "https://web.telegram.org/k/",
	}, "127.0.0.1")
	if err == nil {
		t.Fatalf("expected forbidden error when actor lacks menu.manage")
	}
}
