// Package ops serves what the people running the server look at: a health
// check for the uptime monitor and deploys, and a few numbers that show a
// queue backing up before users notice.
package ops

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/toprakgureli/santral-c/backend/internal/sse"
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
	db    *sql.DB
	redis *goredis.Client
	build Build
}

// NewHandler builds the handler.
func NewHandler(db *sql.DB, redis *goredis.Client, build Build) *Handler {
	return &Handler{db: db, redis: redis, build: build}
}

// Routes mounts /healthz and /metrics on the app root.
func (h *Handler) Routes(app fiber.Router) {
	app.Get("/healthz", h.Health)
	app.Get("/metrics", h.Metrics)
}

// Health answers 200 when the server can reach its database and Redis, and
// 503 naming what is down otherwise.
func (h *Handler) Health(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), checkTimeout)
	defer cancel()
	checks := fiber.Map{"database": "ok", "redis": "ok"}
	status := fiber.StatusOK
	if err := h.db.PingContext(ctx); err != nil {
		checks["database"] = "down"
		status = fiber.StatusServiceUnavailable
		slog.ErrorContext(ctx, "health check: database is down", "error", err)
	}
	if err := h.redis.Ping(ctx).Err(); err != nil {
		checks["redis"] = "down"
		status = fiber.StatusServiceUnavailable
		slog.ErrorContext(ctx, "health check: redis is down", "error", err)
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

// metric is one number on the metrics page.
type metric struct {
	name, help string
	query      string
}

var queueMetrics = []metric{
	{"santral_whatsapp_outbox_queued", "Outgoing WhatsApp messages waiting to be sent.", "SELECT count(*) FROM wa_messages WHERE status = 'queued'"},
	{"santral_whatsapp_outbox_sending", "Outgoing WhatsApp messages being sent right now.", "SELECT count(*) FROM wa_messages WHERE status = 'sending'"},
	{"santral_whatsapp_webhook_events_pending", "Meta notices waiting to be processed.", "SELECT count(*) FROM wa_webhook_events WHERE status = 'pending'"},
	{"santral_whatsapp_webhook_events_failed", "Meta notices that could not be processed.", "SELECT count(*) FROM wa_webhook_events WHERE status = 'failed'"},
	{"santral_whatsapp_inbound_jobs", "Customer messages whose follow-up work is not done yet.", "SELECT count(*) FROM wa_inbound_jobs"},
	{"santral_call_surveys_queued", "Call surveys waiting to be sent.", "SELECT count(*) FROM wa_call_surveys WHERE status = 'queued'"},
}

// Metrics writes the numbers in the Prometheus text format. It answers only
// on the server itself (the reverse proxy never forwards it), since the
// numbers are nobody else's business.
func (h *Handler) Metrics(c *fiber.Ctx) error {
	if !local(c) {
		return fiber.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), checkTimeout)
	defer cancel()
	var b strings.Builder
	write := func(name, help string, value int64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
	}
	for _, m := range queueMetrics {
		var n int64
		if err := h.db.QueryRowContext(ctx, m.query).Scan(&n); err != nil {
			slog.WarnContext(ctx, "metric could not be read", "metric", m.name, "error", err)
			continue
		}
		write(m.name, m.help, n)
	}
	write("santral_sse_streams", "Live event streams open to the panel.", sse.Open())
	c.Set(fiber.HeaderContentType, "text/plain; version=0.0.4; charset=utf-8")
	return c.SendString(b.String())
}
