// Package telemetry is what the people running the server watch: numbers
// for Prometheus (requests, errors, latency, queues, live streams, the
// database pool, the last backup) and traces of requests and their database
// queries for Jaeger. Grafana shows both; the dashboard and the alerts live
// in deploy/observability.
package telemetry

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics holds the server's Prometheus registry and its HTTP numbers.
type Metrics struct {
	Registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// NewMetrics builds the registry: Go and process numbers, the database
// pool, the HTTP numbers, the numbers read from the database at scrape
// time (queues, backups) and the open live streams.
func NewMetrics(db *sql.DB, streams func() int64) *Metrics {
	reg := prometheus.NewRegistry()
	m := newHTTPMetrics()
	m.Registry = reg
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewDBStatsCollector(db, "santral"),
		m.requests, m.duration, m.inFlight,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "santral_sse_streams", Help: "Live event streams open to the panel."},
			func() float64 { return float64(streams()) }),
		&queryCollector{db: db},
	)
	return m
}

// newHTTPMetrics builds the request numbers, not yet registered.
func newHTTPMetrics() *Metrics {
	return &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "santral_http_requests_total",
			Help: "HTTP requests answered, by method, route and status.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "santral_http_request_duration_seconds",
			Help:    "How long HTTP requests took, by method and route. Live streams are left out.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "santral_http_in_flight",
			Help: "HTTP requests being answered right now.",
		}),
	}
}

// Middleware counts every request by its route pattern (never the raw path,
// which would carry ids), status and duration. status maps an error a
// handler returned to the status the error handler will answer with.
func (m *Metrics) Middleware(status func(error) int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		m.inFlight.Inc()
		err := c.Next()
		m.inFlight.Dec()
		code := c.Response().StatusCode()
		if err != nil {
			code = status(err)
		}
		// Fiber hands out strings that point into buffers it reuses for the
		// next request; a label must own its copy.
		method, route := strings.Clone(c.Method()), "unmatched"
		if r := c.Route(); r != nil && r.Path != "" && r.Path != "/" {
			route = strings.Clone(r.Path)
		}
		if route == "/metrics" {
			return err // Prometheus reading the numbers is not traffic
		}
		m.requests.WithLabelValues(method, route, strconv.Itoa(code)).Inc()
		if string(c.Response().Header.ContentType()) != "text/event-stream" {
			m.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		}
		return err
	}
}

// gauge is a number read from the database when Prometheus scrapes.
type gauge struct {
	desc  *prometheus.Desc
	query string
}

func newGauge(name, help, query string) gauge {
	return gauge{desc: prometheus.NewDesc(name, help, nil, nil), query: query}
}

var gauges = []gauge{
	newGauge("santral_whatsapp_outbox_queued", "Outgoing WhatsApp messages waiting to be sent.", "SELECT count(*) FROM wa_messages WHERE status = 'queued'"),
	newGauge("santral_whatsapp_outbox_sending", "Outgoing WhatsApp messages being sent right now.", "SELECT count(*) FROM wa_messages WHERE status = 'sending'"),
	newGauge("santral_whatsapp_outbox_oldest_seconds", "Age of the oldest message waiting to be sent, 0 when none waits.", "SELECT COALESCE(EXTRACT(EPOCH FROM now() - min(created_at)), 0) FROM wa_messages WHERE status IN ('queued','sending')"),
	newGauge("santral_whatsapp_webhook_events_pending", "Meta notices waiting to be processed.", "SELECT count(*) FROM wa_webhook_events WHERE status = 'pending'"),
	newGauge("santral_whatsapp_webhook_events_failed", "Meta notices that could not be processed and are still waiting for a retry, however old.", "SELECT count(*) FROM wa_webhook_events WHERE status = 'failed'"),
	newGauge("santral_whatsapp_webhook_events_failed_recent", "Meta notices given up on in the last hour; the alert counts these, so one old notice does not keep it on.", "SELECT count(*) FROM wa_webhook_events WHERE status = 'failed' AND failed_at > now() - interval '1 hour'"),
	newGauge("santral_whatsapp_inbound_jobs", "Customer messages whose follow-up work is not done yet.", "SELECT count(*) FROM wa_inbound_jobs"),
	newGauge("santral_call_surveys_queued", "Call surveys waiting to be sent.", "SELECT count(*) FROM wa_call_surveys WHERE status = 'queued'"),
	newGauge("santral_call_logs_unverified", "Ended calls waiting for the phone system's record.", "SELECT count(*) FROM call_logs WHERE hooks_done = false"),
	newGauge("santral_backup_enabled", "1 when database backups are switched on.", "SELECT COALESCE(max(CASE WHEN enabled THEN 1 ELSE 0 END), 0) FROM backup_settings"),
	newGauge("santral_backup_last_success_timestamp_seconds", "When the last good backup finished, 0 when there was none.", "SELECT COALESCE(EXTRACT(EPOCH FROM max(finished_at)), 0) FROM backup_runs WHERE ok"),
	newGauge("santral_backup_last_failed", "1 when the most recent backup failed.", "SELECT COALESCE((SELECT CASE WHEN ok THEN 0 ELSE 1 END FROM backup_runs WHERE finished_at IS NOT NULL ORDER BY id DESC LIMIT 1), 0)"),
	newGauge("santral_users_active", "Active panel accounts.", "SELECT count(*) FROM users WHERE active AND deleted_at IS NULL"),
}

// queryCollector reads the database gauges at scrape time.
type queryCollector struct {
	db *sql.DB
}

var _ prometheus.Collector = (*queryCollector)(nil)

// Describe sends the gauges' descriptions.
func (q *queryCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, g := range gauges {
		ch <- g.desc
	}
}

// Collect reads every gauge; one that cannot be read is left out of this
// scrape and logged. While the database is down they are all missing;
// santral_dependency_up (from the health checks) says so, and the alerts
// on it fire.
func (q *queryCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, g := range gauges {
		var v float64
		if err := q.db.QueryRowContext(ctx, g.query).Scan(&v); err != nil {
			slog.WarnContext(ctx, "metric could not be read", "metric", g.desc.String(), "error", err)
			continue
		}
		ch <- prometheus.MustNewConstMetric(g.desc, prometheus.GaugeValue, v)
	}
}
