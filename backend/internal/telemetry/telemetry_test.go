package telemetry

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRequestMetricsUseRoutePatterns(t *testing.T) {
	m := newHTTPMetrics()

	app := fiber.New()
	app.Use(m.Middleware(func(error) int { return 418 }))
	app.Get("/api/v1/users/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/metrics", func(c *fiber.Ctx) error { return c.SendString("numbers") })
	app.Get("/boom", func(*fiber.Ctx) error { return fiber.ErrTeapot })
	for _, path := range []string{"/api/v1/users/1", "/api/v1/users/2", "/metrics", "/boom"} {
		res, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/api/v1/users/:id", "200")); got != 2 {
		t.Errorf("users route counted %v times, want 2 (ids must not become labels)", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/boom", "418")); got != 1 {
		t.Errorf("error status not taken from the error: %v", got)
	}
	if n := testutil.CollectAndCount(m.requests); n != 2 {
		t.Errorf("%d series, want 2 (/metrics is left out)", n)
	}
	if !strings.Contains(m.requests.WithLabelValues("GET", "/api/v1/users/:id", "200").Desc().String(), "santral_http_requests_total") {
		t.Error("unexpected metric name")
	}
}

func TestTracingOffWithoutEndpoint(t *testing.T) {
	stop, err := SetupTracing(context.Background(), TraceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
