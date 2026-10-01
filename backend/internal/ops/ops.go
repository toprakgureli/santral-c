// Package ops serves what the people running the server look at: a health
// check for the uptime monitor and deploys, and a few numbers that show a
// queue backing up before users notice.
package ops

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
)

// checkTimeout bounds each dependency check, so a hung database makes the
// health check fail instead of hang.
const checkTimeout = 2 * time.Second

// Build names the running build.
type Build struct {
	Version string
	Time    string
}

// Handler serves the health check and the metrics.
type Handler struct {
	db      *sql.DB
	redis   *goredis.Client
	build   Build
	metrics fiber.Handler
}

// NewHandler builds the handler; registry holds the server's numbers. It
// adds santral_dependency_up to it, from the same checks /healthz runs.
func NewHandler(db *sql.DB, redis *goredis.Client, build Build, registry *prometheus.Registry) *Handler {
	h := &Handler{db: db, redis: redis, build: build,
		metrics: adaptor.HTTPHandler(promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))}
	registry.MustRegister(&dependencyCollector{h: h})
	return h
}

// deps pings the database and Redis, each with its own time limit, so a
// hung database does not make Redis look down too.
func (h *Handler) deps(ctx context.Context) (dbErr, redisErr error) {
	dctx, cancel := context.WithTimeout(ctx, checkTimeout)
	dbErr = h.db.PingContext(dctx)
	cancel()
	if h.redis == nil {
		return dbErr, errors.New("redis is not connected")
	}
	rctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	return dbErr, h.redis.Ping(rctx).Err()
}

// dependencyDesc is 1 while the server reaches a dependency and 0 when it
// does not. A gauge read from the database is missing while the database
// is down; this one is not, so the alert on it fires.
var dependencyDesc = prometheus.NewDesc("santral_dependency_up",
	"1 when the server reaches the dependency (postgres, redis), 0 when it does not.", []string{"dep"}, nil)

type dependencyCollector struct {
	h *Handler
}

// Describe sends the gauge's description.
func (d *dependencyCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- dependencyDesc
}

// Collect checks both dependencies at scrape time.
func (d *dependencyCollector) Collect(ch chan<- prometheus.Metric) {
	dbErr, redisErr := d.h.deps(context.Background())
	ch <- prometheus.MustNewConstMetric(dependencyDesc, prometheus.GaugeValue, up(dbErr), "postgres")
	ch <- prometheus.MustNewConstMetric(dependencyDesc, prometheus.GaugeValue, up(redisErr), "redis")
}

func up(err error) float64 {
	if err != nil {
		return 0
	}
	return 1
}

// Routes mounts /healthz and /metrics on the app root.
func (h *Handler) Routes(app fiber.Router) {
	app.Get("/healthz", h.Health)
	app.Get("/metrics", h.Metrics)
}

// Health answers 200 when the server can reach its database and Redis, and
// 503 naming what is down otherwise.
func (h *Handler) Health(c *fiber.Ctx) error {
	ctx := c.UserContext()
	checks := fiber.Map{"database": "ok", "redis": "ok"}
	status := fiber.StatusOK
	dbErr, redisErr := h.deps(ctx)
	if dbErr != nil {
		checks["database"] = "down"
		status = fiber.StatusServiceUnavailable
		slog.ErrorContext(ctx, "health check: database is down", "error", dbErr)
	}
	if redisErr != nil {
		checks["redis"] = "down"
		status = fiber.StatusServiceUnavailable
		slog.ErrorContext(ctx, "health check: redis is down", "error", redisErr)
	}
	word := "ok"
	if status != fiber.StatusOK {
		word = "down"
	}
	// The build and the parts are told only on the server itself; from
	// outside the answer is just up or down, so nobody learns which commit
	// of the public code runs here.
	if !local(c) {
		return c.Status(status).JSON(fiber.Map{"status": word})
	}
	return c.Status(status).JSON(fiber.Map{"status": word, "checks": checks, "version": h.build.Version, "buildTime": h.build.Time})
}

// local reports whether a request comes from the server itself rather than
// through the reverse proxy.
func local(c *fiber.Ctx) bool {
	return c.Context().RemoteIP().IsLoopback() && c.Get(fiber.HeaderXForwardedFor) == ""
}

// Metrics serves the numbers in the Prometheus format. It answers only on
// the server itself (the reverse proxy never forwards it), since the
// numbers are nobody else's business; Prometheus scrapes it from there.
func (h *Handler) Metrics(c *fiber.Ctx) error {
	if !local(c) {
		return fiber.ErrNotFound
	}
	return h.metrics(c)
}
