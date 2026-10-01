package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// tracer names the spans this server makes.
var tracer = otel.Tracer("santral")

// TraceConfig says where traces go. An empty Endpoint turns tracing off.
type TraceConfig struct {
	// Endpoint is the OTLP/HTTP address of the collector, for example
	// Jaeger's http://127.0.0.1:4318.
	Endpoint string
	// SampleRatio is the share of requests traced, 0 to 1.
	SampleRatio float64
	Version     string
}

// SetupTracing starts sending traces to the collector and returns the
// function that flushes and stops it at shutdown.
func SetupTracing(ctx context.Context, cfg TraceConfig) (func(context.Context) error, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(strings.TrimRight(cfg.Endpoint, "/")+"/v1/traces"))
	if err != nil {
		return nil, fmt.Errorf("trace exporter could not be created: %w", err)
	}
	ratio := cfg.SampleRatio
	if ratio <= 0 || ratio > 1 {
		ratio = 1
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", "santral"),
			attribute.String("service.version", cfg.Version),
		)),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tracer = tp.Tracer("santral")
	return tp.Shutdown, nil
}

// TraceMiddleware opens a span for every request, named after its route
// pattern, and hands the request's context on so database queries and
// outside calls made for it become its children.
func TraceMiddleware(status func(error) int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Concatenation copies Fiber's reused buffers into new strings.
		ctx, span := tracer.Start(c.UserContext(), c.Method()+" "+c.Path(), trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		c.SetUserContext(ctx)
		err := c.Next()
		code := c.Response().StatusCode()
		if err != nil {
			code = status(err)
		}
		if r := c.Route(); r != nil && r.Path != "" {
			span.SetName(c.Method() + " " + r.Path)
			span.SetAttributes(attribute.String("http.route", strings.Clone(r.Path)))
		}
		span.SetAttributes(
			attribute.String("http.request.method", strings.Clone(c.Method())),
			attribute.Int("http.response.status_code", code),
		)
		if code >= 500 {
			span.SetStatus(codes.Error, http.StatusText(code))
		}
		return err
	}
}

// TraceDatabase makes a child span for every query GORM runs. The span
// carries the statement with its placeholders, never the values.
func TraceDatabase(db *gorm.DB) error {
	cb := db.Callback()
	before := func(op string) func(*gorm.DB) {
		return func(tx *gorm.DB) {
			// Only queries made for a traced request are traced; the
			// background loops' polling would bury everything else.
			if tx.Statement == nil || tx.Statement.Context == nil || !trace.SpanContextFromContext(tx.Statement.Context).IsValid() {
				return
			}
			ctx, span := tracer.Start(tx.Statement.Context, "db "+op, trace.WithSpanKind(trace.SpanKindClient))
			tx.Statement.Context = ctx
			tx.InstanceSet("telemetry:span", span)
		}
	}
	after := func(tx *gorm.DB) {
		v, ok := tx.InstanceGet("telemetry:span")
		if !ok {
			return
		}
		span, ok := v.(trace.Span)
		if !ok {
			return
		}
		defer span.End()
		span.SetAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.query.text", tx.Statement.SQL.String()),
			attribute.String("db.collection.name", tx.Statement.Table),
			attribute.Int64("db.rows", tx.RowsAffected),
		)
		if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			span.RecordError(tx.Error)
			span.SetStatus(codes.Error, tx.Error.Error())
		}
	}
	for _, h := range []struct {
		op     string
		before func(string, func(*gorm.DB)) error
		after  func(string, func(*gorm.DB)) error
	}{
		{"create", cb.Create().Before("gorm:create").Register, cb.Create().After("gorm:create").Register},
		{"query", cb.Query().Before("gorm:query").Register, cb.Query().After("gorm:query").Register},
		{"update", cb.Update().Before("gorm:update").Register, cb.Update().After("gorm:update").Register},
		{"delete", cb.Delete().Before("gorm:delete").Register, cb.Delete().After("gorm:delete").Register},
		{"row", cb.Row().Before("gorm:row").Register, cb.Row().After("gorm:row").Register},
		{"raw", cb.Raw().Before("gorm:raw").Register, cb.Raw().After("gorm:raw").Register},
	} {
		if err := h.before("telemetry:before_"+h.op, before(h.op)); err != nil {
			return fmt.Errorf("query tracing could not be set up: %w", err)
		}
		if err := h.after("telemetry:after_"+h.op, after); err != nil {
			return fmt.Errorf("query tracing could not be set up: %w", err)
		}
	}
	return nil
}

// Transport wraps an HTTP client's transport so calls to outside systems
// (Meta, the phone system, Google) show in a request's trace.
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripper{base: base}
}

type roundTripper struct {
	base http.RoundTripper
}

// RoundTrip sends the request inside a client span.
func (t roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := tracer.Start(req.Context(), req.Method+" "+req.URL.Host, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(
		attribute.String("http.request.method", req.Method),
		attribute.String("server.address", req.URL.Host),
		attribute.String("url.path", req.URL.Path),
	)
	res, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", res.StatusCode))
	if res.StatusCode >= 500 {
		span.SetStatus(codes.Error, http.StatusText(res.StatusCode))
	}
	return res, nil
}
