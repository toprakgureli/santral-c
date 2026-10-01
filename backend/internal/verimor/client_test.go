package verimor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testKey = "K12345678-1234-5678-4321-123456789012"

// dropServer accepts every request and closes the connection without an
// answer, the way a proxy does when the phone system restarts.
func dropServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("connection cannot be taken over")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// noSecret fails when an error's text carries the key or a token.
func noSecret(t *testing.T, name string, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: no error", name)
		return
	}
	for _, s := range append(secrets, testKey, url.QueryEscape(testKey)) {
		if strings.Contains(err.Error(), s) {
			t.Errorf("%s: error carries a secret: %s", name, err)
		}
	}
}

func TestClientErrorsNeverCarryTheKey(t *testing.T) {
	srv := dropServer(t)
	c := NewClient(testKey, srv.URL)
	ctx := context.Background()

	_, _, err := c.CDRs(ctx, url.Values{})
	noSecret(t, "cdrs", err)
	_, _, err = c.CDRsSlow(ctx, url.Values{})
	noSecret(t, "cdrs slow", err)
	_, err = c.UserStatuses(ctx)
	noSecret(t, "user statuses", err)
	_, err = c.Queues(ctx)
	noSecret(t, "queues", err)
	noSecret(t, "dnd", c.SetDND(ctx, "1010", true))
	_, err = c.Originate(ctx, "1010", "05551234567")
	noSecret(t, "originate", err)
	_, err = c.WebphoneToken(ctx, "1010")
	noSecret(t, "webphone token", err)
	_, err = c.RecordingURL(ctx, "uuid-1")
	noSecret(t, "recording url", err)
	_, err = c.CDRCount(ctx, url.Values{})
	noSecret(t, "cdr count", err)

	// A signed recording link and the webphone page carry their own secrets.
	_, err = c.OpenRecording(ctx, srv.URL+"/rec/1.mp3?signature=s3cr3t-sig&expires=1")
	noSecret(t, "recording download", err, "s3cr3t-sig")
	if err != nil && !strings.Contains(err.Error(), "/rec/1.mp3") {
		t.Errorf("the redacted error lost the path: %s", err)
	}
}

func TestWebphonePageErrorHidesTheToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/webphone_tokens", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"tok-very-secret"}`))
	})
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)
	page := dropServer(t)

	_, err := NewClient(testKey, api.URL).WebphoneSIP(context.Background(), page.URL+"/webphone", "1010")
	noSecret(t, "webphone page", err, "tok-very-secret")
}

func TestRefusalBodyIsMasked(t *testing.T) {
	// The phone system echoes a wrong key back in its refusal.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Gecersiz anahtar: "+r.URL.Query().Get("key"), http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	err := NewClient(testKey, srv.URL).SetDND(context.Background(), "1010", false)
	noSecret(t, "refused dnd", err)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("want an APIError with 400, got %v", err)
	}
	if !strings.Contains(err.Error(), secretMask) {
		t.Errorf("the masked key is not marked: %s", err)
	}
}

func TestRedactedTimeoutStaysATimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := NewClient(testKey, srv.URL).Queues(ctx)
	noSecret(t, "timeout", err)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a timeout no longer reads as one: %v", err)
	}
}
